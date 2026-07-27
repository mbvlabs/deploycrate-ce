#!/usr/bin/env bash
set -euo pipefail

: "${ADMIN_USER:?ADMIN_USER is required}"
: "${CONTAINERD_PACKAGE_VERSION:?CONTAINERD_PACKAGE_VERSION is required}"
: "${DOCKER_BUILDX_PACKAGE_VERSION:?DOCKER_BUILDX_PACKAGE_VERSION is required}"
: "${DOCKER_CE_PACKAGE_VERSION:?DOCKER_CE_PACKAGE_VERSION is required}"
: "${DOCKER_COMPOSE_PACKAGE_VERSION:?DOCKER_COMPOSE_PACKAGE_VERSION is required}"
: "${DOCKER_ENGINE_VERSION:?DOCKER_ENGINE_VERSION is required}"
: "${SERVICE_USER:?SERVICE_USER is required}"

os_id=$(. /etc/os-release && printf '%s' "${ID}")
codename=$(. /etc/os-release && printf '%s' "${VERSION_CODENAME}")

install -m 0755 -d /etc/apt/keyrings
curl -fsSL "https://download.docker.com/linux/${os_id}/gpg" | gpg --dearmor --yes -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg
printf 'deb [arch=%s signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/%s %s stable\n' \
  "$(dpkg --print-architecture)" "${os_id}" "${codename}" > /etc/apt/sources.list.d/docker.list
DEBIAN_FRONTEND=noninteractive apt-get -o dpkg::lock::timeout=300 update
DEBIAN_FRONTEND=noninteractive apt-get -o dpkg::lock::timeout=300 install -y \
  --allow-downgrades \
  --allow-change-held-packages \
  "docker-ce=${DOCKER_CE_PACKAGE_VERSION}" \
  "docker-ce-cli=${DOCKER_CE_PACKAGE_VERSION}" \
  "containerd.io=${CONTAINERD_PACKAGE_VERSION}" \
  "docker-buildx-plugin=${DOCKER_BUILDX_PACKAGE_VERSION}" \
  "docker-compose-plugin=${DOCKER_COMPOSE_PACKAGE_VERSION}"

for package_version in \
  "docker-ce:${DOCKER_CE_PACKAGE_VERSION}" \
  "docker-ce-cli:${DOCKER_CE_PACKAGE_VERSION}" \
  "containerd.io:${CONTAINERD_PACKAGE_VERSION}" \
  "docker-buildx-plugin:${DOCKER_BUILDX_PACKAGE_VERSION}" \
  "docker-compose-plugin:${DOCKER_COMPOSE_PACKAGE_VERSION}"; do
  package="${package_version%%:*}"
  expected_version="${package_version#*:}"
  installed_version="$(dpkg-query -W -f='${Version}' "${package}")"
  if [ "${installed_version}" != "${expected_version}" ]; then
    printf 'Expected %s %s after installation, found %s\n' \
      "${package}" "${expected_version}" "${installed_version}" >&2
    exit 1
  fi
done

apt-mark hold \
  docker-ce \
  docker-ce-cli \
  containerd.io \
  docker-buildx-plugin \
  docker-compose-plugin >/dev/null

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
installed_engine_version="$(docker version --format '{{.Server.Version}}')"
if [ "${installed_engine_version}" != "${DOCKER_ENGINE_VERSION}" ]; then
  printf 'Expected Docker Engine %s after installation, found %s\n' \
    "${DOCKER_ENGINE_VERSION}" "${installed_engine_version}" >&2
  exit 1
fi
usermod --append --groups docker "${ADMIN_USER}"
usermod --append --groups docker "${SERVICE_USER}"
