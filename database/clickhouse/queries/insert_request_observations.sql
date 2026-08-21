INSERT INTO request_observations
(
  ObservedAt,
  ProcessedAt,
  Fingerprint,
  ApplicationID,
  EnvironmentID,
  Domain,
  Method,
  Path,
  StatusCode,
  CountryCode,
  DurationMS
)
FORMAT JSONEachRow
