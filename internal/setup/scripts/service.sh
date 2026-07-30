#!/usr/bin/env bash
set -euo pipefail

: "${SERVICE_USER:?SERVICE_USER is required}"
: "${CADDY_VERSION:?CADDY_VERSION is required}"

install -d -m 0755 /opt/deploycrate-ce/slots/blue /opt/deploycrate-ce/slots/green
install -d -m 0700 /etc/deploycrate-ce/slots
install -d -m 0755 /var/lib/deploycrate-ce
install -d -m 0750 -o "${SERVICE_USER}" -g "${SERVICE_USER}" /var/lib/deploycrate-ce/runtime

cat > /etc/deploycrate-ce/slots/blue.env <<EOF
PORT="8080"
DEPLOYCRATE_SLOT="blue"
EOF

cat > /etc/deploycrate-ce/slots/green.env <<EOF
PORT="8081"
DEPLOYCRATE_SLOT="green"
EOF

chmod 0600 /etc/deploycrate-ce/slots/blue.env /etc/deploycrate-ce/slots/green.env

cat > /etc/systemd/system/deploycrate-ce@.service <<EOF
[Unit]
Description=DeployCrate CE %i slot
Wants=network-online.target otelcol-contrib.service
After=network-online.target docker.service otelcol-contrib.service

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
WorkingDirectory=/opt/deploycrate-ce
EnvironmentFile=/etc/deploycrate-ce/app.env
EnvironmentFile=/etc/deploycrate-ce/slots/%i.env
ExecStart=/opt/deploycrate-ce/slots/%i/deploycrate-ce
ExecStop=/bin/kill -SIGTERM \$MAINPID
KillMode=mixed
Restart=on-failure
RestartSec=5
TimeoutStopSec=30
LimitNOFILE=65535
CPUAccounting=yes
MemoryAccounting=yes
IOAccounting=yes
TasksAccounting=yes

[Install]
WantedBy=multi-user.target
EOF

readonly CADDY_MODULE="http.reverse_proxy.selection_policies.weighted_round_robin"
architecture="$(dpkg --print-architecture)"
case "${architecture}" in
  amd64)
    caddy_checksum="c41708ffb4af9bc6d19f7d22a7a034804352a21ecc62e1d3dfe3d58e30b38a3e"
    ;;
  arm64)
    caddy_checksum="aeab2e38bf77a0162611a1703a5e16c09475b000d41f7edaa9337734d16642fd"
    ;;
  *)
    printf 'Unsupported Caddy architecture: %s\n' "${architecture}" >&2
    exit 1
    ;;
esac

installed_caddy_version=""
if command -v caddy >/dev/null 2>&1; then
  installed_caddy_version="$(caddy version | awk '{print $1}')"
fi
if [ "${installed_caddy_version}" != "v${CADDY_VERSION}" ]; then
  caddy_package="caddy_${CADDY_VERSION}_linux_${architecture}.deb"
  caddy_url="https://github.com/caddyserver/caddy/releases/download/v${CADDY_VERSION}/${caddy_package}"
  download_directory="$(mktemp -d)"
  trap 'rm -rf "${download_directory}"' EXIT
  curl --fail --silent --show-error --location --retry 3 --output "${download_directory}/${caddy_package}" "${caddy_url}"
  printf '%s  %s\n' "${caddy_checksum}" "${download_directory}/${caddy_package}" | sha256sum --check --status
  DEBIAN_FRONTEND=noninteractive apt-get -o dpkg::lock::timeout=300 install -y --allow-downgrades --allow-change-held-packages "${download_directory}/${caddy_package}"
fi

if [ "$(caddy version | awk '{print $1}')" != "v${CADDY_VERSION}" ]; then
  printf 'Expected Caddy v%s after installation\n' "${CADDY_VERSION}" >&2
  exit 1
fi
if ! caddy list-modules | grep -Fx "${CADDY_MODULE}" >/dev/null; then
  printf 'Installed Caddy v%s does not provide required module %s\n' "${CADDY_VERSION}" "${CADDY_MODULE}" >&2
  exit 1
fi
apt-mark hold caddy >/dev/null

cat > /etc/caddy/Caddyfile <<EOF
{
  admin 127.0.0.1:2019
  metrics {
    per_host
  }
}
EOF

systemctl daemon-reload
systemctl enable --now deploycrate-ce@blue.service
systemctl enable --now caddy
systemctl restart caddy
