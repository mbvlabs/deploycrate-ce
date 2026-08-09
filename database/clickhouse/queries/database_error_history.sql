SELECT
  toString(toUInt64(toUnixTimestamp(bucket_start)) * 1000) AS observed_at_milliseconds,
  sum(error_delta) AS errors
FROM
(
  SELECT
    toStartOfInterval(TimeUnix, toIntervalSecond({bucket_seconds:UInt32})) AS bucket_start,
    greatest(argMax(Value, TimeUnix) - argMin(Value, TimeUnix), 0) AS error_delta
  FROM otel_metrics_sum
  WHERE {{scope}}
    AND MetricName = 'db.client.operation.errors'
    AND TimeUnix >= toDateTime({since_seconds:UInt32})
    AND Attributes['pgx.operation.type'] IN ('query', 'batch')
  GROUP BY bucket_start, ResourceAttributes['service.instance.id'], cityHash64(Attributes)
  HAVING max(TimeUnix) > min(TimeUnix)
)
GROUP BY bucket_start
ORDER BY bucket_start
FORMAT JSONEachRow
