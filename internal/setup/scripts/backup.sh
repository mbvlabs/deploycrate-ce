#!/usr/bin/env bash
set -euo pipefail

readonly version="0.18.1"
temporary_directory=""
temporary_sources=""
temporary_excludes=""
cleanup() {
  if [ -n "${temporary_directory}" ]; then
    rm -rf "${temporary_directory}"
  fi
  if [ -n "${temporary_sources}" ]; then
    rm -f "${temporary_sources}"
  fi
  if [ -n "${temporary_excludes}" ]; then
    rm -f "${temporary_excludes}"
  fi
}
trap cleanup EXIT

architecture="$(dpkg --print-architecture)"
case "${architecture}" in
  amd64)
    checksum="680838f19d67151adba227e1570cdd8af12c19cf1735783ed1ba928bc41f363d"
    ;;
  arm64)
    checksum="87f53fddde38764095e9c058a3b31834052c37e5826d2acf34e18923c006bd45"
    ;;
  *)
    printf 'Unsupported Restic architecture: %s\n' "${architecture}" >&2
    exit 1
    ;;
esac

installed_version=""
if [ -x /usr/local/bin/restic ]; then
  installed_version="$(/usr/local/bin/restic version | awk 'NR == 1 {print $2}')"
fi
if [ "${installed_version}" != "${version}" ]; then
  temporary_directory="$(mktemp -d)"
  archive="restic_${version}_linux_${architecture}.bz2"
  curl --fail --silent --show-error --location --retry 3 \
    --output "${temporary_directory}/${archive}" \
    "https://github.com/restic/restic/releases/download/v${version}/${archive}"
  printf '%s  %s\n' "${checksum}" "${temporary_directory}/${archive}" | sha256sum --check --status
  bunzip2 "${temporary_directory}/${archive}"
  install -o root -g root -m 0755 \
    "${temporary_directory}/${archive%.bz2}" \
    /usr/local/bin/restic
fi

install -d -o root -g root -m 0755 /usr/local/share/deploycrate-ce
install -d -o deploycrate -g deploycrate -m 0700 /var/lib/deploycrate-ce/runtime/backups

temporary_sources="$(mktemp)"
temporary_excludes="$(mktemp)"

printf '%s\n' \
  /etc/deploycrate-ce \
  /etc/caddy \
  /etc/wireguard \
  /etc/ssh \
  /var/lib/deploycrate-ce \
  /home/admin/.ssh/authorized_keys \
  /etc/systemd/system/deploycrate-ce@.service \
  /etc/systemd/system/caddy.service \
  /etc/systemd/system/clickhouse.service \
  /etc/systemd/system/prometheus.service \
  /etc/systemd/system/node-exporter.service \
  > "${temporary_sources}"

printf '%s\n' \
  /var/lib/deploycrate-ce/runtime/backups \
  /var/lib/docker/volumes/deploycrate-ce-postgres \
  '**/postgresql/**' \
  '**/pg_wal/**' \
  '**/*.log' \
  '**/tmp/**' \
  '**/cache/**' \
  > "${temporary_excludes}"

install -o root -g root -m 0644 "${temporary_sources}" \
  /usr/local/share/deploycrate-ce/backup-sources-v1
install -o root -g root -m 0644 "${temporary_excludes}" \
  /usr/local/share/deploycrate-ce/backup-excludes-v1
