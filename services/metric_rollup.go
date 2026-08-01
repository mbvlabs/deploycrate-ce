package services

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	clickhouseclient "deploycrate-ce/clients/clickhouse"
	prometheusclient "deploycrate-ce/clients/prometheus"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/telemetry"

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
		enabled: configuration.Metrics.Enabled, db: db,
		prometheus: prometheusclient.New(configuration.Metrics.PrometheusURL),
		clickhouse: clickhouse, now: time.Now,
	}
}

type rollupDefinition struct {
	scope      string
	name       string
	expression string
}

const containerIdentityLabels = "application,environment,release,deployment,instance,resource_installation,component,id"

var metricRollupDefinitions = []rollupDefinition{
	{scope: "host", name: "cpu_cores_used", expression: `sum(rate(node_cpu_seconds_total{mode!="idle"}[1m]))`},
	{scope: "host", name: "cpu_cores_total", expression: `count(node_cpu_seconds_total{mode="idle"})`},
	{scope: "host", name: "memory_available_bytes", expression: `node_memory_MemAvailable_bytes`},
	{scope: "host", name: "memory_total_bytes", expression: `node_memory_MemTotal_bytes`},
	{scope: "host", name: "root_filesystem_available_bytes", expression: `node_filesystem_avail_bytes{mountpoint="/",fstype!=""}`},
	{scope: "host", name: "root_filesystem_size_bytes", expression: `node_filesystem_size_bytes{mountpoint="/",fstype!=""}`},
	{scope: "host", name: "disk_read_bytes_per_second", expression: `sum(rate(node_disk_read_bytes_total{device!~"loop.*|ram.*"}[1m]))`},
	{scope: "host", name: "disk_write_bytes_per_second", expression: `sum(rate(node_disk_written_bytes_total{device!~"loop.*|ram.*"}[1m]))`},
	{scope: "host", name: "network_receive_bytes_per_second", expression: `sum(rate(node_network_receive_bytes_total{device!="lo"}[1m]))`},
	{scope: "host", name: "network_transmit_bytes_per_second", expression: `sum(rate(node_network_transmit_bytes_total{device!="lo"}[1m]))`},
	{scope: "host", name: "oom_events", expression: `clamp_min(increase(node_vmstat_oom_kill[1m]), 0)`},
	{scope: "host", name: "tasks", expression: `node_procs_running`},

	{scope: "container", name: "cpu_cores_used", expression: `sum by (` + containerIdentityLabels + `) (rate(container_cpu_usage_seconds_total{id!="/",component!~"prometheus|node-exporter|cadvisor|docker|caddy|otel-collector|deploycrate-ce"}[1m]))`},
	{scope: "container", name: "memory_working_set_bytes", expression: `sum by (` + containerIdentityLabels + `) (container_memory_working_set_bytes{id!="/",component!~"prometheus|node-exporter|cadvisor|docker|caddy|otel-collector|deploycrate-ce"})`},
	{scope: "container", name: "disk_read_bytes_per_second", expression: `sum by (` + containerIdentityLabels + `) (rate(container_fs_reads_bytes_total{id!="/",component!~"prometheus|node-exporter|cadvisor|docker|caddy|otel-collector|deploycrate-ce"}[1m]))`},
	{scope: "container", name: "disk_write_bytes_per_second", expression: `sum by (` + containerIdentityLabels + `) (rate(container_fs_writes_bytes_total{id!="/",component!~"prometheus|node-exporter|cadvisor|docker|caddy|otel-collector|deploycrate-ce"}[1m]))`},
	{scope: "container", name: "network_receive_bytes_per_second", expression: `sum by (` + containerIdentityLabels + `) (rate(container_network_receive_bytes_total{id!="/",component!~"prometheus|node-exporter|cadvisor|docker|caddy|otel-collector|deploycrate-ce"}[1m]))`},
	{scope: "container", name: "network_transmit_bytes_per_second", expression: `sum by (` + containerIdentityLabels + `) (rate(container_network_transmit_bytes_total{id!="/",component!~"prometheus|node-exporter|cadvisor|docker|caddy|otel-collector|deploycrate-ce"}[1m]))`},
	{scope: "container", name: "oom_events", expression: `sum by (` + containerIdentityLabels + `) (clamp_min(increase(container_oom_events_total{id!="/",component!~"prometheus|node-exporter|cadvisor|docker|caddy|otel-collector|deploycrate-ce"}[1m]), 0))`},
	{scope: "container", name: "cpu_throttling_ratio", expression: `sum by (` + containerIdentityLabels + `) (rate(container_cpu_cfs_throttled_periods_total{id!="/",component!~"prometheus|node-exporter|cadvisor|docker|caddy|otel-collector|deploycrate-ce"}[1m])) / clamp_min(sum by (` + containerIdentityLabels + `) (rate(container_cpu_cfs_periods_total{id!="/",component!~"prometheus|node-exporter|cadvisor|docker|caddy|otel-collector|deploycrate-ce"}[1m])), 1)`},
	{scope: "container", name: "tasks", expression: `sum by (` + containerIdentityLabels + `) (container_processes{id!="/",component!~"prometheus|node-exporter|cadvisor|docker|caddy|otel-collector|deploycrate-ce"})`},

	{scope: "native", name: "cpu_cores_used", expression: `sum by (component) (rate(container_cpu_usage_seconds_total{id=~"/system.slice/(prometheus|node-exporter|cadvisor|docker|caddy|otelcol-contrib|deploycrate-ce@(blue|green))\\.service"}[1m]))`},
	{scope: "native", name: "memory_working_set_bytes", expression: `sum by (component) (container_memory_working_set_bytes{id=~"/system.slice/(prometheus|node-exporter|cadvisor|docker|caddy|otelcol-contrib|deploycrate-ce@(blue|green))\\.service"})`},
	{scope: "native", name: "disk_read_bytes_per_second", expression: `sum by (component) (rate(container_fs_reads_bytes_total{id=~"/system.slice/(prometheus|node-exporter|cadvisor|docker|caddy|otelcol-contrib|deploycrate-ce@(blue|green))\\.service"}[1m]))`},
	{scope: "native", name: "disk_write_bytes_per_second", expression: `sum by (component) (rate(container_fs_writes_bytes_total{id=~"/system.slice/(prometheus|node-exporter|cadvisor|docker|caddy|otelcol-contrib|deploycrate-ce@(blue|green))\\.service"}[1m]))`},
	{scope: "native", name: "tasks", expression: `sum by (component) (container_processes{id=~"/system.slice/(prometheus|node-exporter|cadvisor|docker|caddy|otelcol-contrib|deploycrate-ce@(blue|green))\\.service"})`},
}

type SystemResourceUsage struct {
	Used float64 `json:"used"`
	Free float64 `json:"free"`
}

type SystemTelemetry struct {
	Available                bool                           `json:"available"`
	ObservedAt               time.Time                      `json:"observedAt"`
	CPU                      SystemResourceUsage            `json:"cpu"`
	CPUCoresUsed             float64                        `json:"cpuCoresUsed"`
	CPUCoresTotal            float64                        `json:"cpuCoresTotal"`
	DiskReadBytesPS          float64                        `json:"diskReadBytesPerSecond"`
	DiskWriteBytesPS         float64                        `json:"diskWriteBytesPerSecond"`
	NetworkReceiveBPS        float64                        `json:"networkReceiveBytesPerSecond"`
	NetworkTransmitBPS       float64                        `json:"networkTransmitBytesPerSecond"`
	OOMEvents                float64                        `json:"oomEvents"`
	Tasks                    float64                        `json:"tasks"`
	DiskReadAvailable        bool                           `json:"diskReadAvailable"`
	DiskWriteAvailable       bool                           `json:"diskWriteAvailable"`
	NetworkReceiveAvailable  bool                           `json:"networkReceiveAvailable"`
	NetworkTransmitAvailable bool                           `json:"networkTransmitAvailable"`
	OOMAvailable             bool                           `json:"oomAvailable"`
	TasksAvailable           bool                           `json:"tasksAvailable"`
	Memory                   SystemResourceUsage            `json:"memory"`
	Storage                  SystemResourceUsage            `json:"storage"`
	MemoryHistory            []SystemTelemetryHistoryPoint  `json:"memoryHistory"`
	StorageHistory           []SystemTelemetryHistoryPoint  `json:"storageHistory"`
	HostHistory              []SystemThroughputHistoryPoint `json:"hostHistory"`
	Platform                 []AttributedTelemetryRow       `json:"platform"`
	SystemContainers         []AttributedTelemetryRow       `json:"systemContainers"`
}

type SystemTelemetryHistoryPoint struct {
	ObservedAt time.Time `json:"observedAt"`
	Used       float64   `json:"used"`
	Free       float64   `json:"free"`
}

type SystemThroughputHistoryPoint struct {
	ObservedAt               time.Time `json:"observedAt"`
	CPUCores                 float64   `json:"cpuCores"`
	CPUCoresTotal            float64   `json:"cpuCoresTotal"`
	DiskReadBytesPS          float64   `json:"diskReadBytesPerSecond"`
	DiskWriteBytesPS         float64   `json:"diskWriteBytesPerSecond"`
	NetworkReceiveBPS        float64   `json:"networkReceiveBytesPerSecond"`
	NetworkTransmitBPS       float64   `json:"networkTransmitBytesPerSecond"`
	CPUAvailable             bool      `json:"cpuAvailable"`
	DiskReadAvailable        bool      `json:"diskReadAvailable"`
	DiskWriteAvailable       bool      `json:"diskWriteAvailable"`
	NetworkReceiveAvailable  bool      `json:"networkReceiveAvailable"`
	NetworkTransmitAvailable bool      `json:"networkTransmitAvailable"`
}

type AttributedTelemetryPoint struct {
	ObservedAt               time.Time `json:"observedAt"`
	CPUCores                 float64   `json:"cpuCores"`
	MemoryBytes              float64   `json:"memoryBytes"`
	DiskReadBytesPS          float64   `json:"diskReadBytesPerSecond"`
	DiskWriteBytesPS         float64   `json:"diskWriteBytesPerSecond"`
	NetworkReceiveBPS        float64   `json:"networkReceiveBytesPerSecond"`
	NetworkTransmitBPS       float64   `json:"networkTransmitBytesPerSecond"`
	CPUAvailable             bool      `json:"cpuAvailable"`
	MemoryAvailable          bool      `json:"memoryAvailable"`
	DiskReadAvailable        bool      `json:"diskReadAvailable"`
	DiskWriteAvailable       bool      `json:"diskWriteAvailable"`
	NetworkReceiveAvailable  bool      `json:"networkReceiveAvailable"`
	NetworkTransmitAvailable bool      `json:"networkTransmitAvailable"`
}

type AttributedTelemetryRow struct {
	Scope                    string                     `json:"scope"`
	Component                string                     `json:"component"`
	ResourceName             string                     `json:"resourceName"`
	ContainerName            string                     `json:"containerName"`
	Application              string                     `json:"application"`
	Environment              string                     `json:"environment"`
	Release                  string                     `json:"release"`
	Deployment               string                     `json:"deployment"`
	Target                   string                     `json:"target"`
	Instance                 string                     `json:"instance"`
	Resource                 string                     `json:"resource"`
	Installation             string                     `json:"installation"`
	DatabaseCluster          string                     `json:"databaseCluster"`
	DatabaseClusterNode      string                     `json:"databaseClusterNode"`
	Available                bool                       `json:"available"`
	ObservedAt               time.Time                  `json:"observedAt"`
	CPUCores                 float64                    `json:"cpuCores"`
	MemoryBytes              float64                    `json:"memoryBytes"`
	DiskReadBytesPS          float64                    `json:"diskReadBytesPerSecond"`
	DiskWriteBytesPS         float64                    `json:"diskWriteBytesPerSecond"`
	NetworkReceiveBPS        float64                    `json:"networkReceiveBytesPerSecond"`
	NetworkTransmitBPS       float64                    `json:"networkTransmitBytesPerSecond"`
	OOMEvents                float64                    `json:"oomEvents"`
	CPUThrottlingRatio       float64                    `json:"cpuThrottlingRatio"`
	Tasks                    float64                    `json:"tasks"`
	CPUAvailable             bool                       `json:"cpuAvailable"`
	MemoryAvailable          bool                       `json:"memoryAvailable"`
	DiskReadAvailable        bool                       `json:"diskReadAvailable"`
	DiskWriteAvailable       bool                       `json:"diskWriteAvailable"`
	NetworkReceiveAvailable  bool                       `json:"networkReceiveAvailable"`
	NetworkTransmitAvailable bool                       `json:"networkTransmitAvailable"`
	OOMAvailable             bool                       `json:"oomAvailable"`
	CPUThrottlingAvailable   bool                       `json:"cpuThrottlingAvailable"`
	TasksAvailable           bool                       `json:"tasksAvailable"`
	History                  []AttributedTelemetryPoint `json:"history"`
}

const systemTelemetryFreshness = 5 * time.Minute
const systemTelemetryHistoryWindow = 24 * time.Hour

var errRejectedMetricIdentity = errors.New("metric identity is invalid or contradictory")

type metricIdentityResolver struct {
	ctx    context.Context
	db     storage.Executor
	system models.MetricRollupIdentities
	cache  map[string]models.MetricRollupIdentities
}

func (service MetricRollupService) Collect(ctx context.Context) (resultErr error) {
	if !service.enabled {
		return nil
	}
	startedAt := time.Now()
	insertedRows := 0
	rejectedSamples := 0
	defer func() {
		telemetry.RecordMetricRollup(ctx, resultErr == nil, insertedRows, rejectedSamples, time.Since(startedAt))
	}()

	observedAt := service.now().UTC().Truncate(time.Minute)
	bucketStart := observedAt.Add(-time.Minute)
	systemIdentities, err := models.Application.FindMetricRollupIdentities(ctx, service.db.Executor())
	if err != nil {
		return fmt.Errorf("load system metric rollup identities: %w", err)
	}
	resolver := metricIdentityResolver{
		ctx: ctx, db: service.db.Executor(), system: systemIdentities,
		cache: make(map[string]models.MetricRollupIdentities),
	}
	rollups := make([]clickhouseclient.MetricRollup, 0)
	queryErrors := make([]error, 0)
	for _, definition := range metricRollupDefinitions {
		values := make(map[string]*clickhouseclient.MetricRollup)
		definitionFailed := false
		for _, statistic := range []struct {
			name     string
			function string
		}{
			{name: "average", function: "avg_over_time"},
			{name: "maximum", function: "max_over_time"},
			{name: "last", function: "last_over_time"},
		} {
			expression := fmt.Sprintf("%s((%s)[1m:15s])", statistic.function, definition.expression)
			samples, queryErr := service.prometheus.Query(ctx, expression, observedAt)
			if queryErr != nil {
				queryErrors = append(queryErrors, fmt.Errorf("collect %s %s: %w", definition.name, statistic.name, queryErr))
				definitionFailed = true
				break
			}
			for _, sample := range samples {
				identities, resolveErr := resolver.resolve(definition.scope, sample.Labels)
				if errors.Is(resolveErr, errRejectedMetricIdentity) || errors.Is(resolveErr, sql.ErrNoRows) {
					rejectedSamples++
					continue
				}
				if resolveErr != nil {
					queryErrors = append(queryErrors, fmt.Errorf("resolve %s metric identity: %w", definition.scope, resolveErr))
					continue
				}
				key := rollupIdentity(definition.scope, identities, sample.Labels)
				rollup := values[key]
				if rollup == nil {
					rollup = &clickhouseclient.MetricRollup{
						BucketStart: bucketStart, ObservedAt: observedAt,
						Scope: definition.scope, Component: sample.Labels["component"], Metric: definition.name,
						Server: identities.Server, Application: identities.Application,
						Environment: identities.Environment, Release: identities.Release,
						Deployment: identities.Deployment, Target: identities.Target,
						Instance: identities.Instance, Resource: identities.Resource,
						Installation: identities.Installation, DatabaseCluster: identities.DatabaseCluster,
						DatabaseClusterNode: identities.DatabaseClusterNode,
						RuntimeID:           runtimeIDFromCgroup(sample.Labels["id"]), ObservationID: uuid.NewString(),
					}
					values[key] = rollup
				}
				switch statistic.name {
				case "average":
					rollup.Average = sample.Value
				case "maximum":
					rollup.Maximum = sample.Value
				case "last":
					rollup.Last = sample.Value
				}
			}
		}
		if definitionFailed {
			continue
		}
		for _, rollup := range values {
			rollups = append(rollups, *rollup)
		}
	}
	client, err := service.clickhouse.Client(ctx)
	if err != nil {
		return errors.Join(append(queryErrors, err)...)
	}
	if err := client.InsertMetricRollups(ctx, rollups); err != nil {
		return errors.Join(append(queryErrors, err)...)
	}
	insertedRows = len(rollups)
	if len(queryErrors) > 0 {
		slog.WarnContext(ctx, "metric rollup completed with partial query failures", "inserted_rows", insertedRows, "rejected_samples", rejectedSamples, "failed_queries", len(queryErrors), "error", errors.Join(queryErrors...))
		if insertedRows == 0 {
			return errors.Join(queryErrors...)
		}
	} else {
		slog.InfoContext(ctx, "metric rollup completed", "inserted_rows", insertedRows, "rejected_samples", rejectedSamples)
	}
	return nil
}

func (resolver *metricIdentityResolver) resolve(scope string, labels map[string]string) (models.MetricRollupIdentities, error) {
	key := scope + "\x00" + labels["application"] + "\x00" + labels["environment"] + "\x00" +
		labels["release"] + "\x00" + labels["deployment"] + "\x00" + labels["instance"] + "\x00" +
		labels["resource_installation"] + "\x00" + labels["component"]
	if identity, ok := resolver.cache[key]; ok {
		return identity, nil
	}
	var identity models.MetricRollupIdentities
	var err error
	switch scope {
	case "host":
		identity = models.MetricRollupIdentities{Server: resolver.system.Server}
	case "native":
		if !slices.Contains([]string{"prometheus", "node-exporter", "cadvisor", "docker", "caddy", "otel-collector", "deploycrate-ce"}, labels["component"]) {
			return models.MetricRollupIdentities{}, errRejectedMetricIdentity
		}
		identity = models.MetricRollupIdentities{Server: resolver.system.Server}
		if labels["component"] == "deploycrate-ce" {
			identity = resolver.system
		}
	case "container":
		instanceValue := labels["instance"]
		installationValue := labels["resource_installation"]
		component := labels["component"]
		if instanceValue != "" && installationValue != "" {
			return models.MetricRollupIdentities{}, errRejectedMetricIdentity
		}
		if instanceValue != "" {
			instanceID, parseErr := uuid.Parse(instanceValue)
			if parseErr != nil {
				return models.MetricRollupIdentities{}, errRejectedMetricIdentity
			}
			identity, err = models.Application.FindMetricWorkloadIdentities(resolver.ctx, resolver.db, instanceID)
			if err == nil && !labelsMatchIdentity(labels, identity) {
				return models.MetricRollupIdentities{}, errRejectedMetricIdentity
			}
		} else if installationValue != "" {
			installationID, parseErr := uuid.Parse(installationValue)
			if parseErr != nil {
				return models.MetricRollupIdentities{}, errRejectedMetricIdentity
			}
			identity, err = models.Application.FindMetricResourceIdentities(resolver.ctx, resolver.db, installationID)
			if errors.Is(err, sql.ErrNoRows) {
				identity, err = models.Application.FindMetricDatabaseInstallationIdentities(resolver.ctx, resolver.db, installationID)
			}
			if err == nil && identity.Installation != installationID.String() {
				return models.MetricRollupIdentities{}, errRejectedMetricIdentity
			}
		} else if slices.Contains([]string{"postgresql", "clickhouse"}, component) {
			identity = models.MetricRollupIdentities{Server: resolver.system.Server}
		} else {
			return models.MetricRollupIdentities{}, errRejectedMetricIdentity
		}
	default:
		return models.MetricRollupIdentities{}, errRejectedMetricIdentity
	}
	if err != nil {
		return models.MetricRollupIdentities{}, err
	}
	resolver.cache[key] = identity
	return identity, nil
}

func labelsMatchIdentity(labels map[string]string, identity models.MetricRollupIdentities) bool {
	return labels["application"] == identity.Application &&
		labels["environment"] == identity.Environment &&
		labels["release"] == identity.Release &&
		labels["deployment"] == identity.Deployment &&
		labels["instance"] == identity.Instance
}

func runtimeIDFromCgroup(value string) string {
	part := value
	if index := strings.LastIndex(part, "/"); index >= 0 {
		part = part[index+1:]
	}
	part = strings.TrimSuffix(strings.TrimPrefix(part, "docker-"), ".scope")
	if len(part) < 12 || len(part) > 64 {
		return ""
	}
	if _, err := hex.DecodeString(part); err != nil {
		return ""
	}
	return part
}

func rollupIdentity(scope string, identity models.MetricRollupIdentities, labels map[string]string) string {
	return strings.Join([]string{
		scope, labels["component"], identity.Server, identity.Application, identity.Environment,
		identity.Release, identity.Deployment, identity.Target, identity.Instance,
		identity.Resource, identity.Installation, runtimeIDFromCgroup(labels["id"]),
		identity.DatabaseCluster, identity.DatabaseClusterNode,
	}, "\x00")
}

func (service MetricRollupService) HostTelemetry(ctx context.Context, server string) (SystemTelemetry, error) {
	result := emptySystemTelemetry()
	if !service.enabled || server == "" {
		return result, nil
	}
	client, err := service.clickhouse.Client(ctx)
	if err != nil {
		return result, err
	}
	values, latestErr := client.LatestSystemMetricValues(ctx, server)
	history, historyErr := client.SystemMetricHistory(ctx, server, service.now().UTC().Add(-systemTelemetryHistoryWindow))
	if historyErr == nil {
		result.MemoryHistory = systemResourceHistory(history, "memory_available_bytes", "memory_total_bytes")
		result.StorageHistory = systemResourceHistory(history, "root_filesystem_available_bytes", "root_filesystem_size_bytes")
		result.HostHistory = systemThroughputHistory(history)
	}
	if latestErr != nil {
		return result, errors.Join(latestErr, historyErr)
	}
	metrics := make(map[string]clickhouseclient.MetricValue, len(values))
	for _, value := range values {
		metrics[value.Metric] = value
	}
	required := []string{
		"cpu_cores_used", "cpu_cores_total", "memory_available_bytes", "memory_total_bytes",
		"root_filesystem_available_bytes", "root_filesystem_size_bytes",
	}
	observedAt := service.now().UTC()
	for _, metric := range required {
		value, ok := metrics[metric]
		if !ok || service.now().UTC().Sub(value.ObservedAt) > systemTelemetryFreshness {
			return result, historyErr
		}
		if value.ObservedAt.Before(observedAt) {
			observedAt = value.ObservedAt
		}
	}
	memoryTotal := metrics["memory_total_bytes"].Value
	storageTotal := metrics["root_filesystem_size_bytes"].Value
	cpuTotal := metrics["cpu_cores_total"].Value
	if memoryTotal <= 0 || storageTotal <= 0 || cpuTotal <= 0 {
		return result, historyErr
	}
	cpuCores := clamp(metrics["cpu_cores_used"].Value, 0, cpuTotal)
	cpuUsed := cpuCores / cpuTotal * 100
	memoryFree := clamp(metrics["memory_available_bytes"].Value, 0, memoryTotal)
	storageFree := clamp(metrics["root_filesystem_available_bytes"].Value, 0, storageTotal)
	diskRead, diskReadAvailable := freshSystemMetric(metrics, "disk_read_bytes_per_second", service.now().UTC())
	diskWrite, diskWriteAvailable := freshSystemMetric(metrics, "disk_write_bytes_per_second", service.now().UTC())
	networkReceive, networkReceiveAvailable := freshSystemMetric(metrics, "network_receive_bytes_per_second", service.now().UTC())
	networkTransmit, networkTransmitAvailable := freshSystemMetric(metrics, "network_transmit_bytes_per_second", service.now().UTC())
	oomEvents, oomAvailable := freshSystemMetric(metrics, "oom_events", service.now().UTC())
	tasks, tasksAvailable := freshSystemMetric(metrics, "tasks", service.now().UTC())
	result.Available = true
	result.ObservedAt = observedAt
	result.CPU = SystemResourceUsage{Used: cpuUsed, Free: 100 - cpuUsed}
	result.CPUCoresUsed = cpuCores
	result.CPUCoresTotal = cpuTotal
	result.DiskReadBytesPS, result.DiskReadAvailable = diskRead, diskReadAvailable
	result.DiskWriteBytesPS, result.DiskWriteAvailable = diskWrite, diskWriteAvailable
	result.NetworkReceiveBPS, result.NetworkReceiveAvailable = networkReceive, networkReceiveAvailable
	result.NetworkTransmitBPS, result.NetworkTransmitAvailable = networkTransmit, networkTransmitAvailable
	result.OOMEvents, result.OOMAvailable = oomEvents, oomAvailable
	result.Tasks, result.TasksAvailable = tasks, tasksAvailable
	result.Memory = SystemResourceUsage{Used: memoryTotal - memoryFree, Free: memoryFree}
	result.Storage = SystemResourceUsage{Used: storageTotal - storageFree, Free: storageFree}
	return result, historyErr
}

func emptySystemTelemetry() SystemTelemetry {
	return SystemTelemetry{
		MemoryHistory:    []SystemTelemetryHistoryPoint{},
		StorageHistory:   []SystemTelemetryHistoryPoint{},
		HostHistory:      []SystemThroughputHistoryPoint{},
		Platform:         []AttributedTelemetryRow{},
		SystemContainers: []AttributedTelemetryRow{},
	}
}

func (service MetricRollupService) SystemTelemetry(ctx context.Context, server string) (SystemTelemetry, error) {
	result, hostErr := service.HostTelemetry(ctx, server)
	if server == "" {
		return result, hostErr
	}
	serverID, err := uuid.Parse(server)
	if err != nil {
		return result, errors.Join(hostErr, fmt.Errorf("parse system telemetry server: %w", err))
	}
	inventory, inventoryErr := models.Application.FindSystemTelemetryContainers(ctx, service.db.Executor(), serverID)
	result.SystemContainers = mergeSystemContainerInventory(result.SystemContainers, inventory)
	if !service.enabled {
		return result, errors.Join(hostErr, inventoryErr)
	}
	client, err := service.clickhouse.Client(ctx)
	if err != nil {
		return result, errors.Join(hostErr, inventoryErr, err)
	}
	queryErrors := []error{hostErr, inventoryErr}
	platform, err := client.LatestAttributedMetricValues(ctx, "native", server, "")
	if err != nil {
		queryErrors = append(queryErrors, err)
	} else {
		historySince := service.now().UTC().Add(-systemTelemetryHistoryWindow)
		platformHistory, historyErr := client.AttributedMetricHistory(ctx, "native", server, "", historySince)
		if historyErr != nil {
			queryErrors = append(queryErrors, historyErr)
		}
		result.Platform = attributedTelemetryRows(platform, platformHistory, service.now().UTC())
	}
	containers, err := client.LatestAttributedMetricValues(ctx, "container", server, "")
	if err != nil {
		queryErrors = append(queryErrors, err)
	} else {
		containers = systemContainerMetricValues(containers)
		containerHistory, historyErr := client.AttributedMetricHistory(ctx, "container", server, "", service.now().UTC().Add(-systemTelemetryHistoryWindow))
		if historyErr != nil {
			queryErrors = append(queryErrors, historyErr)
		}
		containerHistory = systemContainerMetricValues(containerHistory)
		rows := attributedTelemetryRows(containers, containerHistory, service.now().UTC())
		result.SystemContainers = mergeSystemContainerInventory(rows, inventory)
	}
	return result, errors.Join(queryErrors...)
}

func mergeSystemContainerInventory(
	rows []AttributedTelemetryRow,
	inventory []models.SystemTelemetryContainer,
) []AttributedTelemetryRow {
	result := make([]AttributedTelemetryRow, 0, len(inventory))
	matched := make([]bool, len(rows))
	for _, container := range inventory {
		match := -1
		for index := range rows {
			if matched[index] {
				continue
			}
			if rows[index].Installation == container.InstallationID ||
				(rows[index].Installation == "" && rows[index].Resource == "" && rows[index].Component == container.ResourceKind) {
				match = index
				break
			}
		}
		if match < 0 {
			result = append(result, AttributedTelemetryRow{
				Scope: "container", Component: container.ResourceKind,
				Resource: container.ResourceID, Installation: container.InstallationID,
				DatabaseCluster: container.DatabaseClusterID, DatabaseClusterNode: container.DatabaseClusterNodeID,
				ResourceName: container.ResourceName, ContainerName: container.ContainerName,
				History: []AttributedTelemetryPoint{},
			})
			continue
		}
		matched[match] = true
		row := rows[match]
		row.Resource = container.ResourceID
		row.Installation = container.InstallationID
		row.ResourceName = container.ResourceName
		row.ContainerName = container.ContainerName
		row.DatabaseCluster = container.DatabaseClusterID
		row.DatabaseClusterNode = container.DatabaseClusterNodeID
		result = append(result, row)
	}
	slices.SortFunc(result, func(a, b AttributedTelemetryRow) int {
		return strings.Compare(a.ResourceName+a.Component+a.ContainerName, b.ResourceName+b.Component+b.ContainerName)
	})
	return result
}

func systemContainerMetricValues(values []clickhouseclient.AttributedMetricValue) []clickhouseclient.AttributedMetricValue {
	result := make([]clickhouseclient.AttributedMetricValue, 0, len(values))
	for _, value := range values {
		if value.Instance == "" {
			result = append(result, value)
		}
	}
	return result
}

func freshSystemMetric(
	metrics map[string]clickhouseclient.MetricValue,
	name string,
	now time.Time,
) (float64, bool) {
	value, ok := metrics[name]
	if !ok || now.Sub(value.ObservedAt) > systemTelemetryFreshness {
		return 0, false
	}
	return value.Value, true
}

func (service MetricRollupService) EnvironmentTelemetry(ctx context.Context, environmentID uuid.UUID) ([]AttributedTelemetryRow, error) {
	if !service.enabled || environmentID == uuid.Nil {
		return []AttributedTelemetryRow{}, nil
	}
	target, err := models.EnvironmentTarget.ActiveForEnvironment(ctx, service.db.Executor(), environmentID)
	if err != nil {
		return nil, err
	}
	client, err := service.clickhouse.Client(ctx)
	if err != nil {
		return nil, err
	}
	current, err := client.LatestAttributedMetricValues(ctx, "container", target.ServerID.String(), environmentID.String())
	if err != nil {
		return nil, err
	}
	history, err := client.AttributedMetricHistory(ctx, "container", target.ServerID.String(), environmentID.String(), service.now().UTC().Add(-systemTelemetryHistoryWindow))
	if err != nil {
		return nil, err
	}
	return attributedTelemetryRows(current, history, service.now().UTC()), nil
}

func attributedTelemetryRows(current, history []clickhouseclient.AttributedMetricValue, now time.Time) []AttributedTelemetryRow {
	rows := make(map[string]*AttributedTelemetryRow)
	for _, value := range current {
		key := attributedIdentity(value)
		row := rows[key]
		if row == nil {
			row = &AttributedTelemetryRow{
				Scope: value.Scope, Component: value.Component, Application: value.Application,
				Environment: value.Environment, Release: value.Release, Deployment: value.Deployment,
				Target: value.Target, Instance: value.Instance, Resource: value.Resource,
				Installation: value.Installation, DatabaseCluster: value.DatabaseCluster,
				DatabaseClusterNode: value.DatabaseClusterNode, History: []AttributedTelemetryPoint{},
			}
			rows[key] = row
		}
		if value.ObservedAt.After(row.ObservedAt) {
			row.ObservedAt = value.ObservedAt
		}
		applyAttributedMetric(row, value.Metric, value.Value)
	}
	historyIndexes := make(map[string]int)
	for _, value := range history {
		key := attributedIdentity(value)
		row := rows[key]
		if row == nil {
			continue
		}
		pointKey := key + "\x00" + value.BucketStart.Format(time.RFC3339)
		index, exists := historyIndexes[pointKey]
		if !exists {
			row.History = append(row.History, AttributedTelemetryPoint{ObservedAt: value.BucketStart})
			index = len(row.History) - 1
			historyIndexes[pointKey] = index
		}
		applyAttributedPointMetric(&row.History[index], value.Metric, value.Value)
	}
	result := make([]AttributedTelemetryRow, 0, len(rows))
	for _, row := range rows {
		row.Available = !row.ObservedAt.IsZero() && now.Sub(row.ObservedAt) <= systemTelemetryFreshness
		slices.SortFunc(row.History, func(a, b AttributedTelemetryPoint) int { return a.ObservedAt.Compare(b.ObservedAt) })
		result = append(result, *row)
	}
	slices.SortFunc(result, func(a, b AttributedTelemetryRow) int {
		return strings.Compare(a.Component+a.Application+a.Instance+a.Installation, b.Component+b.Application+b.Instance+b.Installation)
	})
	return result
}

func attributedIdentity(value clickhouseclient.AttributedMetricValue) string {
	return strings.Join([]string{value.Scope, value.Component, value.Application, value.Environment, value.Release, value.Deployment, value.Target, value.Instance, value.Resource, value.Installation, value.DatabaseCluster, value.DatabaseClusterNode}, "\x00")
}

func applyAttributedMetric(row *AttributedTelemetryRow, metric string, value float64) {
	switch metric {
	case "cpu_cores_used":
		row.CPUCores = value
		row.CPUAvailable = true
	case "memory_working_set_bytes":
		row.MemoryBytes = value
		row.MemoryAvailable = true
	case "disk_read_bytes_per_second":
		row.DiskReadBytesPS = value
		row.DiskReadAvailable = true
	case "disk_write_bytes_per_second":
		row.DiskWriteBytesPS = value
		row.DiskWriteAvailable = true
	case "network_receive_bytes_per_second":
		row.NetworkReceiveBPS = value
		row.NetworkReceiveAvailable = true
	case "network_transmit_bytes_per_second":
		row.NetworkTransmitBPS = value
		row.NetworkTransmitAvailable = true
	case "oom_events":
		row.OOMEvents = value
		row.OOMAvailable = true
	case "cpu_throttling_ratio":
		row.CPUThrottlingRatio = clamp(value, 0, 1)
		row.CPUThrottlingAvailable = true
	case "tasks":
		row.Tasks = value
		row.TasksAvailable = true
	}
}

func applyAttributedPointMetric(point *AttributedTelemetryPoint, metric string, value float64) {
	switch metric {
	case "cpu_cores_used":
		point.CPUCores = value
		point.CPUAvailable = true
	case "memory_working_set_bytes":
		point.MemoryBytes = value
		point.MemoryAvailable = true
	case "disk_read_bytes_per_second":
		point.DiskReadBytesPS = value
		point.DiskReadAvailable = true
	case "disk_write_bytes_per_second":
		point.DiskWriteBytesPS = value
		point.DiskWriteAvailable = true
	case "network_receive_bytes_per_second":
		point.NetworkReceiveBPS = value
		point.NetworkReceiveAvailable = true
	case "network_transmit_bytes_per_second":
		point.NetworkTransmitBPS = value
		point.NetworkTransmitAvailable = true
	}
}

func systemResourceHistory(values []clickhouseclient.MetricHistoryValue, availableMetric, totalMetric string) []SystemTelemetryHistoryPoint {
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
			current.available, current.hasFree = value.Value, true
		} else {
			current.total, current.hasTotal = value.Value, true
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
		points = append(points, SystemTelemetryHistoryPoint{ObservedAt: current.observedAt, Used: current.total - free, Free: free})
	}
	return points
}

func systemThroughputHistory(values []clickhouseclient.MetricHistoryValue) []SystemThroughputHistoryPoint {
	pointsByBucket := make(map[int64]*SystemThroughputHistoryPoint)
	for _, value := range values {
		key := value.BucketStart.UnixMilli()
		point := pointsByBucket[key]
		if point == nil {
			point = &SystemThroughputHistoryPoint{ObservedAt: value.BucketStart}
			pointsByBucket[key] = point
		}
		switch value.Metric {
		case "cpu_cores_used":
			point.CPUCores, point.CPUAvailable = value.Value, true
		case "cpu_cores_total":
			point.CPUCoresTotal = value.Value
		case "disk_read_bytes_per_second":
			point.DiskReadBytesPS, point.DiskReadAvailable = value.Value, true
		case "disk_write_bytes_per_second":
			point.DiskWriteBytesPS, point.DiskWriteAvailable = value.Value, true
		case "network_receive_bytes_per_second":
			point.NetworkReceiveBPS, point.NetworkReceiveAvailable = value.Value, true
		case "network_transmit_bytes_per_second":
			point.NetworkTransmitBPS, point.NetworkTransmitAvailable = value.Value, true
		}
	}
	keys := make([]int64, 0, len(pointsByBucket))
	for key := range pointsByBucket {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	points := make([]SystemThroughputHistoryPoint, 0, len(keys))
	for _, key := range keys {
		points = append(points, *pointsByBucket[key])
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
