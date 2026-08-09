SELECT
  toString(toInt64(toUnixTimestamp(max(last_time))) * 1000) AS observed_at_milliseconds,
  dateDiff('millisecond', min(first_time), max(last_time)) / 1000.0 AS window_seconds,
  sum(request_delta) AS request_count,
  sumIf(request_delta, status >= 400 AND status < 500) AS client_errors,
  sumIf(request_delta, status >= 500) AS server_errors,
  sum(duration_delta) AS duration_total
FROM
(
  SELECT
    greatest(toFloat64(argMax(Count, TimeUnix)) - toFloat64(argMin(Count, TimeUnix)), 0) AS request_delta,
    greatest(argMax(Sum, TimeUnix) - argMin(Sum, TimeUnix), 0) AS duration_delta,
    toUInt16OrZero(argMax(Attributes['http.response.status_code'], TimeUnix)) AS status,
    min(TimeUnix) AS first_time,
    max(TimeUnix) AS last_time
  FROM otel_metrics_histogram
  WHERE {{scope}}
    AND MetricName = 'http.server.request.duration'
    AND Attributes['http.route'] != '/api/health'
    AND TimeUnix >= now() - INTERVAL 5 MINUTE
  GROUP BY ResourceAttributes['service.instance.id'], cityHash64(Attributes)
  HAVING last_time > first_time
)
FORMAT JSONEachRow
