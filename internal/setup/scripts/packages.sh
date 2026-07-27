#!/usr/bin/env bash
set -euo pipefail

if command -v cloud-init >/dev/null 2>&1; then
  cloud-init status --wait || true
fi

packages="curl ca-certificates gnupg debian-keyring debian-archive-keyring apt-transport-https openssh-server sudo git ufw fail2ban wireguard-tools bzip2"
missing=""
for package in ${packages}; do
  if ! dpkg-query -W -f='${Status}' "${package}" 2>/dev/null | grep -q 'install ok installed'; then
    missing="${missing} ${package}"
  fi
done

if [ -n "${missing}" ]; then
  DEBIAN_FRONTEND=noninteractive apt-get -o dpkg::lock::timeout=300 update
  DEBIAN_FRONTEND=noninteractive apt-get -o dpkg::lock::timeout=300 install -y ${missing}
fi

if [ ! -x /usr/lib/systemd/systemd-socket-proxyd ] && [ ! -x /lib/systemd/systemd-socket-proxyd ]; then
  printf 'systemd-socket-proxyd is required for private Resource listeners\n' >&2
  exit 1
fi
