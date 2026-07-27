package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	clickhouseclient "deploycrate-ce/clients/clickhouse"
	prometheusclient "deploycrate-ce/clients/prometheus"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type MetricRollupService struct {
	enabled    bool
	db         storage.Pool
	prometheus prometheusclient.Client
	clickhouse clickhouseclient.Client
	now        func() time.Time
}

func NewMetricRollupService(configuration config.Config, db storage.Pool) MetricRollupService {
	return MetricRollupService{
		enabled:    configuration.Metrics.Enabled,
		db:         db,
		prometheus: prometheusclient.New(configuration.Metrics.PrometheusURL),
		clickhouse: clickhouseclient.New(
			configuration.Metrics.ClickHouseURL,
			configuration.Metrics.ClickHouseDatabase,
			configuration.Metrics.ClickHouseUser,
			configuration.Metrics.ClickHousePassword,
		),
		now: time.Now,
	}
}

type rollupDefinition struct {
	name       string
	expression string
}

var metricRollupDefinitions = []rollupDefinition{
	{name: "cpu_utilization_percent", expression: `(1 - avg without (cpu, mode) (rate(node_cpu_seconds_total{mode="idle"}[1m]))) * 100`},
	{name: "load_1", expression: `node_load1`},
	{name: "memory_available_bytes", expression: `node_memory_MemAvailable_bytes`},
	{name: "root_filesystem_available_bytes", expression: `node_filesystem_avail_bytes{mountpoint="/",fstype!=""}`},
}

func (service MetricRollupService) Collect(ctx context.Context) error {
	if !service.enabled {
		return nil
	}
	observedAt := service.now().UTC().Truncate(time.Minute)
	bucketStart := observedAt.Add(-time.Minute)
	identities, err := models.Application.FindMetricRollupIdentities(ctx, service.db.Executor())
	if err != nil {
		return fmt.Errorf("load metric rollup identities: %w", err)
	}
	rollups := make([]clickhouseclient.MetricRollup, 0)
	for _, definition := range metricRollupDefinitions {
		values := make(map[string]*clickhouseclient.MetricRollup)
		for statistic, function := range map[string]string{
			"average": "avg_over_time",
			"maximum": "max_over_time",
			"last":    "last_over_time",
		} {
			expression := fmt.Sprintf("%s((%s)[1m:15s])", function, definition.expression)
			samples, err := service.prometheus.Query(ctx, expression, observedAt)
			if err != nil {
				return fmt.Errorf("collect %s %s: %w", definition.name, statistic, err)
			}
			for _, sample := range samples {
				key := rollupIdentity(sample.Labels)
				rollup := values[key]
				if rollup == nil {
					rollup = &clickhouseclient.MetricRollup{
						BucketStart: bucketStart, ObservedAt: observedAt, Metric: definition.name,
						Server: identities.Server, Environment: identities.Environment,
						Deployment: identities.Deployment, Target: identities.Target,
						Resource: sample.Labels["resource"], ObservationID: uuid.NewString(),
					}
					values[key] = rollup
				}
				switch statistic {
				case "average":
					rollup.Average = sample.Value
				case "maximum":
					rollup.Maximum = sample.Value
				case "last":
					rollup.Last = sample.Value
				}
			}
		}
		for _, rollup := range values {
			rollups = append(rollups, *rollup)
		}
	}
	return service.clickhouse.InsertMetricRollups(ctx, rollups)
}

func rollupIdentity(labels map[string]string) string {
	return strings.Join([]string{
		labels["server"], labels["environment"], labels["deployment"], labels["target"],
		labels["resource"], labels["instance"], labels["device"], labels["fstype"],
	}, "\x00")
}
