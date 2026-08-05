-- +goose NO TRANSACTION
-- +goose Up

CREATE TABLE deploycrate.otel_traces_trace_id_ts
(
  TraceId String CODEC(ZSTD(1)),
  Start DateTime CODEC(Delta, ZSTD(1)),
  End DateTime CODEC(Delta, ZSTD(1)),
  INDEX idx_trace_id TraceId TYPE bloom_filter(0.01) GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toDate(Start)
ORDER BY (TraceId, Start)
TTL Start + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

-- +goose Down

DROP TABLE deploycrate.otel_traces_trace_id_ts;
