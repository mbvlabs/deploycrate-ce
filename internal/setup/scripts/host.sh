#!/usr/bin/env bash
set -euo pipefail

: "${SSH_PORT:?SSH_PORT is required}"

install -d -m 0755 /etc/systemd/journald.conf.d
install -d -m 2755 /var/log/journal
cat > /etc/systemd/journald.conf.d/deploycrate-ce.conf <<'EOF'
[Journal]
Storage=persistent
Compress=yes
SystemMaxUse=1G
SystemKeepFree=1G
RuntimeMaxUse=256M
RuntimeKeepFree=256M
MaxRetentionSec=14day
EOF
systemctl restart systemd-journald

cat > /etc/fail2ban/jail.d/deploycrate-ce.conf <<EOF
[sshd]
enabled = true
port = ${SSH_PORT}
filter = sshd
backend = systemd
maxretry = 3
bantime = 3600
EOF
systemctl enable --now fail2ban
systemctl restart fail2ban

if ! swapon --noheadings --raw 2>/dev/null | grep -q .; then
  if [ ! -f /swapfile ]; then
    if ! fallocate -l 1G /swapfile; then
      dd if=/dev/zero of=/swapfile bs=1M count=1024 status=none
    fi
  fi
  chmod 0600 /swapfile
  mkswap /swapfile >/dev/null
  swapon /swapfile
fi

if ! grep -q '^/swapfile[[:space:]]' /etc/fstab; then
  printf '/swapfile none swap sw 0 0\n' >> /etc/fstab
fi
