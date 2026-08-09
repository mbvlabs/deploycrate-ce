SELECT
  toString(toUInt64(toUnixTimestamp(bucket_start)) * 1000) AS observed_at_milliseconds,
  metric,
  status,
  operation,
  route,
  method,
  toString(count_delta) AS count_delta_value,
  sum_delta,
  bucket_counts,
  explicit_bounds,
  maximum
FROM
(
  SELECT
    toStartOfInterval(TimeUnix, toIntervalSecond({bucket_seconds:UInt32})) AS bucket_start,
    MetricName AS metric,
    toUInt16OrZero(Attributes['http.response.status_code']) AS status,
    Attributes['pgx.operation.type'] AS operation,
    coalesce(nullIf(Attributes['http.route'], ''), nullIf(Attributes['url.path'], ''), Attributes['http.target']) AS route,
    coalesce(nullIf(Attributes['http.request.method'], ''), Attributes['http.method']) AS method,
    greatest(toInt64(argMax(Count, TimeUnix)) - toInt64(argMin(Count, TimeUnix)), 0) AS count_delta,
    greatest(argMax(Sum, TimeUnix) - argMin(Sum, TimeUnix), 0) AS sum_delta,
    arrayMap(
      (latest, earliest) -> toUInt64(if(
        latest >= earliest,
        toInt64(latest) - toInt64(earliest),
        toInt64(latest)
      )),
      argMax(BucketCounts, TimeUnix),
      argMin(BucketCounts, TimeUnix)
    ) AS bucket_counts,
    argMax(ExplicitBounds, TimeUnix) AS explicit_bounds,
    max(Max) AS maximum
  FROM otel_metrics_histogram
  WHERE {{scope}}
    AND MetricName IN ('http.server.request.duration', 'db.client.operation.duration')
    AND TimeUnix >= toDateTime({since_seconds:UInt32})
    AND (MetricName != 'http.server.request.duration' OR Attributes['http.route'] != '/api/health')
    AND (MetricName != 'db.client.operation.duration' OR Attributes['pgx.operation.type'] IN ('query', 'batch'))
  GROUP BY
    bucket_start,
    metric,
    ResourceAttributes['service.instance.id'],
    cityHash64(Attributes),
    status,
    operation,
    route,
    method
  HAVING max(TimeUnix) > min(TimeUnix)
)
WHERE count_delta > 0
ORDER BY bucket_start, metric, status, operation
FORMAT JSONEachRow
