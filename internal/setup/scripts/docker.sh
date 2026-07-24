#!/usr/bin/env bash
set -euo pipefail

: "${ADMIN_USER:?ADMIN_USER is required}"
: "${SERVICE_USER:?SERVICE_USER is required}"

os_id=$(. /etc/os-release && printf '%s' "${ID}")
codename=$(. /etc/os-release && printf '%s' "${VERSION_CODENAME}")

if ! command -v docker >/dev/null 2>&1; then
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL "https://download.docker.com/linux/${os_id}/gpg" | gpg --dearmor --yes -o /etc/apt/keyrings/docker.gpg
  chmod a+r /etc/apt/keyrings/docker.gpg
  printf 'deb [arch=%s signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/%s %s stable\n' \
    "$(dpkg --print-architecture)" "${os_id}" "${codename}" > /etc/apt/sources.list.d/docker.list
  DEBIAN_FRONTEND=noninteractive apt-get -o dpkg::lock::timeout=300 update
  DEBIAN_FRONTEND=noninteractive apt-get -o dpkg::lock::timeout=300 install -y \
    docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
fi

install -d -m 0755 /etc/docker
cat > /etc/docker/daemon.json <<'EOF'
{
  "log-driver": "local",
  "log-opts": {
    "max-size": "10m",
    "max-file": "5",
    "compress": "true"
  },
  "live-restore": true
}
EOF

systemctl enable --now docker
systemctl restart docker
usermod --append --groups docker "${ADMIN_USER}"
usermod --append --groups docker "${SERVICE_USER}"
