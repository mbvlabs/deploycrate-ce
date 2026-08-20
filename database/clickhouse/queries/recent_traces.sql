SELECT
  TraceId AS trace_id,
  argMaxIf(SpanName, Duration, ParentSpanId = '') AS root_span_name,
  argMaxIf(
    coalesce(
      nullIf(SpanAttributes['http.request.method'], ''),
      SpanAttributes['http.method']
    ),
    Timestamp,
    coalesce(
      nullIf(SpanAttributes['http.request.method'], ''),
      SpanAttributes['http.method']
    ) != ''
    AND (
      {response_class:UInt8} = 0
      OR intDiv(toUInt16OrZero(coalesce(
        nullIf(SpanAttributes['http.response.status_code'], ''),
        SpanAttributes['http.status_code']
      )), 100) = {response_class:UInt8}
    )
  ) AS request_method,
  argMaxIf(
    coalesce(
      nullIf(SpanAttributes['http.route'], ''),
      nullIf(SpanAttributes['url.path'], ''),
      SpanAttributes['http.target']
    ),
    Timestamp,
    coalesce(
      nullIf(SpanAttributes['http.route'], ''),
      nullIf(SpanAttributes['url.path'], ''),
      SpanAttributes['http.target']
    ) != ''
    AND (
      {response_class:UInt8} = 0
      OR intDiv(toUInt16OrZero(coalesce(
        nullIf(SpanAttributes['http.response.status_code'], ''),
        SpanAttributes['http.status_code']
      )), 100) = {response_class:UInt8}
    )
  ) AS request_route,
  toUInt16OrZero(argMaxIf(
    coalesce(
      nullIf(SpanAttributes['http.response.status_code'], ''),
      SpanAttributes['http.status_code']
    ),
    Timestamp,
    coalesce(
      nullIf(SpanAttributes['http.response.status_code'], ''),
      SpanAttributes['http.status_code']
    ) != ''
    AND (
      {response_class:UInt8} = 0
      OR intDiv(toUInt16OrZero(coalesce(
        nullIf(SpanAttributes['http.response.status_code'], ''),
        SpanAttributes['http.status_code']
      )), 100) = {response_class:UInt8}
    )
  )) AS response_code,
  toString(toUnixTimestamp64Nano(min(Timestamp))) AS timestamp_nanoseconds,
  toString(if(countIf(ParentSpanId = '') > 0, argMaxIf(Duration, Duration, ParentSpanId = ''), max(Duration))) AS duration_nanoseconds,
  toString(count()) AS span_count,
  toString(countIf(lower(StatusCode) = 'error')) AS error_count
FROM otel_traces
WHERE {{scope}}
  AND Timestamp >= toDateTime({since_seconds:UInt32})
GROUP BY TraceId
HAVING countIf(SpanAttributes['http.route'] = '/api/health') = 0
  AND (
    {response_class:UInt8} = 0
    OR countIf(intDiv(toUInt16OrZero(coalesce(
      nullIf(SpanAttributes['http.response.status_code'], ''),
      SpanAttributes['http.status_code']
    )), 100) = {response_class:UInt8}) > 0
  )
ORDER BY min(Timestamp) DESC
LIMIT 100
FORMAT JSONEachRow
