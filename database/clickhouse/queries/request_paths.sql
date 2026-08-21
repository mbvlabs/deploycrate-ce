SELECT
  Method AS method,
  Path AS path,
  count() AS requests,
  countIf(StatusCode >= 400) AS errors,
  quantile(0.95)(DurationMS) AS p95_duration_ms
FROM request_observations FINAL
WHERE EnvironmentID = toUUID({environment:String})
  AND ObservedAt >= toDateTime({since_seconds:UInt32})
GROUP BY Method, Path
ORDER BY requests DESC, Path ASC
LIMIT 100
FORMAT JSONEachRow
