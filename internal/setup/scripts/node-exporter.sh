#!/usr/bin/env bash
set -euo pipefail

readonly version="1.11.1"
architecture="$(dpkg --print-architecture)"
case "${architecture}" in
  amd64)
    checksum="9f5ea48e5bc7b656f8a91a32e7d7deb89f70f73dabd0d974418aca15f37d6810"
    ;;
  arm64)
    checksum="ba1886efbd76cb96b0087c695ea8d1b9cb6e8aa946c996d744e9ee16c8e3591a"
    ;;
  *)
    printf 'Unsupported node-exporter architecture: %s\n' "${architecture}" >&2
    exit 1
    ;;
esac

if ! id node_exporter >/dev/null 2>&1; then
  useradd --system --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin node_exporter
fi

installed_version=""
if [ -x /usr/local/bin/node_exporter ]; then
  installed_version="$(/usr/local/bin/node_exporter --version 2>&1 | awk 'NR == 1 {print $3}')"
fi
if [ "${installed_version}" != "${version}" ]; then
  temporary_directory="$(mktemp -d)"
  trap 'rm -rf "${temporary_directory}"' EXIT
  archive="node_exporter-${version}.linux-${architecture}.tar.gz"
  curl --fail --silent --show-error --location --retry 3 \
    --output "${temporary_directory}/${archive}" \
    "https://github.com/prometheus/node_exporter/releases/download/v${version}/${archive}"
  printf '%s  %s\n' "${checksum}" "${temporary_directory}/${archive}" | sha256sum --check --status
  tar --extract --gzip --file "${temporary_directory}/${archive}" --directory "${temporary_directory}"
  install -o root -g root -m 0755 \
    "${temporary_directory}/node_exporter-${version}.linux-${architecture}/node_exporter" \
    /usr/local/bin/node_exporter
fi

cat > /etc/systemd/system/node-exporter.service <<'EOF'
[Unit]
Description=Prometheus node exporter for DeployCrate
Wants=network-online.target wg-quick@wg0.service
After=network-online.target wg-quick@wg0.service
StartLimitIntervalSec=300
StartLimitBurst=5

[Service]
Type=simple
User=node_exporter
Group=node_exporter
ExecStart=/usr/local/bin/node_exporter --web.listen-address=10.99.0.1:9100
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

ufw delete allow 9100/tcp >/dev/null 2>&1 || true
ufw allow in on wg0 to 10.99.0.1 port 9100 proto tcp
systemctl daemon-reload
systemctl enable node-exporter.service
systemctl restart node-exporter.service

node_exporter_diagnostics() {
  printf 'node-exporter did not become healthy at 10.99.0.1:9100\n' >&2
  systemctl status node-exporter.service --no-pager >&2 || true
  journalctl -u node-exporter.service -b --no-pager --lines=100 >&2 || true
  ip -4 address show dev wg0 >&2 || true
  ss -lntp >&2 || true
  curl --verbose --max-time 5 http://10.99.0.1:9100/metrics >/dev/null || true
}

for attempt in $(seq 1 30); do
  if curl --fail --silent http://10.99.0.1:9100/metrics >/dev/null; then
    exit 0
  fi
  if ! systemctl is-active --quiet node-exporter.service; then
    node_exporter_diagnostics
    exit 1
  fi
  [ "${attempt}" -lt 30 ] || {
    node_exporter_diagnostics
    exit 1
  }
  sleep 1
done
