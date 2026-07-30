#!/usr/bin/env bash
set -euo pipefail

readonly version="0.57.0"
readonly listen_address="127.0.0.1"
readonly listen_port="9101"
architecture="$(dpkg --print-architecture)"
case "${architecture}" in
  amd64)
    checksum="9359a1192775eafeead41941690f7d94fb55f5f85833071b70593f8e7eae31ec"
    ;;
  arm64)
    checksum="38477947aab2dc5ff0288d4ee59e2ddb351d3a627140b707416f4b4ee91c1b85"
    ;;
  *)
    printf 'Unsupported cAdvisor architecture: %s\n' "${architecture}" >&2
    exit 1
    ;;
esac

installed_version=""
if [ -x /usr/local/bin/cadvisor ]; then
  installed_version="$(/usr/local/bin/cadvisor --version 2>&1 | sed -n 's/^cAdvisor version v\([^ ]*\).*/\1/p')"
fi
if [ "${installed_version}" != "${version}" ]; then
  temporary_directory="$(mktemp -d)"
  trap 'rm -rf "${temporary_directory}"' EXIT
  artifact="cadvisor-v${version}-linux-${architecture}"
  curl --fail --silent --show-error --location --retry 3 \
    --output "${temporary_directory}/${artifact}" \
    "https://github.com/google/cadvisor/releases/download/v${version}/${artifact}"
  printf '%s  %s\n' "${checksum}" "${temporary_directory}/${artifact}" | sha256sum --check --status
  install -o root -g root -m 0755 "${temporary_directory}/${artifact}" /usr/local/bin/cadvisor
fi

cat > /etc/systemd/system/cadvisor.service <<EOF
[Unit]
Description=cAdvisor resource accounting for DeployCrate
Wants=network-online.target docker.service
After=network-online.target docker.service

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/bin/cadvisor \\
  --listen_ip=${listen_address} \\
  --port=${listen_port} \\
  --housekeeping_interval=15s \\
  --max_housekeeping_interval=15s \\
  --allow_dynamic_housekeeping=false \\
  --docker_only=true \\
  --raw_cgroup_prefix_whitelist=/system.slice/prometheus.service,/system.slice/node-exporter.service,/system.slice/cadvisor.service,/system.slice/docker.service,/system.slice/caddy.service,/system.slice/otelcol-contrib.service,/system.slice/deploycrate-ce@ \\
  --store_container_labels=false \\
  --whitelisted_container_labels=com.deploycrate.application,com.deploycrate.environment,com.deploycrate.deployment,com.deploycrate.instance,com.deploycrate.release,com.deploycrate.resource-installation,com.deploycrate.component \\
  --enable_metrics=cpu,memory,diskIO,network,oom_event,process
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
LockPersonality=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
RestrictNamespaces=true
RestrictRealtime=true
RestrictSUIDSGID=true
SystemCallArchitectures=native

[Install]
WantedBy=multi-user.target
EOF

ufw delete allow "${listen_port}/tcp" >/dev/null 2>&1 || true
systemctl daemon-reload
systemctl enable --now cadvisor.service
systemctl restart cadvisor.service

cadvisor_diagnostics() {
  printf 'cAdvisor did not become healthy at %s:%s\n' "${listen_address}" "${listen_port}" >&2
  systemctl status cadvisor.service --no-pager >&2 || true
  journalctl -u cadvisor.service -b --no-pager --lines=100 >&2 || true
  ss -lntp >&2 || true
}

for attempt in $(seq 1 30); do
  if curl --fail --silent "http://${listen_address}:${listen_port}/healthz" >/dev/null &&
    curl --fail --silent "http://${listen_address}:${listen_port}/metrics" |
      awk -v expected_version="v${version}" '
        /^cadvisor_version_info/ && index($0, "cadvisorVersion=\"" expected_version "\"") { found = 1 }
        END { exit !found }
      '; then
    exit 0
  fi
  if ! systemctl is-active --quiet cadvisor.service; then
    cadvisor_diagnostics
    exit 1
  fi
  [ "${attempt}" -lt 30 ] || {
    cadvisor_diagnostics
    exit 1
  }
  sleep 1
done
