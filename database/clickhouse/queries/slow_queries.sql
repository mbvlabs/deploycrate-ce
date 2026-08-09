SELECT
  query,
  database_system,
  operation,
  toString(count()) AS executions,
  quantile(0.95)(Duration) / 1000000.0 AS p95_duration_milliseconds
FROM
(
  SELECT
    if(SpanAttributes['db.query.text'] != '', SpanAttributes['db.query.text'], SpanAttributes['db.statement']) AS query,
    if(SpanAttributes['db.system.name'] != '', SpanAttributes['db.system.name'], SpanAttributes['db.system']) AS database_system,
    if(SpanAttributes['db.operation.name'] != '', SpanAttributes['db.operation.name'], SpanAttributes['pgx.operation.type']) AS operation,
    Duration
  FROM otel_traces
  WHERE {{scope}}
    AND Timestamp >= toDateTime({since_seconds:UInt32})
)
WHERE query != ''
GROUP BY query, database_system, operation
ORDER BY p95_duration_milliseconds DESC, executions DESC, query
LIMIT {limit:UInt8}
FORMAT JSONEachRow
