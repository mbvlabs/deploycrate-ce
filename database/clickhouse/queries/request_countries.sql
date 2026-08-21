SELECT
  CountryCode AS code,
  count() AS requests
FROM request_observations FINAL
WHERE EnvironmentID = toUUID({environment:String})
  AND ObservedAt >= toDateTime({since_seconds:UInt32})
  AND CountryCode != ''
GROUP BY CountryCode
ORDER BY requests DESC, code ASC
FORMAT JSONEachRow
