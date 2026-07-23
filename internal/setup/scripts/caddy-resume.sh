#!/usr/bin/env bash
set -euo pipefail

install -d -m 0755 /etc/systemd/system/caddy.service.d

cat > /etc/systemd/system/caddy.service.d/deploycrate-ce.conf <<'EOF'
[Service]
ExecStart=
ExecStart=/usr/bin/caddy run --environ --resume
ExecReload=
EOF

systemctl daemon-reload
systemctl restart caddy
