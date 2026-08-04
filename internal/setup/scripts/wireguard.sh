#!/usr/bin/env bash
set -euo pipefail

: "${WG_ADDRESS:?WG_ADDRESS is required}"
: "${WG_LISTEN_PORT:?WG_LISTEN_PORT is required}"

wireguard_directory="/etc/wireguard"
private_key_path="${wireguard_directory}/deploycrate-ce.key"
public_key_path="${wireguard_directory}/deploycrate-ce.pub"
configuration_path="${wireguard_directory}/wg0.conf"
forwarding_configuration_path="/etc/sysctl.d/99-deploycrate-wireguard-gateway.conf"

install -d -m 0700 "${wireguard_directory}"

temporary_private=""
temporary_public=""
temporary_configuration=""
cleanup() {
  [ -z "${temporary_private}" ] || rm -f "${temporary_private}"
  [ -z "${temporary_public}" ] || rm -f "${temporary_public}"
  [ -z "${temporary_configuration}" ] || rm -f "${temporary_configuration}"
}
trap cleanup EXIT

if [ ! -s "${private_key_path}" ]; then
  temporary_private="$(mktemp "${wireguard_directory}/deploycrate-ce.key.XXXXXX")"
  wg genkey > "${temporary_private}"
  chmod 0600 "${temporary_private}"
  mv "${temporary_private}" "${private_key_path}"
  temporary_private=""
fi

temporary_public="$(mktemp "${wireguard_directory}/deploycrate-ce.pub.XXXXXX")"
wg pubkey < "${private_key_path}" > "${temporary_public}"
chmod 0600 "${temporary_public}"
mv "${temporary_public}" "${public_key_path}"
temporary_public=""

private_key="$(<"${private_key_path}")"
temporary_configuration="$(mktemp "${wireguard_directory}/wg0.conf.XXXXXX")"
cat > "${temporary_configuration}" <<EOF
[Interface]
PrivateKey = ${private_key}
Address = ${WG_ADDRESS}
ListenPort = ${WG_LISTEN_PORT}
EOF
chmod 0600 "${temporary_configuration}"
mv "${temporary_configuration}" "${configuration_path}"
temporary_configuration=""
unset private_key

cat > "${forwarding_configuration_path}" <<'EOF'
net.ipv4.ip_forward = 1
EOF
chmod 0644 "${forwarding_configuration_path}"
sysctl --system >/dev/null

ufw allow "${WG_LISTEN_PORT}/udp"
systemctl enable wg-quick@wg0
systemctl restart wg-quick@wg0
wg show wg0
