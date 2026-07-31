#!/usr/bin/env bash
set -euo pipefail

: "${CLICKHOUSE_PASSWORD:?CLICKHOUSE_PASSWORD is required}"
: "${INSTANCE_ID:?INSTANCE_ID is required}"
: "${OTELCOL_VERSION:?OTELCOL_VERSION is required}"

architecture="$(dpkg --print-architecture)"
case "${architecture}" in
  amd64)
    checksum="d33177515a244a2393f03ffd66ab3e68a8fc11a56bc145ec4d0ca2644ee95504"
    ;;
  arm64)
    checksum="34eb82390c462c877dd60ec5ec84de899088916facd07306ec988e4c34bd05b3"
    ;;
  *)
    printf 'Unsupported OpenTelemetry Collector architecture: %s\n' "${architecture}" >&2
    exit 1
    ;;
esac

if ! id otelcol-contrib >/dev/null 2>&1; then
  useradd --system --no-create-home --home-dir /var/lib/otelcol-contrib --shell /usr/sbin/nologin otelcol-contrib
fi
usermod --append --groups systemd-journal otelcol-contrib

installed_version=""
if [ -x /usr/local/bin/otelcol-contrib ]; then
  installed_version="$(/usr/local/bin/otelcol-contrib --version 2>/dev/null | sed -n 's/.* version \([^ ]*\).*/\1/p')"
fi
if [ "${installed_version}" != "${OTELCOL_VERSION}" ]; then
  temporary_directory="$(mktemp -d)"
  trap 'rm -rf "${temporary_directory}"' EXIT
  archive="otelcol-contrib_${OTELCOL_VERSION}_linux_${architecture}.tar.gz"
  curl --fail --silent --show-error --location --retry 3 \
    --output "${temporary_directory}/${archive}" \
    "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v${OTELCOL_VERSION}/${archive}"
  printf '%s  %s\n' "${checksum}" "${temporary_directory}/${archive}" | sha256sum --check --status
  tar --extract --gzip --file "${temporary_directory}/${archive}" --directory "${temporary_directory}" otelcol-contrib
  install -o root -g root -m 0755 "${temporary_directory}/otelcol-contrib" /usr/local/bin/otelcol-contrib
fi

install -d -o root -g otelcol-contrib -m 0750 /etc/otelcol-contrib
install -d -o otelcol-contrib -g otelcol-contrib -m 0750 /var/lib/otelcol-contrib/storage

cat > /etc/otelcol-contrib/environment <<EOF
CLICKHOUSE_PASSWORD="${CLICKHOUSE_PASSWORD}"
EOF
chown root:root /etc/otelcol-contrib/environment
chmod 0600 /etc/otelcol-contrib/environment

cat > /etc/otelcol-contrib/config.yaml <<EOF
extensions:
  file_storage:
    directory: /var/lib/otelcol-contrib/storage
    create_directory: true
    compaction:
      directory: /var/lib/otelcol-contrib/storage
      on_start: true
  health_check:
    endpoint: 127.0.0.1:13133

receivers:
  journald:
    directory: /var/log/journal
    start_at: end
    priority: debug
    units:
      - caddy.service
      - cadvisor.service
      - docker.service
      - fail2ban.service
      - node-exporter.service
      - prometheus.service
      - wg-quick@wg0.service
    all: true
    convert_message_bytes: true
    storage: file_storage
    retry_on_failure:
      enabled: true
      initial_interval: 1s
      max_interval: 30s
      max_elapsed_time: 0
  otlp:
    protocols:
      http:
        endpoint: 127.0.0.1:4318

processors:
  memory_limiter:
    check_interval: 1s
    limit_mib: 256
    spike_limit_mib: 64
  transform/workload_logs:
    error_mode: ignore
    log_statements:
      - set(log.attributes["deploycrate.application.id"], log.body["COM_DEPLOYCRATE_APPLICATION"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["deploycrate.environment.id"], log.body["COM_DEPLOYCRATE_ENVIRONMENT"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["deploycrate.deployment.id"], log.body["COM_DEPLOYCRATE_DEPLOYMENT"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["deploycrate.instance.id"], log.body["COM_DEPLOYCRATE_INSTANCE"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["deploycrate.release.id"], log.body["COM_DEPLOYCRATE_RELEASE"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["deploycrate.log.epoch"], log.body["CONTAINER_LOG_EPOCH"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["deploycrate.log.ordinal"], log.body["CONTAINER_LOG_ORDINAL"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["container.id"], log.body["CONTAINER_ID_FULL"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["container.name"], log.body["CONTAINER_NAME"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["log.iostream"], "stderr") where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil and log.body["PRIORITY"] == "3"
      - set(log.attributes["log.iostream"], "stdout") where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil and log.body["PRIORITY"] != "3"
      - set(log.body, log.body["MESSAGE"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil and log.body["MESSAGE"] != nil
  resource/host:
    attributes:
      - key: service.namespace
        value: deploycrate-ce
        action: upsert
      - key: host.id
        value: "${INSTANCE_ID}"
        action: upsert
  batch:
    send_batch_size: 1024
    send_batch_max_size: 4096
    timeout: 5s

exporters:
  clickhouse:
    endpoint: tcp://127.0.0.1:9000?dial_timeout=10s
    database: deploycrate
    username: deploycrate
    password: \${env:CLICKHOUSE_PASSWORD}
    create_schema: true
    logs_table_name: otel_logs
    ttl: 168h
    async_insert: true
    compress: lz4
    timeout: 10s
    sending_queue:
      enabled: true
      storage: file_storage
      queue_size: 4096
      num_consumers: 4
    retry_on_failure:
      enabled: true
      initial_interval: 1s
      max_interval: 30s
      max_elapsed_time: 0

service:
  extensions: [file_storage, health_check]
  telemetry:
    logs:
      level: info
    metrics:
      readers:
        - pull:
            exporter:
              prometheus:
                host: 127.0.0.1
                port: 8888
                without_scope_info: true
                without_type_suffix: true
                without_units: true
  pipelines:
    logs:
      receivers: [journald, otlp]
      processors: [memory_limiter, transform/workload_logs, resource/host, batch]
      exporters: [clickhouse]
EOF
chown root:otelcol-contrib /etc/otelcol-contrib/config.yaml
chmod 0640 /etc/otelcol-contrib/config.yaml

cat > /etc/systemd/system/otelcol-contrib.service <<'EOF'
[Unit]
Description=OpenTelemetry log collector for DeployCrate
Wants=network-online.target docker.service
After=network-online.target docker.service

[Service]
Type=simple
User=otelcol-contrib
Group=otelcol-contrib
SupplementaryGroups=systemd-journal
EnvironmentFile=/etc/otelcol-contrib/environment
ExecStart=/usr/local/bin/otelcol-contrib --config=/etc/otelcol-contrib/config.yaml
Restart=on-failure
RestartSec=5
CPUAccounting=yes
MemoryAccounting=yes
IOAccounting=yes
TasksAccounting=yes
NoNewPrivileges=true
PrivateTmp=true
ProtectClock=true
ProtectHome=true
ProtectHostname=true
ProtectKernelLogs=true
ProtectKernelModules=true
ProtectKernelTunables=true
ProtectSystem=strict
ReadWritePaths=/var/lib/otelcol-contrib
LockPersonality=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
RestrictNamespaces=true
RestrictRealtime=true
RestrictSUIDSGID=true
SystemCallArchitectures=native

[Install]
WantedBy=multi-user.target
EOF

ufw delete allow 4318/tcp >/dev/null 2>&1 || true
ufw delete allow 8888/tcp >/dev/null 2>&1 || true
ufw delete allow 13133/tcp >/dev/null 2>&1 || true
/usr/local/bin/otelcol-contrib validate --config=/etc/otelcol-contrib/config.yaml
systemctl daemon-reload
systemctl enable --now otelcol-contrib.service
systemctl restart otelcol-contrib.service

collector_diagnostics() {
  printf 'OpenTelemetry Collector did not become healthy\n' >&2
  systemctl status otelcol-contrib.service --no-pager >&2 || true
  journalctl -u otelcol-contrib.service -b --no-pager --lines=100 >&2 || true
  ss -lntp >&2 || true
}

for attempt in $(seq 1 30); do
  if curl --fail --silent http://127.0.0.1:13133/ >/dev/null; then
    exit 0
  fi
  if ! systemctl is-active --quiet otelcol-contrib.service; then
    collector_diagnostics
    exit 1
  fi
  [ "${attempt}" -lt 30 ] || {
    collector_diagnostics
    exit 1
  }
  sleep 1
done
