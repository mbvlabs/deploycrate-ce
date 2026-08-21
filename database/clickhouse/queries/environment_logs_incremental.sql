SELECT
  toString(toUnixTimestamp64Nano(Timestamp)) AS timestamp_nanoseconds,
  Body AS message,
  LogAttributes['log.iostream'] AS stream,
  LogAttributes['container.name'] AS container,
  ResourceAttributes['deploycrate.deployment.id'] AS deployment,
  ResourceAttributes['deploycrate.instance.id'] AS instance,
  ResourceAttributes['deploycrate.release.id'] AS release,
  ResourceAttributes['deploycrate.process.name'] AS process_name,
  ResourceAttributes['deploycrate.process.kind'] AS process_kind,
  ResourceAttributes['deploycrate.process.replica'] AS process_replica,
  LogAttributes['deploycrate.log.epoch'] AS epoch,
  LogAttributes['deploycrate.log.ordinal'] AS ordinal
FROM otel_logs
WHERE ResourceAttributes['deploycrate.environment.id'] = {environment:String}
  AND (
    Timestamp,
    LogAttributes['deploycrate.log.epoch'],
    toUInt64OrZero(LogAttributes['deploycrate.log.ordinal'])
  ) > (
    fromUnixTimestamp64Nano({after_nanoseconds:Int64}),
    {after_epoch:String},
    {after_ordinal:UInt64}
  )
ORDER BY
  Timestamp,
  epoch,
  toUInt64OrZero(LogAttributes['deploycrate.log.ordinal'])
LIMIT {limit:UInt64}
FORMAT JSONEachRow
