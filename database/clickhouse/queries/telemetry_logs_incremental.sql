SELECT
  toString(toUnixTimestamp64Nano(Timestamp)) AS timestamp_nanoseconds,
  toString(sipHash64(SeverityText, Body, TraceId, SpanId, ScopeName, toString(LogAttributes), toString(ResourceAttributes))) AS fingerprint,
  Body AS message,
  SeverityText AS severity,
  SeverityNumber AS severity_number,
  LogAttributes AS attributes,
  TraceId AS trace_id,
  SpanId AS span_id,
  ScopeName AS scope,
  LogAttributes['code.file.path'] AS source,
  LogAttributes['code.line.number'] AS line,
  ResourceAttributes['service.instance.id'] AS instance,
  ResourceAttributes['deploycrate.slot'] AS slot,
  ServiceName AS service,
  ResourceAttributes['deploycrate.process.name'] AS process_name,
  ResourceAttributes['deploycrate.process.kind'] AS process_kind,
  ResourceAttributes['deploycrate.process.replica'] AS process_replica,
  coalesce(
    nullIf(LogAttributes['url.path'], ''),
    nullIf(LogAttributes['http.target'], ''),
    LogAttributes['http.route']
  ) AS request_path,
  toUInt16OrZero(coalesce(
    nullIf(LogAttributes['http.response.status_code'], ''),
    LogAttributes['http.status_code']
  )) AS response_code
FROM otel_logs
WHERE (
    ({scope:String} = 'system' AND ServiceName = {service:String} AND SeverityNumber >= 9)
    OR
    ({scope:String} = 'environment' AND ResourceAttributes['deploycrate.environment.id'] = {environment:String})
  )
  AND Timestamp >= fromUnixTimestamp64Nano({since_nanoseconds:Int64})
  AND (
    {response_class:UInt8} = 0
    OR intDiv(toUInt16OrZero(coalesce(
      nullIf(LogAttributes['http.response.status_code'], ''),
      LogAttributes['http.status_code']
    )), 100) = {response_class:UInt8}
  )
  AND (
    {search:String} = ''
    OR positionCaseInsensitiveUTF8(
      concat(Body, ' ', SeverityText, ' ', ScopeName, ' ', toString(LogAttributes), ' ', toString(ResourceAttributes), ' ', toString(TraceId), ' ', toString(SpanId)),
      {search:String}
    ) > 0
  )
  AND (
    Timestamp,
    sipHash64(SeverityText, Body, TraceId, SpanId, ScopeName, toString(LogAttributes), toString(ResourceAttributes))
  ) > (
    fromUnixTimestamp64Nano({after_nanoseconds:Int64}),
    {after_fingerprint:UInt64}
  )
ORDER BY
  Timestamp,
  sipHash64(SeverityText, Body, TraceId, SpanId, ScopeName, toString(LogAttributes), toString(ResourceAttributes))
LIMIT {limit:UInt64}
FORMAT JSONEachRow
