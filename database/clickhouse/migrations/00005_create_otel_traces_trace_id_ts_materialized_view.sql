-- +goose NO TRANSACTION
-- +goose Up

CREATE MATERIALIZED VIEW deploycrate.otel_traces_trace_id_ts_mv
TO deploycrate.otel_traces_trace_id_ts
AS SELECT
  TraceId,
  min(Timestamp) AS Start,
  max(Timestamp) AS End
FROM deploycrate.otel_traces
WHERE TraceId != ''
GROUP BY TraceId;

-- +goose Down

DROP TABLE deploycrate.otel_traces_trace_id_ts_mv;
