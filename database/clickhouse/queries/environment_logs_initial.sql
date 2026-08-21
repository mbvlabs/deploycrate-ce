SELECT
  toString(toUnixTimestamp64Nano(Timestamp)) AS timestamp_nanoseconds,
  Body AS message,
  LogAttributes['log.iostream'] AS stream,
  LogAttributes['container.name'] AS container,
  if(coalesce(nullIf(LogAttributes['telemetry.source'], ''), ResourceAttributes['telemetry.source']) = 'journald', LogAttributes['deploycrate.deployment.id'], coalesce(nullIf(LogAttributes['deploycrate.deployment.id'], ''), ResourceAttributes['deploycrate.deployment.id'])) AS deployment,
  if(coalesce(nullIf(LogAttributes['telemetry.source'], ''), ResourceAttributes['telemetry.source']) = 'journald', LogAttributes['deploycrate.instance.id'], coalesce(nullIf(LogAttributes['deploycrate.instance.id'], ''), ResourceAttributes['deploycrate.instance.id'])) AS instance,
  if(coalesce(nullIf(LogAttributes['telemetry.source'], ''), ResourceAttributes['telemetry.source']) = 'journald', LogAttributes['deploycrate.release.id'], coalesce(nullIf(LogAttributes['deploycrate.release.id'], ''), ResourceAttributes['deploycrate.release.id'])) AS release,
  if(coalesce(nullIf(LogAttributes['telemetry.source'], ''), ResourceAttributes['telemetry.source']) = 'journald', LogAttributes['deploycrate.process.name'], coalesce(nullIf(LogAttributes['deploycrate.process.name'], ''), ResourceAttributes['deploycrate.process.name'])) AS process_name,
  if(coalesce(nullIf(LogAttributes['telemetry.source'], ''), ResourceAttributes['telemetry.source']) = 'journald', LogAttributes['deploycrate.process.kind'], coalesce(nullIf(LogAttributes['deploycrate.process.kind'], ''), ResourceAttributes['deploycrate.process.kind'])) AS process_kind,
  if(coalesce(nullIf(LogAttributes['telemetry.source'], ''), ResourceAttributes['telemetry.source']) = 'journald', LogAttributes['deploycrate.process.replica'], coalesce(nullIf(LogAttributes['deploycrate.process.replica'], ''), ResourceAttributes['deploycrate.process.replica'])) AS process_replica,
  LogAttributes['deploycrate.log.epoch'] AS epoch,
  LogAttributes['deploycrate.log.ordinal'] AS ordinal
FROM otel_logs
WHERE LogAttributes['deploycrate.environment.id'] = {environment:String}
  OR (
    coalesce(nullIf(LogAttributes['telemetry.source'], ''), ResourceAttributes['telemetry.source']) != 'journald'
    AND ResourceAttributes['deploycrate.environment.id'] = {environment:String}
  )
ORDER BY
  Timestamp DESC,
  epoch DESC,
  toUInt64OrZero(LogAttributes['deploycrate.log.ordinal']) DESC
LIMIT {limit:UInt64}
FORMAT JSONEachRow
