SELECT
  metric,
  sum(series_value) AS metric_value,
  toString(toInt64(toUnixTimestamp(max(observed_at))) * 1000) AS observed_at_milliseconds
FROM
(
  SELECT
    MetricName AS metric,
    ResourceAttributes['service.instance.id'] AS instance,
    cityHash64(Attributes) AS attributes,
    argMax(Value, TimeUnix) AS series_value,
    max(TimeUnix) AS observed_at
  FROM
  (
    SELECT ResourceAttributes, MetricName, Attributes, TimeUnix, Value
    FROM otel_metrics_gauge
    WHERE {{scope}} AND TimeUnix >= now() - INTERVAL 5 MINUTE
    UNION ALL
    SELECT ResourceAttributes, MetricName, Attributes, TimeUnix, Value
    FROM otel_metrics_sum
    WHERE {{scope}} AND TimeUnix >= now() - INTERVAL 5 MINUTE
  )
  WHERE MetricName IN (
    'go.memory.used',
    'go.memory.allocated',
    'go.memory.allocations',
    'go.memory.gc.goal',
    'go.goroutine.count'
  )
  GROUP BY metric, instance, attributes
)
GROUP BY metric
FORMAT JSONEachRow
