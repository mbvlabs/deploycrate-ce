package clickhouse

import (
	"embed"
	"fmt"
)

//go:embed queries/*.sql
var queryFiles embed.FS

var (
	applicationMetricHistoryQuery   = mustQuery("application_metric_history")
	applicationRequestOverviewQuery = mustQuery("application_request_overview")
	applicationRuntimeOverviewQuery = mustQuery("application_runtime_overview")
	attributedMetricHistoryQuery    = mustQuery("attributed_metric_history")
	databaseErrorHistoryQuery       = mustQuery("database_error_history")
	environmentLogsIncrementalQuery = mustQuery("environment_logs_incremental")
	environmentLogsInitialQuery     = mustQuery("environment_logs_initial")
	exportMetricRollupsQuery        = mustQuery("export_metric_rollups")
	insertMetricRollupsQuery        = mustQuery("insert_metric_rollups")
	latestAttributedMetricsQuery    = mustQuery("latest_attributed_metrics")
	latestSystemMetricsQuery        = mustQuery("latest_system_metrics")
	recentTracesQuery               = mustQuery("recent_traces")
	requestGeographyQuery           = mustQuery("request_geography")
	slowQueriesQuery                = mustQuery("slow_queries")
	systemMetricHistoryQuery        = mustQuery("system_metric_history")
	telemetryLogsIncrementalQuery   = mustQuery("telemetry_logs_incremental")
	telemetryLogsInitialQuery       = mustQuery("telemetry_logs_initial")
	traceQuery                      = mustQuery("trace")
)

func mustQuery(name string) string {
	statement, err := queryFiles.ReadFile("queries/" + name + ".sql")
	if err != nil {
		panic(fmt.Sprintf("load ClickHouse query %q: %v", name, err))
	}
	return string(statement)
}
