-- +goose NO TRANSACTION
-- +goose Up

CREATE TABLE deploycrate.request_observations
(
  ObservedAt DateTime64(9) CODEC(Delta(8), ZSTD(1)),
  ProcessedAt DateTime64(9) CODEC(Delta(8), ZSTD(1)),
  Fingerprint UInt64,
  ApplicationID UUID,
  EnvironmentID UUID,
  Domain String CODEC(ZSTD(1)),
  Method LowCardinality(String) CODEC(ZSTD(1)),
  Path String CODEC(ZSTD(1)),
  StatusCode UInt16,
  CountryCode LowCardinality(String) CODEC(ZSTD(1)),
  DurationMS Float64
)
ENGINE = ReplacingMergeTree(ProcessedAt)
PARTITION BY toYYYYMM(ObservedAt)
ORDER BY (EnvironmentID, ObservedAt, Fingerprint)
TTL ObservedAt + INTERVAL 30 DAY DELETE
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

-- +goose Down

DROP TABLE deploycrate.request_observations;
