-- +goose NO TRANSACTION
-- +goose Up

CREATE TABLE deploycrate.metric_rollups
(
  bucket_start DateTime,
  observed_at DateTime64(3),
  scope LowCardinality(String),
  component LowCardinality(String),
  metric LowCardinality(String),
  average Float64,
  maximum Float64,
  last Float64,
  server String,
  application String,
  environment String,
  release String,
  deployment String,
  target String,
  instance String,
  resource String,
  installation String,
  runtime_id String,
  observation_id UUID
)
ENGINE = MergeTree
ORDER BY (scope, metric, server, environment, component, instance, installation, bucket_start, observation_id)
TTL bucket_start + INTERVAL 7 DAY DELETE;

-- +goose Down

DROP TABLE deploycrate.metric_rollups;
