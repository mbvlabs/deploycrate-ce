#!/usr/bin/env bash
set -euo pipefail

: "${CLICKHOUSE_PASSWORD:?CLICKHOUSE_PASSWORD is required}"

readonly image="clickhouse/clickhouse-server:25.8.28.1"
readonly container="deploycrate-ce-clickhouse"
readonly volume="deploycrate-ce-clickhouse"
readonly metrics_config="/etc/deploycrate-ce/clickhouse/prometheus.xml"

install -d -o root -g root -m 0755 /etc/deploycrate-ce/clickhouse
cat > "${metrics_config}" <<'EOF'
<clickhouse>
  <prometheus>
    <endpoint>/metrics</endpoint>
    <port>9363</port>
    <metrics>true</metrics>
    <events>true</events>
    <asynchronous_metrics>true</asynchronous_metrics>
    <status_info>true</status_info>
  </prometheus>
</clickhouse>
EOF
chmod 0644 "${metrics_config}"

docker volume create "${volume}" >/dev/null
if docker container inspect "${container}" >/dev/null 2>&1; then
  component="$(docker inspect --format '{{ index .Config.Labels "com.deploycrate.component" }}' "${container}")"
  logging_driver="$(docker inspect --format '{{.HostConfig.LogConfig.Type}}' "${container}")"
  published_http="$(docker inspect --format '{{with (index .HostConfig.PortBindings "8123/tcp")}}{{(index . 0).HostIp}}:{{(index . 0).HostPort}}{{end}}' "${container}")"
  published_native="$(docker inspect --format '{{with (index .HostConfig.PortBindings "9000/tcp")}}{{(index . 0).HostIp}}:{{(index . 0).HostPort}}{{end}}' "${container}")"
  published_metrics="$(docker inspect --format '{{with (index .HostConfig.PortBindings "9363/tcp")}}{{(index . 0).HostIp}}:{{(index . 0).HostPort}}{{end}}' "${container}")"
  mounted_metrics="$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/etc/clickhouse-server/config.d/prometheus.xml"}}{{.Source}}{{end}}{{end}}' "${container}")"
  if [ "${component}" != clickhouse ] || [ "${logging_driver}" != journald ] || \
    [ "${published_http}" != 127.0.0.1:8123 ] || [ "${published_native}" != 127.0.0.1:9000 ] || \
    [ "${published_metrics}" != 127.0.0.1:9363 ] || [ "${mounted_metrics}" != "${metrics_config}" ]; then
    docker rm --force "${container}" >/dev/null
  fi
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
    --publish 127.0.0.1:9363:9363 \
    --env CLICKHOUSE_USER=deploycrate \
    --env "CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD}" \
    --volume "${volume}:/var/lib/clickhouse" \
    --volume "${metrics_config}:/etc/clickhouse-server/config.d/prometheus.xml:ro" \
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
  --user deploycrate --password "${CLICKHOUSE_PASSWORD}" \
  --query 'CREATE DATABASE IF NOT EXISTS deploycrate' >/dev/null; then
  clickhouse_diagnostics
  exit 1
fi
