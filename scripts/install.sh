#!/usr/bin/env bash
set -euo pipefail

repository="${DEPLOYCRATE_RELEASE_REPOSITORY:-mbvlabs/deploycrate-ce-cli}"
requested_version="${DEPLOYCRATE_VERSION:-latest}"
requested_base_url="${DEPLOYCRATE_RELEASE_BASE_URL:-}"

fail() {
  printf 'deploycrate installer: %s\n' "$1" >&2
  exit 1
}

[ "$(id -u)" -eq 0 ] || fail "run with curl -fsSL https://get.deploycrate.com/ce | sudo bash"

case "$(uname -s)" in
  Linux) os="linux" ;;
  *) fail "only Linux is supported" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) fail "unsupported architecture $(uname -m)" ;;
esac

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

if [ "$requested_version" = "latest" ]; then
  release_path="latest/download"
  version="latest"
else
  version="${requested_version#v}"
  release_path="download/v${version}"
fi

archive="deploycrate-ce_${os}_${arch}.tar.gz"
if [ -n "${requested_base_url}" ]; then
  base_url="${requested_base_url%/}"
else
  base_url="https://github.com/${repository}/releases/${release_path}"
fi

curl -fL "${base_url}/${archive}" -o "${work_dir}/${archive}"
curl -fL "${base_url}/checksums.txt" -o "${work_dir}/checksums.txt"

(
  cd "$work_dir"
  grep " ${archive}$" checksums.txt | sha256sum --check --status
) || fail "release checksum verification failed"

if command -v cosign >/dev/null 2>&1; then
  curl -fL "${base_url}/checksums.txt.sigstore.json" -o "${work_dir}/checksums.txt.sigstore.json"
  cosign verify-blob \
    --certificate-identity-regexp 'https://github.com/.+/.github/workflows/.+' \
    --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
    --bundle "${work_dir}/checksums.txt.sigstore.json" \
    "${work_dir}/checksums.txt" >/dev/null
else
  printf 'deploycrate installer: cosign is not installed, checksum verified but signature verification was skipped\n' >&2
fi

tar -xzf "${work_dir}/${archive}" -C "$work_dir"
install -m 0755 "${work_dir}/deploycrate" /usr/local/bin/deploycrate
install -m 0755 "${work_dir}/deploycrate-ce" /usr/local/bin/deploycrate-ce

printf 'DeployCrate CE CLI installed at /usr/local/bin/deploycrate\n'
if [ -r /dev/tty ] && [ -w /dev/tty ]; then
  exec /usr/local/bin/deploycrate install </dev/tty >/dev/tty 2>/dev/tty
fi
printf 'Run sudo deploycrate install from a terminal to continue.\n'
