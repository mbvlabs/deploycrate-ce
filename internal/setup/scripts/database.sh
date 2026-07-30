#!/usr/bin/env bash
set -euo pipefail

: "${DB_NAME:?DB_NAME is required}"
: "${DB_USER:?DB_USER is required}"
: "${DB_PASSWORD:?DB_PASSWORD is required}"

container="deploycrate-ce-postgres"
volume="deploycrate-ce-postgres"

docker volume create "${volume}" >/dev/null
if docker container inspect "${container}" >/dev/null 2>&1; then
  component="$(docker inspect --format '{{ index .Config.Labels "com.deploycrate.component" }}' "${container}")"
  logging_driver="$(docker inspect --format '{{.HostConfig.LogConfig.Type}}' "${container}")"
  if [ "${component}" != postgresql ] || [ "${logging_driver}" != journald ]; then
    docker rm --force "${container}" >/dev/null
  fi
fi
if docker container inspect "${container}" >/dev/null 2>&1; then
  docker start "${container}" >/dev/null
else
  docker run --detach \
    --name "${container}" \
    --label com.deploycrate.component=postgresql \
    --restart unless-stopped \
    --publish 127.0.0.1:5432:5432 \
    --env "POSTGRES_DB=${DB_NAME}" \
    --env "POSTGRES_USER=${DB_USER}" \
    --env "POSTGRES_PASSWORD=${DB_PASSWORD}" \
    --volume "${volume}:/var/lib/postgresql/data" \
    postgres:17-alpine >/dev/null
fi

for attempt in $(seq 1 60); do
  if docker exec "${container}" pg_isready --username "${DB_USER}" --dbname "${DB_NAME}" >/dev/null 2>&1; then
    exit 0
  fi
  if [ "${attempt}" -eq 60 ]; then
    docker logs --tail 50 "${container}" >&2 || true
    exit 1
  fi
  sleep 1
done
