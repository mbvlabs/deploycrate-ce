#!/usr/bin/env bash
set -euo pipefail

base_url="${DEPLOYCRATE_INSTALLER_BASE_URL:-https://get-dev.deploycrate.com}"
base_url="${base_url%/}"

fail() { printf 'DeployCrate installer: %s\n' "$1" >&2; exit 1; }
[ "$(id -u)" -eq 0 ] || fail "run with curl -fsSL ${base_url}/install.sh | sudo bash"
[ "$(uname -s)" = Linux ] || fail "only Linux is supported"
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) fail "unsupported architecture $(uname -m)" ;;
esac

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
curl -fsSL "${base_url}/manifest.json" -o "${work_dir}/manifest.json"
release_base_url="$(sed -n 's/.*"releaseBaseUrl": "\([^"]*\)".*/\1/p' "${work_dir}/manifest.json")"
case "${release_base_url}" in https://*) ;; *) fail "release manifest is invalid" ;; esac
artifact_url="${release_base_url}/linux/${arch}/bootstrap"
curl -fsSL "${artifact_url}" -o "${work_dir}/bootstrap"
curl -fsSL "${artifact_url}.sha256" -o "${work_dir}/bootstrap.sha256"
(cd "${work_dir}" && sha256sum --check --status bootstrap.sha256) || fail "checksum verification failed"
install -m 0755 "${work_dir}/bootstrap" /usr/local/bin/bootstrap

if [ "${DEPLOYCRATE_INSTALL_ONLY:-0}" = 1 ]; then exit 0; fi
if [ -r /dev/tty ] && [ -w /dev/tty ]; then
  exec /usr/local/bin/bootstrap install </dev/tty >/dev/tty 2>/dev/tty
fi
printf 'Run sudo bootstrap install from a terminal to continue.\n'
