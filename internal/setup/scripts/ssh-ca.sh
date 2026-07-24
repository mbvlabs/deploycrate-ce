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

install -d -o root -g root -m 0755 /etc/deploycrate-ce
printf 'DOMAIN=%q\n' "${DOMAIN}" > /etc/deploycrate-ce/ssh-host-certificate.env
chmod 0644 /etc/deploycrate-ce/ssh-host-certificate.env

cat > /usr/local/sbin/deploycrate-renew-ssh-host-certificate <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

source /etc/deploycrate-ce/ssh-host-certificate.env
: "${DOMAIN:?DOMAIN is required}"

ca_directory="/var/lib/deploycrate/ssh-ca"
host_key="/etc/ssh/ssh_host_ed25519_key"
temporary_directory="$(mktemp -d)"
trap 'rm -rf "${temporary_directory}"' EXIT

install -o root -g root -m 0644 "${host_key}.pub" "${temporary_directory}/host.pub"
ssh-keygen -q -s "${ca_directory}/host-ca" \
  -I "deploycrate-control-plane-host" -h \
  -n "$(hostname),${DOMAIN},10.99.0.1" -V '-5m:+52w' "${temporary_directory}/host.pub"
install -o root -g root -m 0644 \
  "${temporary_directory}/host-cert.pub" "${host_key}-cert.pub"
ssh-keygen -L -f "${host_key}-cert.pub" >/dev/null

if systemctl is-active --quiet ssh.service; then
  systemctl reload ssh.service
fi
EOF
chmod 0755 /usr/local/sbin/deploycrate-renew-ssh-host-certificate

cat > /etc/systemd/system/deploycrate-renew-ssh-host-certificate.service <<'EOF'
[Unit]
Description=Renew the DeployCrate SSH host certificate
After=ssh.service

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/deploycrate-renew-ssh-host-certificate
EOF

cat > /etc/systemd/system/deploycrate-renew-ssh-host-certificate.timer <<'EOF'
[Unit]
Description=Monthly renewal of the DeployCrate SSH host certificate

[Timer]
OnCalendar=monthly
RandomizedDelaySec=1d
Persistent=true

[Install]
WantedBy=timers.target
EOF

/usr/local/sbin/deploycrate-renew-ssh-host-certificate
systemctl daemon-reload
systemctl enable --now deploycrate-renew-ssh-host-certificate.timer

ssh -G 10.99.0.2 | grep -Fq 'globalknownhostsfile /etc/ssh/ssh_known_hosts /etc/ssh/deploycrate-known-hosts'
