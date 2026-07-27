package services

import (
	"context"
	"fmt"
	"slices"
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
	clickhouse *ClickHouseResource
	now        func() time.Time
}

func NewMetricRollupService(
	configuration config.Config,
	db storage.Pool,
	clickhouse *ClickHouseResource,
) MetricRollupService {
	return MetricRollupService{
		enabled:    configuration.Metrics.Enabled,
		db:         db,
		prometheus: prometheusclient.New(configuration.Metrics.PrometheusURL),
		clickhouse: clickhouse,
		now:        time.Now,
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
	{name: "memory_total_bytes", expression: `node_memory_MemTotal_bytes`},
	{name: "root_filesystem_available_bytes", expression: `node_filesystem_avail_bytes{mountpoint="/",fstype!=""}`},
	{name: "root_filesystem_size_bytes", expression: `node_filesystem_size_bytes{mountpoint="/",fstype!=""}`},
}

type SystemResourceUsage struct {
	Used float64 `json:"used"`
	Free float64 `json:"free"`
}

type SystemTelemetry struct {
	Available      bool                          `json:"available"`
	ObservedAt     time.Time                     `json:"observedAt"`
	CPU            SystemResourceUsage           `json:"cpu"`
	Memory         SystemResourceUsage           `json:"memory"`
	Storage        SystemResourceUsage           `json:"storage"`
	MemoryHistory  []SystemTelemetryHistoryPoint `json:"memoryHistory"`
	StorageHistory []SystemTelemetryHistoryPoint `json:"storageHistory"`
}

const systemTelemetryFreshness = 5 * time.Minute
const systemTelemetryHistoryWindow = 24 * time.Hour

type SystemTelemetryHistoryPoint struct {
	ObservedAt time.Time `json:"observedAt"`
	Used       float64   `json:"used"`
	Free       float64   `json:"free"`
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
	client, err := service.clickhouse.Client(ctx)
	if err != nil {
		return err
	}
	return client.InsertMetricRollups(ctx, rollups)
}

func (service MetricRollupService) SystemTelemetry(
	ctx context.Context,
	server string,
) (SystemTelemetry, error) {
	if !service.enabled || server == "" {
		return SystemTelemetry{}, nil
	}
	client, err := service.clickhouse.Client(ctx)
	if err != nil {
		return SystemTelemetry{}, err
	}
	values, err := client.LatestSystemMetricValues(ctx, server)
	if err != nil {
		return SystemTelemetry{}, err
	}
	metrics := make(map[string]clickhouseclient.MetricValue, len(values))
	for _, value := range values {
		metrics[value.Metric] = value
	}
	required := []string{
		"cpu_utilization_percent",
		"memory_available_bytes",
		"memory_total_bytes",
		"root_filesystem_available_bytes",
		"root_filesystem_size_bytes",
	}
	observedAt := service.now().UTC()
	for _, metric := range required {
		value, ok := metrics[metric]
		if !ok || service.now().UTC().Sub(value.ObservedAt) > systemTelemetryFreshness {
			return SystemTelemetry{}, nil
		}
		if value.ObservedAt.Before(observedAt) {
			observedAt = value.ObservedAt
		}
	}

	memoryTotal := metrics["memory_total_bytes"].Value
	storageTotal := metrics["root_filesystem_size_bytes"].Value
	if memoryTotal <= 0 || storageTotal <= 0 {
		return SystemTelemetry{}, nil
	}
	cpuUsed := clamp(metrics["cpu_utilization_percent"].Value, 0, 100)
	memoryFree := clamp(
		metrics["memory_available_bytes"].Value,
		0,
		memoryTotal,
	)
	memoryUsed := memoryTotal - memoryFree
	storageFree := clamp(
		metrics["root_filesystem_available_bytes"].Value,
		0,
		storageTotal,
	)
	storageUsed := storageTotal - storageFree

	telemetry := SystemTelemetry{
		Available:      true,
		ObservedAt:     observedAt,
		CPU:            SystemResourceUsage{Used: cpuUsed, Free: 100 - cpuUsed},
		Memory:         SystemResourceUsage{Used: memoryUsed, Free: memoryFree},
		Storage:        SystemResourceUsage{Used: storageUsed, Free: storageFree},
		MemoryHistory:  []SystemTelemetryHistoryPoint{},
		StorageHistory: []SystemTelemetryHistoryPoint{},
	}
	history, err := client.SystemMetricHistory(
		ctx,
		server,
		service.now().UTC().Add(-systemTelemetryHistoryWindow),
	)
	if err != nil {
		return telemetry, err
	}
	telemetry.MemoryHistory = systemResourceHistory(
		history,
		"memory_available_bytes",
		"memory_total_bytes",
	)
	telemetry.StorageHistory = systemResourceHistory(
		history,
		"root_filesystem_available_bytes",
		"root_filesystem_size_bytes",
	)
	return telemetry, nil
}

func systemResourceHistory(
	values []clickhouseclient.MetricHistoryValue,
	availableMetric string,
	totalMetric string,
) []SystemTelemetryHistoryPoint {
	type bucket struct {
		observedAt time.Time
		available  float64
		total      float64
		hasFree    bool
		hasTotal   bool
	}
	buckets := make(map[int64]*bucket)
	for _, value := range values {
		if value.Metric != availableMetric && value.Metric != totalMetric {
			continue
		}
		key := value.BucketStart.UnixMilli()
		current := buckets[key]
		if current == nil {
			current = &bucket{observedAt: value.BucketStart}
			buckets[key] = current
		}
		if value.Metric == availableMetric {
			current.available = value.Value
			current.hasFree = true
		} else {
			current.total = value.Value
			current.hasTotal = true
		}
	}
	keys := make([]int64, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	points := make([]SystemTelemetryHistoryPoint, 0, len(keys))
	for _, key := range keys {
		current := buckets[key]
		if !current.hasFree || !current.hasTotal || current.total <= 0 {
			continue
		}
		free := clamp(current.available, 0, current.total)
		points = append(points, SystemTelemetryHistoryPoint{
			ObservedAt: current.observedAt,
			Used:       current.total - free,
			Free:       free,
		})
	}
	return points
}

func clamp(value, minimum, maximum float64) float64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func rollupIdentity(labels map[string]string) string {
	return strings.Join([]string{
		labels["server"], labels["environment"], labels["deployment"], labels["target"],
		labels["resource"], labels["instance"], labels["device"], labels["fstype"],
	}, "\x00")
}
