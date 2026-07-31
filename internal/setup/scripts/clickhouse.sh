#!/usr/bin/env bash
set -euo pipefail

: "${CLICKHOUSE_PASSWORD:?CLICKHOUSE_PASSWORD is required}"

readonly image="clickhouse/clickhouse-server:25.8.28.1"
readonly container="deploycrate-ce-clickhouse"
readonly volume="deploycrate-ce-clickhouse"
readonly metrics_config="/etc/deploycrate-ce/clickhouse/prometheus.xml"

install -d -o root -g root -m 0755 /etc/deploycrate-ce/clickhouse
cat > "${metrics_config}" <<'EOF'
<clickhouse>
  <prometheus>
    <endpoint>/metrics</endpoint>
    <port>9363</port>
    <metrics>true</metrics>
    <events>true</events>
    <asynchronous_metrics>true</asynchronous_metrics>
    <status_info>true</status_info>
  </prometheus>
</clickhouse>
EOF
chmod 0644 "${metrics_config}"

docker volume create "${volume}" >/dev/null
if docker container inspect "${container}" >/dev/null 2>&1; then
  component="$(docker inspect --format '{{ index .Config.Labels "com.deploycrate.component" }}' "${container}")"
  logging_driver="$(docker inspect --format '{{.HostConfig.LogConfig.Type}}' "${container}")"
  published_http="$(docker inspect --format '{{with (index .HostConfig.PortBindings "8123/tcp")}}{{(index . 0).HostIp}}:{{(index . 0).HostPort}}{{end}}' "${container}")"
  published_native="$(docker inspect --format '{{with (index .HostConfig.PortBindings "9000/tcp")}}{{(index . 0).HostIp}}:{{(index . 0).HostPort}}{{end}}' "${container}")"
  published_metrics="$(docker inspect --format '{{with (index .HostConfig.PortBindings "9363/tcp")}}{{(index . 0).HostIp}}:{{(index . 0).HostPort}}{{end}}' "${container}")"
  mounted_metrics="$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/etc/clickhouse-server/config.d/prometheus.xml"}}{{.Source}}{{end}}{{end}}' "${container}")"
  if [ "${component}" != clickhouse ] || [ "${logging_driver}" != journald ] || \
    [ "${published_http}" != 127.0.0.1:8123 ] || [ "${published_native}" != 127.0.0.1:9000 ] || \
    [ "${published_metrics}" != 127.0.0.1:9363 ] || [ "${mounted_metrics}" != "${metrics_config}" ]; then
    docker rm --force "${container}" >/dev/null
  fi
fi
if docker container inspect "${container}" >/dev/null 2>&1; then
  docker start "${container}" >/dev/null
else
  docker run --detach \
    --name "${container}" \
    --label com.deploycrate.component=clickhouse \
    --restart unless-stopped \
    --publish 127.0.0.1:8123:8123 \
    --publish 127.0.0.1:9000:9000 \
    --publish 127.0.0.1:9363:9363 \
    --env CLICKHOUSE_USER=deploycrate \
    --env "CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD}" \
    --volume "${volume}:/var/lib/clickhouse" \
    --volume "${metrics_config}:/etc/clickhouse-server/config.d/prometheus.xml:ro" \
    "${image}" >/dev/null
fi

clickhouse_diagnostics() {
  printf 'ClickHouse did not become ready for schema initialization\n' >&2
  docker inspect --format \
    'status={{.State.Status}} running={{.State.Running}} exit_code={{.State.ExitCode}} error={{json .State.Error}} restart_count={{.RestartCount}}' \
    "${container}" >&2 || true
  docker logs --tail 200 "${container}" >&2 || true
  ss -lntp >&2 || true
}

for attempt in $(seq 1 60); do
  if docker exec "${container}" clickhouse-client \
    --user deploycrate --password "${CLICKHOUSE_PASSWORD}" --query 'SELECT 1' >/dev/null 2>&1; then
    break
  fi
  if [ "$(docker inspect --format '{{.State.Running}}' "${container}" 2>/dev/null)" != true ]; then
    clickhouse_diagnostics
    exit 1
  fi
  [ "${attempt}" -lt 60 ] || {
    clickhouse_diagnostics
    exit 1
  }
  sleep 1
done

if ! docker exec "${container}" clickhouse-client \
  --user deploycrate --password "${CLICKHOUSE_PASSWORD}" \
  --query 'CREATE DATABASE IF NOT EXISTS deploycrate' >/dev/null; then
  clickhouse_diagnostics
  exit 1
fi

metric_rollup_scope_column="$(docker exec "${container}" clickhouse-client \
  --user deploycrate --password "${CLICKHOUSE_PASSWORD}" \
  --query "SELECT count() FROM system.columns WHERE database = 'deploycrate' AND table = 'metric_rollups' AND name = 'scope'")"
if [ "${metric_rollup_scope_column}" != 1 ]; then
  docker exec "${container}" clickhouse-client \
    --user deploycrate --password "${CLICKHOUSE_PASSWORD}" \
    --query 'DROP TABLE IF EXISTS deploycrate.metric_rollups' >/dev/null
fi
docker exec "${container}" clickhouse-client \
  --user deploycrate --password "${CLICKHOUSE_PASSWORD}" \
  --query 'DROP TABLE IF EXISTS deploycrate.metric_rollups_v2' >/dev/null

if ! docker exec --interactive "${container}" clickhouse-client \
  --user deploycrate --password "${CLICKHOUSE_PASSWORD}" --multiquery >/dev/null <<'SQL'
CREATE TABLE IF NOT EXISTS deploycrate.metric_rollups
(
  bucket_start DateTime,
  observed_at DateTime64(3),
  scope LowCardinality(String),
  component LowCardinality(String),
  metric LowCardinality(String),
  average Float64,
  maximum Float64,
  last Float64,
  server String,
  application String,
  environment String,
  release String,
  deployment String,
  target String,
  instance String,
  resource String,
  installation String,
  runtime_id String,
  observation_id UUID
)
ENGINE = MergeTree
ORDER BY (scope, metric, server, environment, component, instance, installation, bucket_start, observation_id)
TTL bucket_start + INTERVAL 7 DAY DELETE;

CREATE TABLE IF NOT EXISTS deploycrate.otel_logs
(
  Timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
  TraceId String CODEC(ZSTD(1)),
  SpanId String CODEC(ZSTD(1)),
  TraceFlags UInt8,
  SeverityText LowCardinality(String) CODEC(ZSTD(1)),
  SeverityNumber UInt8,
  ServiceName LowCardinality(String) CODEC(ZSTD(1)),
  Body String CODEC(ZSTD(1)),
  ResourceSchemaUrl LowCardinality(String) CODEC(ZSTD(1)),
  ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  ScopeSchemaUrl LowCardinality(String) CODEC(ZSTD(1)),
  ScopeName String CODEC(ZSTD(1)),
  ScopeVersion LowCardinality(String) CODEC(ZSTD(1)),
  ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  LogAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  EventName String CODEC(ZSTD(1)),
  INDEX idx_trace_id TraceId TYPE bloom_filter(0.001) GRANULARITY 1,
  INDEX idx_res_attr_key mapKeys(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_res_attr_value mapValues(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_scope_attr_key mapKeys(ScopeAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_scope_attr_value mapValues(ScopeAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_log_attr_key mapKeys(LogAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_log_attr_value mapValues(LogAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_lower_body lower(Body) TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 8
)
ENGINE = MergeTree
PARTITION BY toDate(Timestamp)
ORDER BY (toStartOfFiveMinutes(Timestamp), ServiceName, Timestamp)
TTL Timestamp + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE TABLE IF NOT EXISTS deploycrate.otel_traces
(
  Timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
  TraceId String CODEC(ZSTD(1)),
  SpanId String CODEC(ZSTD(1)),
  ParentSpanId String CODEC(ZSTD(1)),
  TraceState String CODEC(ZSTD(1)),
  SpanName LowCardinality(String) CODEC(ZSTD(1)),
  SpanKind LowCardinality(String) CODEC(ZSTD(1)),
  ServiceName LowCardinality(String) CODEC(ZSTD(1)),
  ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  ScopeName String CODEC(ZSTD(1)),
  ScopeVersion String CODEC(ZSTD(1)),
  SpanAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  Duration UInt64 CODEC(ZSTD(1)),
  StatusCode LowCardinality(String) CODEC(ZSTD(1)),
  StatusMessage String CODEC(ZSTD(1)),
  Events Nested
  (
    Timestamp DateTime64(9),
    Name LowCardinality(String),
    Attributes Map(LowCardinality(String), String)
  ) CODEC(ZSTD(1)),
  Links Nested
  (
    TraceId String,
    SpanId String,
    TraceState String,
    Attributes Map(LowCardinality(String), String)
  ) CODEC(ZSTD(1)),
  INDEX idx_trace_id TraceId TYPE bloom_filter(0.001) GRANULARITY 1,
  INDEX idx_res_attr_key mapKeys(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_res_attr_value mapValues(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_span_attr_key mapKeys(SpanAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_span_attr_value mapValues(SpanAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_duration Duration TYPE minmax GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, SpanName, toDateTime(Timestamp))
TTL Timestamp + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE TABLE IF NOT EXISTS deploycrate.otel_traces_trace_id_ts
(
  TraceId String CODEC(ZSTD(1)),
  Start DateTime CODEC(Delta, ZSTD(1)),
  End DateTime CODEC(Delta, ZSTD(1)),
  INDEX idx_trace_id TraceId TYPE bloom_filter(0.01) GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toDate(Start)
ORDER BY (TraceId, Start)
TTL Start + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE MATERIALIZED VIEW IF NOT EXISTS deploycrate.otel_traces_trace_id_ts_mv
TO deploycrate.otel_traces_trace_id_ts
AS SELECT
  TraceId,
  min(Timestamp) AS Start,
  max(Timestamp) AS End
FROM deploycrate.otel_traces
WHERE TraceId != ''
GROUP BY TraceId;

CREATE TABLE IF NOT EXISTS deploycrate.otel_metrics_gauge
(
  ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  ResourceSchemaUrl String CODEC(ZSTD(1)),
  ScopeName String CODEC(ZSTD(1)),
  ScopeVersion String CODEC(ZSTD(1)),
  ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  ScopeDroppedAttrCount UInt32 CODEC(ZSTD(1)),
  ScopeSchemaUrl String CODEC(ZSTD(1)),
  ServiceName LowCardinality(String) CODEC(ZSTD(1)),
  MetricName LowCardinality(String) CODEC(ZSTD(1)),
  MetricDescription String CODEC(ZSTD(1)),
  MetricUnit String CODEC(ZSTD(1)),
  Attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  StartTimeUnix DateTime CODEC(Delta, ZSTD(1)),
  TimeUnix DateTime CODEC(Delta, ZSTD(1)),
  Value Float64 CODEC(ZSTD(1)),
  Flags UInt32 CODEC(ZSTD(1)),
  Exemplars Nested
  (
    FilteredAttributes Map(LowCardinality(String), String),
    TimeUnix DateTime,
    Value Float64,
    SpanId String,
    TraceId String
  ) CODEC(ZSTD(1)),
  INDEX idx_res_attr_key mapKeys(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_res_attr_value mapValues(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_scope_attr_key mapKeys(ScopeAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_scope_attr_value mapValues(ScopeAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_attr_key mapKeys(Attributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_attr_value mapValues(Attributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_time_minmax TimeUnix TYPE minmax GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toDate(TimeUnix)
ORDER BY (ServiceName, MetricName, toStartOfHour(TimeUnix), cityHash64(Attributes), TimeUnix)
TTL TimeUnix + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE TABLE IF NOT EXISTS deploycrate.otel_metrics_sum
(
  ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  ResourceSchemaUrl String CODEC(ZSTD(1)),
  ScopeName String CODEC(ZSTD(1)),
  ScopeVersion String CODEC(ZSTD(1)),
  ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  ScopeDroppedAttrCount UInt32 CODEC(ZSTD(1)),
  ScopeSchemaUrl String CODEC(ZSTD(1)),
  ServiceName LowCardinality(String) CODEC(ZSTD(1)),
  MetricName LowCardinality(String) CODEC(ZSTD(1)),
  MetricDescription String CODEC(ZSTD(1)),
  MetricUnit String CODEC(ZSTD(1)),
  Attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  StartTimeUnix DateTime CODEC(Delta, ZSTD(1)),
  TimeUnix DateTime CODEC(Delta, ZSTD(1)),
  Value Float64 CODEC(ZSTD(1)),
  Flags UInt32 CODEC(ZSTD(1)),
  Exemplars Nested
  (
    FilteredAttributes Map(LowCardinality(String), String),
    TimeUnix DateTime,
    Value Float64,
    SpanId String,
    TraceId String
  ) CODEC(ZSTD(1)),
  AggregationTemporality Int32 CODEC(ZSTD(1)),
  IsMonotonic Boolean CODEC(Delta, ZSTD(1)),
  INDEX idx_res_attr_key mapKeys(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_res_attr_value mapValues(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_scope_attr_key mapKeys(ScopeAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_scope_attr_value mapValues(ScopeAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_attr_key mapKeys(Attributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_attr_value mapValues(Attributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_time_minmax TimeUnix TYPE minmax GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toDate(TimeUnix)
ORDER BY (ServiceName, MetricName, toStartOfHour(TimeUnix), cityHash64(Attributes), TimeUnix)
TTL TimeUnix + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE TABLE IF NOT EXISTS deploycrate.otel_metrics_histogram
(
  ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  ResourceSchemaUrl String CODEC(ZSTD(1)),
  ScopeName String CODEC(ZSTD(1)),
  ScopeVersion String CODEC(ZSTD(1)),
  ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  ScopeDroppedAttrCount UInt32 CODEC(ZSTD(1)),
  ScopeSchemaUrl String CODEC(ZSTD(1)),
  ServiceName LowCardinality(String) CODEC(ZSTD(1)),
  MetricName LowCardinality(String) CODEC(ZSTD(1)),
  MetricDescription String CODEC(ZSTD(1)),
  MetricUnit String CODEC(ZSTD(1)),
  Attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  StartTimeUnix DateTime CODEC(Delta, ZSTD(1)),
  TimeUnix DateTime CODEC(Delta, ZSTD(1)),
  Count UInt64 CODEC(Delta, ZSTD(1)),
  Sum Float64 CODEC(ZSTD(1)),
  BucketCounts Array(UInt64) CODEC(ZSTD(1)),
  ExplicitBounds Array(Float64) CODEC(ZSTD(1)),
  Exemplars Nested
  (
    FilteredAttributes Map(LowCardinality(String), String),
    TimeUnix DateTime,
    Value Float64,
    SpanId String,
    TraceId String
  ) CODEC(ZSTD(1)),
  Flags UInt32 CODEC(ZSTD(1)),
  Min Float64 CODEC(ZSTD(1)),
  Max Float64 CODEC(ZSTD(1)),
  AggregationTemporality Int32 CODEC(ZSTD(1)),
  INDEX idx_res_attr_key mapKeys(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_res_attr_value mapValues(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_scope_attr_key mapKeys(ScopeAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_scope_attr_value mapValues(ScopeAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_attr_key mapKeys(Attributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_attr_value mapValues(Attributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_time_minmax TimeUnix TYPE minmax GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toDate(TimeUnix)
ORDER BY (ServiceName, MetricName, toStartOfHour(TimeUnix), cityHash64(Attributes), TimeUnix)
TTL TimeUnix + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE TABLE IF NOT EXISTS deploycrate.otel_metrics_exp_histogram
(
  ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  ResourceSchemaUrl String CODEC(ZSTD(1)),
  ScopeName String CODEC(ZSTD(1)),
  ScopeVersion String CODEC(ZSTD(1)),
  ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  ScopeDroppedAttrCount UInt32 CODEC(ZSTD(1)),
  ScopeSchemaUrl String CODEC(ZSTD(1)),
  ServiceName LowCardinality(String) CODEC(ZSTD(1)),
  MetricName LowCardinality(String) CODEC(ZSTD(1)),
  MetricDescription String CODEC(ZSTD(1)),
  MetricUnit String CODEC(ZSTD(1)),
  Attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  StartTimeUnix DateTime CODEC(Delta, ZSTD(1)),
  TimeUnix DateTime CODEC(Delta, ZSTD(1)),
  Count UInt64 CODEC(Delta, ZSTD(1)),
  Sum Float64 CODEC(ZSTD(1)),
  Scale Int32 CODEC(ZSTD(1)),
  ZeroCount UInt64 CODEC(ZSTD(1)),
  PositiveOffset Int32 CODEC(ZSTD(1)),
  PositiveBucketCounts Array(UInt64) CODEC(ZSTD(1)),
  NegativeOffset Int32 CODEC(ZSTD(1)),
  NegativeBucketCounts Array(UInt64) CODEC(ZSTD(1)),
  Exemplars Nested
  (
    FilteredAttributes Map(LowCardinality(String), String),
    TimeUnix DateTime,
    Value Float64,
    SpanId String,
    TraceId String
  ) CODEC(ZSTD(1)),
  Flags UInt32 CODEC(ZSTD(1)),
  Min Float64 CODEC(ZSTD(1)),
  Max Float64 CODEC(ZSTD(1)),
  AggregationTemporality Int32 CODEC(ZSTD(1)),
  INDEX idx_res_attr_key mapKeys(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_res_attr_value mapValues(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_scope_attr_key mapKeys(ScopeAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_scope_attr_value mapValues(ScopeAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_attr_key mapKeys(Attributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_attr_value mapValues(Attributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_time_minmax TimeUnix TYPE minmax GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toDate(TimeUnix)
ORDER BY (ServiceName, MetricName, toStartOfHour(TimeUnix), cityHash64(Attributes), TimeUnix)
TTL TimeUnix + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE TABLE IF NOT EXISTS deploycrate.otel_metrics_summary
(
  ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  ResourceSchemaUrl String CODEC(ZSTD(1)),
  ScopeName String CODEC(ZSTD(1)),
  ScopeVersion String CODEC(ZSTD(1)),
  ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  ScopeDroppedAttrCount UInt32 CODEC(ZSTD(1)),
  ScopeSchemaUrl String CODEC(ZSTD(1)),
  ServiceName LowCardinality(String) CODEC(ZSTD(1)),
  MetricName LowCardinality(String) CODEC(ZSTD(1)),
  MetricDescription String CODEC(ZSTD(1)),
  MetricUnit String CODEC(ZSTD(1)),
  Attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
  StartTimeUnix DateTime CODEC(Delta, ZSTD(1)),
  TimeUnix DateTime CODEC(Delta, ZSTD(1)),
  Count UInt64 CODEC(Delta, ZSTD(1)),
  Sum Float64 CODEC(ZSTD(1)),
  ValueAtQuantiles Nested
  (
    Quantile Float64,
    Value Float64
  ) CODEC(ZSTD(1)),
  Flags UInt32 CODEC(ZSTD(1)),
  INDEX idx_res_attr_key mapKeys(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_res_attr_value mapValues(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_scope_attr_key mapKeys(ScopeAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_scope_attr_value mapValues(ScopeAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_attr_key mapKeys(Attributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_attr_value mapValues(Attributes) TYPE bloom_filter(0.01) GRANULARITY 1,
  INDEX idx_time_minmax TimeUnix TYPE minmax GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toDate(TimeUnix)
ORDER BY (ServiceName, MetricName, toStartOfHour(TimeUnix), cityHash64(Attributes), TimeUnix)
TTL TimeUnix + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;
SQL
then
  clickhouse_diagnostics
  exit 1
fi
