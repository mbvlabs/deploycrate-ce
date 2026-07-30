#!/usr/bin/env bash
set -euo pipefail

readonly version="3.13.1"
architecture="$(dpkg --print-architecture)"
case "${architecture}" in
  amd64)
    checksum="962b812371aff838d152b6ff2d56fdb7a6396f5542f48ebf73421b9721f0d103"
    ;;
  arm64)
    checksum="fbd8e5e0f6ad2e7d053e717739186caee4fd0cab2cf9335bfc86c292fe2a2bfe"
    ;;
  *)
    printf 'Unsupported Prometheus architecture: %s\n' "${architecture}" >&2
    exit 1
    ;;
esac

if ! id prometheus >/dev/null 2>&1; then
  useradd --system --no-create-home --home-dir /var/lib/prometheus --shell /usr/sbin/nologin prometheus
fi
install -d -o root -g prometheus -m 0750 /etc/prometheus
install -d -o prometheus -g prometheus -m 0750 /var/lib/prometheus

installed_version=""
if [ -x /usr/local/bin/prometheus ]; then
  installed_version="$(/usr/local/bin/prometheus --version 2>&1 | awk 'NR == 1 {print $3}')"
fi
if [ "${installed_version}" != "${version}" ]; then
  temporary_directory="$(mktemp -d)"
  trap 'rm -rf "${temporary_directory}"' EXIT
  archive="prometheus-${version}.linux-${architecture}.tar.gz"
  curl --fail --silent --show-error --location --retry 3 \
    --output "${temporary_directory}/${archive}" \
    "https://github.com/prometheus/prometheus/releases/download/v${version}/${archive}"
  printf '%s  %s\n' "${checksum}" "${temporary_directory}/${archive}" | sha256sum --check --status
  tar --extract --gzip --file "${temporary_directory}/${archive}" --directory "${temporary_directory}"
  install -o root -g root -m 0755 \
    "${temporary_directory}/prometheus-${version}.linux-${architecture}/prometheus" \
    /usr/local/bin/prometheus
  install -o root -g root -m 0755 \
    "${temporary_directory}/prometheus-${version}.linux-${architecture}/promtool" \
    /usr/local/bin/promtool
fi

cat > /etc/prometheus/prometheus.yml <<'EOF'
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: prometheus
    static_configs:
      - targets: ['127.0.0.1:9090']
        labels:
          server: control-plane
          target: prometheus
          component: prometheus
  - job_name: node-exporter
    static_configs:
      - targets: ['10.99.0.1:9100']
        labels:
          server: control-plane
          target: control-plane
          component: node-exporter
  - job_name: cadvisor
    static_configs:
      - targets: ['127.0.0.1:9101']
        labels:
          server: control-plane
          target: cadvisor
          collector: cadvisor
    metric_relabel_configs:
      - source_labels: [__name__]
        action: keep
        regex: 'container_(cpu_usage_seconds_total|cpu_cfs_throttled_seconds_total|cpu_cfs_periods_total|cpu_cfs_throttled_periods_total|memory_working_set_bytes|memory_usage_bytes|oom_events_total|fs_reads_bytes_total|fs_writes_bytes_total|network_receive_bytes_total|network_transmit_bytes_total|processes|tasks_state|spec_cpu_quota|spec_cpu_period|spec_memory_limit_bytes|last_seen)|process_.*|cadvisor_version_info|container_scrape_error'
      - source_labels: [__name__, container_label_com_deploycrate_application, container_label_com_deploycrate_resource_installation, container_label_com_deploycrate_component, id]
        separator: ';'
        action: keep
        regex: '(process_.*|cadvisor_version_info|container_scrape_error);.*|[^;]+;[^;]+;.*|[^;]+;;[^;]+;.*|[^;]+;;;[^;]+;.*|[^;]+;;;;/system\.slice/(prometheus|node-exporter|cadvisor|docker|caddy|deploycrate-ce@(blue|green))\.service'
      - action: labelmap
        regex: container_label_com_deploycrate_(application|environment|deployment|instance|release|resource_installation|component)
        replacement: '$1'
      - source_labels: [__name__]
        regex: 'process_.*|cadvisor_version_info|container_scrape_error'
        target_label: component
        replacement: cadvisor
      - source_labels: [id]
        regex: '/system\.slice/prometheus\.service'
        target_label: component
        replacement: prometheus
      - source_labels: [id]
        regex: '/system\.slice/node-exporter\.service'
        target_label: component
        replacement: node-exporter
      - source_labels: [id]
        regex: '/system\.slice/cadvisor\.service'
        target_label: component
        replacement: cadvisor
      - source_labels: [id]
        regex: '/system\.slice/docker\.service'
        target_label: component
        replacement: docker
      - source_labels: [id]
        regex: '/system\.slice/caddy\.service'
        target_label: component
        replacement: caddy
      - source_labels: [id]
        regex: '/system\.slice/deploycrate-ce@(blue|green)\.service'
        target_label: component
        replacement: deploycrate-ce
      - regex: 'container_label_.*|image|name|collector'
        action: labeldrop
EOF
chown root:prometheus /etc/prometheus/prometheus.yml
chmod 0640 /etc/prometheus/prometheus.yml
/usr/local/bin/promtool check config /etc/prometheus/prometheus.yml

cat > /etc/systemd/system/prometheus.service <<'EOF'
[Unit]
Description=Prometheus for DeployCrate
Wants=network-online.target node-exporter.service cadvisor.service
After=network-online.target node-exporter.service cadvisor.service

[Service]
Type=simple
User=prometheus
Group=prometheus
ExecStart=/usr/local/bin/prometheus --config.file=/etc/prometheus/prometheus.yml --storage.tsdb.path=/var/lib/prometheus --storage.tsdb.retention.time=24h --web.listen-address=127.0.0.1:9090
Restart=on-failure
RestartSec=5
CPUAccounting=yes
MemoryAccounting=yes
IOAccounting=yes
TasksAccounting=yes
NoNewPrivileges=true
PrivateDevices=true
PrivateTmp=true
ProtectClock=true
ProtectControlGroups=true
ProtectHome=true
ProtectHostname=true
ProtectKernelLogs=true
ProtectKernelModules=true
ProtectKernelTunables=true
ProtectSystem=strict
ReadWritePaths=/var/lib/prometheus
LockPersonality=true
MemoryDenyWriteExecute=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
RestrictNamespaces=true
RestrictRealtime=true
RestrictSUIDSGID=true
SystemCallArchitectures=native
CapabilityBoundingSet=
AmbientCapabilities=

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now prometheus.service
systemctl restart prometheus.service
for attempt in $(seq 1 30); do
  if curl --fail --silent http://127.0.0.1:9090/-/ready >/dev/null; then
    exit 0
  fi
  [ "${attempt}" -lt 30 ] || exit 1
  sleep 1
done
