SELECT
  toString(toUnixTimestamp64Nano(Timestamp)) AS timestamp_nanoseconds,
  TraceId AS trace_id,
  SpanId AS span_id,
  ParentSpanId AS parent_span_id,
  SpanName AS name,
  SpanKind AS kind,
  ServiceName AS service_name,
  ScopeName AS scope,
  StatusCode AS status_code,
  StatusMessage AS status_message,
  ResourceAttributes AS resource_attributes,
  SpanAttributes AS span_attributes,
  toString(Duration) AS duration_nanoseconds
FROM otel_traces
WHERE TraceId = {trace_id:String}
  AND (
    {environment:String} = ''
    OR TraceId IN (
      SELECT TraceId
      FROM otel_traces
      WHERE TraceId = {trace_id:String}
        AND ResourceAttributes['deploycrate.environment.id'] = {environment:String}
    )
  )
ORDER BY Timestamp, Duration DESC
LIMIT 1000
FORMAT JSONEachRow
