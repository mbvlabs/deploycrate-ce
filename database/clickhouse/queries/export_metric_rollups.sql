SELECT
  bucket_start,
  observed_at,
  scope,
  component,
  metric,
  average,
  maximum,
  `last`,
  server,
  application,
  environment,
  release,
  deployment,
  target,
  instance,
  resource,
  installation,
  runtime_id,
  observation_id
FROM metric_rollups
ORDER BY
  bucket_start,
  scope,
  metric,
  server,
  environment,
  component,
  instance,
  installation,
  runtime_id,
  observation_id
FORMAT JSONEachRow
