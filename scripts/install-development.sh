#!/usr/bin/env bash
set -euo pipefail

base_url="${DEPLOYCRATE_DEVELOPMENT_BASE_URL:-https://get-dev.deploycrate.com}"
base_url="${base_url%/}"

fail() {
  printf 'bootstrap development installer: %s\n' "$1" >&2
  exit 1
}

[ "$(id -u)" -eq 0 ] || fail "run with curl -fsSL ${base_url}/install.sh | sudo bash"

case "$(uname -s)" in
  Linux) ;;
  *) fail "only Linux is supported" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) ;;
  *) fail "only AMD64 is supported by the development build" ;;
esac

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

download() {
  component="$1"
  binary="$2"

  curl -fsSL "${base_url}/${component}/${binary}" -o "${work_dir}/${binary}"
  curl -fsSL "${base_url}/${component}/${binary}.sha256" -o "${work_dir}/${binary}.sha256"
  (
    cd "${work_dir}"
    sha256sum --check --status "${binary}.sha256"
  ) || fail "checksum verification failed for ${binary}"
}

download "dc-ce-cli" "bootstrap"

install -m 0755 "${work_dir}/bootstrap" /usr/local/bin/bootstrap

printf 'DeployCrate CE development bootstrap installed\n'
if [ "${DEPLOYCRATE_INSTALL_ONLY:-0}" = "1" ]; then
  exit 0
fi
if [ -r /dev/tty ] && [ -w /dev/tty ]; then
  exec /usr/local/bin/bootstrap install </dev/tty >/dev/tty 2>/dev/tty
fi
printf 'Run sudo bootstrap install from a terminal to continue.\n'
