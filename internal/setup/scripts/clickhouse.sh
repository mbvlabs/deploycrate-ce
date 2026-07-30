#!/usr/bin/env bash
set -euo pipefail

: "${CLICKHOUSE_PASSWORD:?CLICKHOUSE_PASSWORD is required}"

readonly image="clickhouse/clickhouse-server:25.8.28.1"
readonly container="deploycrate-ce-clickhouse"
readonly volume="deploycrate-ce-clickhouse"

docker volume create "${volume}" >/dev/null
if docker container inspect "${container}" >/dev/null 2>&1 &&
  [ "$(docker inspect --format '{{ index .Config.Labels "com.deploycrate.component" }}' "${container}")" != clickhouse ]; then
  docker rm --force "${container}" >/dev/null
fi
if docker container inspect "${container}" >/dev/null 2>&1; then
  docker start "${container}" >/dev/null
else
  docker run --detach \
    --name "${container}" \
    --label com.deploycrate.component=clickhouse \
    --restart unless-stopped \
    --publish 127.0.0.1:8123:8123 \
    --publish 127.0.0.1:9000:9000 \
    --env CLICKHOUSE_USER=deploycrate \
    --env "CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD}" \
    --volume "${volume}:/var/lib/clickhouse" \
    "${image}" >/dev/null
fi

clickhouse_diagnostics() {
  printf 'ClickHouse did not become ready for schema initialization\n' >&2
  docker inspect --format \
    'status={{.State.Status}} running={{.State.Running}} exit_code={{.State.ExitCode}} error={{json .State.Error}} restart_count={{.RestartCount}}' \
    "${container}" >&2 || true
  docker logs --tail 200 "${container}" >&2 || true
  ss -lntp >&2 || true
}

for attempt in $(seq 1 60); do
  if docker exec "${container}" clickhouse-client \
    --user deploycrate --password "${CLICKHOUSE_PASSWORD}" --query 'SELECT 1' >/dev/null 2>&1; then
    break
  fi
  if [ "$(docker inspect --format '{{.State.Running}}' "${container}" 2>/dev/null)" != true ]; then
    clickhouse_diagnostics
    exit 1
  fi
  [ "${attempt}" -lt 60 ] || {
    clickhouse_diagnostics
    exit 1
  }
  sleep 1
done

if ! docker exec "${container}" clickhouse-client \
  --user deploycrate --password "${CLICKHOUSE_PASSWORD}" --multiquery --query '
CREATE DATABASE IF NOT EXISTS deploycrate;
CREATE TABLE IF NOT EXISTS deploycrate.metric_rollups_v2
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
' >/dev/null; then
  clickhouse_diagnostics
  exit 1
fi
