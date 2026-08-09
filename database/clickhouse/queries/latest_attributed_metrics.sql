SELECT
  scope,
  component,
  application,
  environment,
  release,
  deployment,
  target,
  instance,
  resource,
  installation,
  argMax(runtime_id, observed_at) AS runtime_id,
  metric,
  argMax(`last`, observed_at) AS value,
  toString(toUnixTimestamp64Milli(max(observed_at))) AS observed_at_milliseconds
FROM metric_rollups
WHERE scope = {scope:String}
  AND server = {server:String}
  AND ({environment:String} = '' OR environment = {environment:String})
GROUP BY
  scope,
  component,
  application,
  environment,
  release,
  deployment,
  target,
  instance,
  resource,
  installation,
  metric
ORDER BY component, application, environment, instance, installation, metric
FORMAT JSONEachRow
