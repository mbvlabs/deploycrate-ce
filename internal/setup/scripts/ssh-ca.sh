#!/usr/bin/env bash
set -euo pipefail

: "${SERVICE_USER:?SERVICE_USER is required}"
: "${DOMAIN:?DOMAIN is required}"

ca_directory="/var/lib/deploycrate/ssh-ca"
install -d -o "${SERVICE_USER}" -g "${SERVICE_USER}" -m 0700 "${ca_directory}"

for ca_name in user-ca host-ca; do
  if [ ! -s "${ca_directory}/${ca_name}" ]; then
    runuser -u "${SERVICE_USER}" -- ssh-keygen -q -t ed25519 -N '' \
      -C "DeployCrate ${ca_name}" -f "${ca_directory}/${ca_name}"
  fi
  chown "${SERVICE_USER}:${SERVICE_USER}" "${ca_directory}/${ca_name}" "${ca_directory}/${ca_name}.pub"
  chmod 0600 "${ca_directory}/${ca_name}"
  chmod 0644 "${ca_directory}/${ca_name}.pub"
done

install -o root -g root -m 0644 "${ca_directory}/user-ca.pub" /etc/ssh/deploycrate-user-ca.pub
install -o root -g root -m 0644 "${ca_directory}/host-ca.pub" /etc/ssh/deploycrate-host-ca.pub
install -d -o root -g root -m 0755 /etc/ssh/ssh_config.d
printf '@cert-authority 10.99.* %s\n' "$(<"${ca_directory}/host-ca.pub")" \
  > /etc/ssh/deploycrate-known-hosts
chmod 0644 /etc/ssh/deploycrate-known-hosts
cat > /etc/ssh/ssh_config.d/99-deploycrate-ce.conf <<'EOF'
Host 10.99.*
    GlobalKnownHostsFile /etc/ssh/ssh_known_hosts /etc/ssh/deploycrate-known-hosts
EOF
chmod 0644 /etc/ssh/ssh_config.d/99-deploycrate-ce.conf

host_key="/etc/ssh/ssh_host_ed25519_key"
if [ ! -s "${host_key}" ]; then
  ssh-keygen -q -t ed25519 -N '' -f "${host_key}"
fi
ssh-keygen -q -s "${ca_directory}/host-ca" \
  -I "deploycrate-control-plane-host" -h \
  -n "$(hostname),${DOMAIN},10.99.0.1" -V '-5m:+52w' "${host_key}.pub"

ssh -G 10.99.0.2 | grep -Fq 'globalknownhostsfile /etc/ssh/ssh_known_hosts /etc/ssh/deploycrate-known-hosts'
