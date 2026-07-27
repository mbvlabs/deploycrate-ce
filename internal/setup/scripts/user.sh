#!/usr/bin/env bash
set -euo pipefail

: "${ADMIN_USER:?ADMIN_USER is required}"
: "${SERVICE_USER:?SERVICE_USER is required}"
: "${PASSWORD:?PASSWORD is required}"
: "${SSH_PUBLIC_KEY:?SSH_PUBLIC_KEY is required}"
: "${OWNER_SSH_PUBLIC_KEY:=}"

if [ "${ADMIN_USER}" = "${SERVICE_USER}" ]; then
  printf 'Administrator and service users must be different\n' >&2
  exit 1
fi

if ! id "${ADMIN_USER}" >/dev/null 2>&1; then
  useradd --create-home --shell /bin/bash "${ADMIN_USER}"
fi
usermod --shell /bin/bash "${ADMIN_USER}"
printf '%s:%s\n' "${ADMIN_USER}" "${PASSWORD}" | chpasswd
usermod --append --groups sudo "${ADMIN_USER}"

install -d -m 0700 -o "${ADMIN_USER}" -g "${ADMIN_USER}" "/home/${ADMIN_USER}/.ssh"
printf '%s\n' "${SSH_PUBLIC_KEY}" > "/home/${ADMIN_USER}/.ssh/authorized_keys"
if [ -n "${OWNER_SSH_PUBLIC_KEY}" ] && [ "${OWNER_SSH_PUBLIC_KEY}" != "${SSH_PUBLIC_KEY}" ]; then
  printf '%s\n' "${OWNER_SSH_PUBLIC_KEY}" >> "/home/${ADMIN_USER}/.ssh/authorized_keys"
fi
chown "${ADMIN_USER}:${ADMIN_USER}" "/home/${ADMIN_USER}/.ssh/authorized_keys"
chmod 0600 "/home/${ADMIN_USER}/.ssh/authorized_keys"

printf '%s ALL=(ALL:ALL) NOPASSWD:ALL\n' "${ADMIN_USER}" > "/etc/sudoers.d/${ADMIN_USER}"
chmod 0440 "/etc/sudoers.d/${ADMIN_USER}"
visudo -cf "/etc/sudoers.d/${ADMIN_USER}" >/dev/null

if ! id "${SERVICE_USER}" >/dev/null 2>&1; then
  useradd --system --create-home --home-dir /var/lib/deploycrate \
    --shell /usr/sbin/nologin --comment "DeployCrate service account" "${SERVICE_USER}"
else
  usermod --shell /usr/sbin/nologin "${SERVICE_USER}"
fi
passwd --lock "${SERVICE_USER}" >/dev/null
install -d -m 0750 -o "${SERVICE_USER}" -g "${SERVICE_USER}" /var/lib/deploycrate

printf '%s ALL=(ALL:ALL) NOPASSWD:ALL\n' "${SERVICE_USER}" > "/etc/sudoers.d/${SERVICE_USER}"
chmod 0440 "/etc/sudoers.d/${SERVICE_USER}"
visudo -cf "/etc/sudoers.d/${SERVICE_USER}" >/dev/null
