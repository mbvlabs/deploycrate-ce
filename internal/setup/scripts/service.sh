#!/usr/bin/env bash
set -euo pipefail

: "${USERNAME:?USERNAME is required}"

install -d -m 0755 /opt/deploycrate-ce/slots/blue /opt/deploycrate-ce/slots/green
install -d -m 0700 /etc/deploycrate-ce/slots
install -d -m 0755 /var/lib/deploycrate-ce
install -d -m 0750 -o "${USERNAME}" -g "${USERNAME}" /var/lib/deploycrate-ce/runtime

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
Wants=network-online.target
After=network-online.target docker.service

[Service]
Type=simple
User=${USERNAME}
Group=${USERNAME}
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

[Install]
WantedBy=multi-user.target
EOF

if ! command -v caddy >/dev/null 2>&1; then
  curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/gpg.key | gpg --dearmor --yes -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
  curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt > /etc/apt/sources.list.d/caddy-stable.list
  DEBIAN_FRONTEND=noninteractive apt-get -o dpkg::lock::timeout=300 update
  DEBIAN_FRONTEND=noninteractive apt-get -o dpkg::lock::timeout=300 install -y caddy
fi

cat > /etc/caddy/Caddyfile <<EOF
{
  admin 127.0.0.1:2019
}
EOF

systemctl daemon-reload
systemctl enable --now deploycrate-ce@blue.service
systemctl enable --now caddy
systemctl restart caddy
