SELECT
  TraceId AS trace_id,
  argMaxIf(SpanName, Duration, ParentSpanId = '') AS root_span_name,
  toString(toUnixTimestamp64Nano(min(Timestamp))) AS timestamp_nanoseconds,
  toString(if(countIf(ParentSpanId = '') > 0, argMaxIf(Duration, Duration, ParentSpanId = ''), max(Duration))) AS duration_nanoseconds,
  toString(count()) AS span_count,
  toString(countIf(lower(StatusCode) = 'error')) AS error_count
FROM otel_traces
WHERE {{scope}}
  AND Timestamp >= toDateTime({since_seconds:UInt32})
GROUP BY TraceId
HAVING countIf(SpanAttributes['http.route'] = '/api/health') = 0
ORDER BY min(Timestamp) DESC
LIMIT 100
FORMAT JSONEachRow
