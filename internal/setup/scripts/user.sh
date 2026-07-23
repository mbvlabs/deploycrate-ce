#!/usr/bin/env bash
set -euo pipefail

: "${USERNAME:?USERNAME is required}"
: "${PASSWORD:?PASSWORD is required}"
: "${SSH_PUBLIC_KEY:?SSH_PUBLIC_KEY is required}"

if ! id "${USERNAME}" >/dev/null 2>&1; then
  useradd --create-home --shell /bin/bash "${USERNAME}"
fi

printf '%s:%s\n' "${USERNAME}" "${PASSWORD}" | chpasswd
usermod --append --groups sudo "${USERNAME}"

install -d -m 0700 -o "${USERNAME}" -g "${USERNAME}" "/home/${USERNAME}/.ssh"
printf '%s\n' "${SSH_PUBLIC_KEY}" > "/home/${USERNAME}/.ssh/authorized_keys"
chown "${USERNAME}:${USERNAME}" "/home/${USERNAME}/.ssh/authorized_keys"
chmod 0600 "/home/${USERNAME}/.ssh/authorized_keys"

printf '%s ALL=(ALL:ALL) NOPASSWD:ALL\n' "${USERNAME}" > "/etc/sudoers.d/${USERNAME}"
chmod 0440 "/etc/sudoers.d/${USERNAME}"
visudo -cf "/etc/sudoers.d/${USERNAME}" >/dev/null
