#!/usr/bin/env bash
set -euo pipefail

DEBIAN_FRONTEND=noninteractive apt-get upgrade -y -o Dpkg::Options::=--force-confold >/dev/null

reboot_required="false"
if [ -f /var/run/reboot-required ]; then
  reboot_required="true"
fi
printf '{"rebootRequired":%s}\n' "${reboot_required}"
