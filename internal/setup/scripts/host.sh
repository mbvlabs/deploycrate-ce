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

swapfile_active=false
if swapon --noheadings --raw 2>/dev/null | awk '{print $1}' | grep -Fxq /swapfile; then
  swapfile_active=true
elif ! swapon --noheadings --raw 2>/dev/null | grep -q .; then
  if [ ! -f /swapfile ]; then
    if ! fallocate -l 1G /swapfile; then
      dd if=/dev/zero of=/swapfile bs=1M count=1024 status=none
    fi
  fi
  chmod 0600 /swapfile
  mkswap /swapfile >/dev/null
  swapon /swapfile
  swapfile_active=true
fi

if [ "${swapfile_active}" = true ] && ! grep -q '^/swapfile[[:space:]]' /etc/fstab; then
  printf '/swapfile none swap sw 0 0\n' >> /etc/fstab
fi

cat > /usr/local/bin/deploycrate-host-preflight <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

check_disk() {
  local mountpoint="$1"
  local free_mb use_percent free_percent
  read -r free_mb use_percent < <(df -Pm "${mountpoint}" | awk 'NR == 2 {print $4, $5}')
  use_percent="${use_percent%%%}"
  free_percent=$((100 - use_percent))
  if [ "${free_mb}" -lt 5120 ] || [ "${free_percent}" -lt 10 ]; then
    printf 'disk pressure on %s: %s MB and %s%% free\n' "${mountpoint}" "${free_mb}" "${free_percent}" >&2
    exit 1
  fi
}

check_disk /
if [ -d /var/lib/docker ]; then
  check_disk /var/lib/docker
fi

available_memory_mb="$(awk '/MemAvailable:/ {print int($2 / 1024)}' /proc/meminfo)"
free_swap_mb="$(awk '/SwapFree:/ {print int($2 / 1024)}' /proc/meminfo)"
if [ "${available_memory_mb}" -lt 256 ]; then
  printf 'memory pressure: %s MB available\n' "${available_memory_mb}" >&2
  exit 1
fi
if [ "${free_swap_mb}" -lt 128 ]; then
  printf 'swap pressure: %s MB free\n' "${free_swap_mb}" >&2
  exit 1
fi
EOF
chmod 0755 /usr/local/bin/deploycrate-host-preflight

cat > /usr/local/bin/deploycrate-docker-gc <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
command -v docker >/dev/null 2>&1 || exit 0
docker container prune --force --filter 'until=24h' >/dev/null 2>&1 || true
docker image prune --force >/dev/null 2>&1 || true
docker builder prune --force --filter 'until=168h' >/dev/null 2>&1 || true
EOF
chmod 0755 /usr/local/bin/deploycrate-docker-gc

cat > /usr/local/bin/deploycrate-host-guard <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if /usr/local/bin/deploycrate-host-preflight >/dev/null 2>&1; then
  exit 0
fi
logger -t deploycrate-host-guard 'host pressure detected, attempting safe Docker cleanup'
/usr/local/bin/deploycrate-docker-gc || true
if ! /usr/local/bin/deploycrate-host-preflight >/dev/null 2>&1; then
  logger -t deploycrate-host-guard 'host pressure persists after cleanup'
  exit 1
fi
logger -t deploycrate-host-guard 'host pressure recovered after cleanup'
EOF
chmod 0755 /usr/local/bin/deploycrate-host-guard

cat > /etc/systemd/system/deploycrate-docker-gc.service <<'EOF'
[Unit]
Description=DeployCrate Docker garbage collection
After=docker.service
Wants=docker.service

[Service]
Type=oneshot
ExecStart=/usr/local/bin/deploycrate-docker-gc
Nice=10
EOF

cat > /etc/systemd/system/deploycrate-docker-gc.timer <<'EOF'
[Unit]
Description=Daily DeployCrate Docker garbage collection

[Timer]
OnCalendar=daily
RandomizedDelaySec=15m
Persistent=true

[Install]
WantedBy=timers.target
EOF

cat > /etc/systemd/system/deploycrate-host-guard.service <<'EOF'
[Unit]
Description=DeployCrate host resource guard
After=docker.service
Wants=docker.service

[Service]
Type=oneshot
ExecStart=/usr/local/bin/deploycrate-host-guard
Nice=10
EOF

cat > /etc/systemd/system/deploycrate-host-guard.timer <<'EOF'
[Unit]
Description=Periodic DeployCrate host resource guard

[Timer]
OnBootSec=2m
OnUnitActiveSec=15m
RandomizedDelaySec=60s
Persistent=true

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now deploycrate-docker-gc.timer
systemctl enable --now deploycrate-host-guard.timer
