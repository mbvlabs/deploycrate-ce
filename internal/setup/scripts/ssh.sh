#!/usr/bin/env bash
set -euo pipefail

: "${USERNAME:?USERNAME is required}"
: "${SSH_PORT:?SSH_PORT is required}"

install -d -m 0755 /etc/ssh/sshd_config.d
cat > /etc/ssh/sshd_config.d/99-deploycrate-ce.conf <<EOF
Port ${SSH_PORT}
AddressFamily inet
PermitRootLogin no
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitEmptyPasswords no
X11Forwarding no
AllowUsers ${USERNAME}
MaxAuthTries 3
MaxSessions 4
ClientAliveInterval 300
ClientAliveCountMax 2
EOF

sshd -t
ufw allow "${SSH_PORT}/tcp"
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable
ufw reload
systemctl restart ssh
