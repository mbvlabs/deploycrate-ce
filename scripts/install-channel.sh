#!/usr/bin/env bash
set -euo pipefail

base_url="${DEPLOYCRATE_INSTALLER_BASE_URL:-https://ce-stable.deploycrate.com}"
base_url="${base_url%/}"
release_channel="stable"
cosign_version="3.0.6"
cosign_path="/usr/local/lib/deploycrate-ce/cosign"
sigstore_issuer="https://token.actions.githubusercontent.com"

fail() { printf 'DeployCrate installer: %s\n' "$1" >&2; exit 1; }
[ "$(id -u)" -eq 0 ] || fail "run with curl -fsSL ${base_url}/install.sh | sudo bash"
[ "$(uname -s)" = Linux ] || fail "only Linux is supported"
case "$(uname -m)" in
  x86_64|amd64)
    arch=amd64
    cosign_sha256=c956e5dfcac53d52bcf058360d579472f0c1d2d9b69f55209e256fe7783f4c74
    ;;
  aarch64|arm64)
    arch=arm64
    cosign_sha256=bedac92e8c3729864e13d4a17048007cfafa79d5deca993a43a90ffe018ef2b8
    ;;
  *) fail "unsupported architecture $(uname -m)" ;;
esac
case "${base_url}" in https://*) ;; *) fail "release base URL must use HTTPS" ;; esac
case "${release_channel}" in
  stable)
    trusted_identity_regexp='^https://github\.com/mbvlabs/deploycrate-ce/\.github/workflows/publish-stable\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$'
    ;;
  edge)
    trusted_identity_regexp='^https://github\.com/mbvlabs/deploycrate-ce/\.github/workflows/publish-edge\.yml@refs/heads/master$'
    ;;
  *) fail "release channel is invalid" ;;
esac

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
curl -fsSL --proto '=https' --tlsv1.2 \
  "https://github.com/sigstore/cosign/releases/download/v${cosign_version}/cosign-linux-${arch}" \
  -o "${work_dir}/cosign"
(cd "${work_dir}" && printf '%s  cosign\n' "${cosign_sha256}" | sha256sum --check --status) || \
  fail "cosign checksum verification failed"
install -d -m 0755 "$(dirname "${cosign_path}")"
install -m 0755 "${work_dir}/cosign" "${cosign_path}"

curl -fsSL --proto '=https' --tlsv1.2 \
  "${base_url}/manifest.json" -o "${work_dir}/manifest.json"
curl -fsSL --proto '=https' --tlsv1.2 \
  "${base_url}/manifest.json.sigstore.json" -o "${work_dir}/manifest.json.sigstore.json"
"${cosign_path}" verify-blob "${work_dir}/manifest.json" \
  --bundle "${work_dir}/manifest.json.sigstore.json" \
  --certificate-identity-regexp "${trusted_identity_regexp}" \
  --certificate-oidc-issuer "${sigstore_issuer}" >/dev/null || \
  fail "release manifest signature verification failed"
manifest_channel="$(sed -n 's/.*"channel": "\([^"]*\)".*/\1/p' "${work_dir}/manifest.json")"
[ "${manifest_channel}" = "${release_channel}" ] || fail "release manifest channel is invalid"
release_base_url="$(sed -n 's/.*"releaseBaseUrl": "\([^"]*\)".*/\1/p' "${work_dir}/manifest.json")"
case "${release_base_url}" in "${base_url}"/releases/*) ;; *) fail "release manifest is invalid" ;; esac
artifact_url="${release_base_url}/linux/${arch}/bootstrap"
curl -fsSL --proto '=https' --tlsv1.2 "${artifact_url}" -o "${work_dir}/bootstrap"
curl -fsSL --proto '=https' --tlsv1.2 \
  "${artifact_url}.sigstore.json" -o "${work_dir}/bootstrap.sigstore.json"
"${cosign_path}" verify-blob "${work_dir}/bootstrap" \
  --bundle "${work_dir}/bootstrap.sigstore.json" \
  --certificate-identity-regexp "${trusted_identity_regexp}" \
  --certificate-oidc-issuer "${sigstore_issuer}" >/dev/null || \
  fail "bootstrap signature verification failed"
curl -fsSL --proto '=https' --tlsv1.2 \
  "${artifact_url}.sha256" -o "${work_dir}/bootstrap.sha256"
(cd "${work_dir}" && sha256sum --check --status bootstrap.sha256) || fail "checksum verification failed"
install -m 0755 "${work_dir}/bootstrap" /usr/local/bin/bootstrap

if [ "${DEPLOYCRATE_INSTALL_ONLY:-0}" = 1 ]; then exit 0; fi
if [ -r /dev/tty ] && [ -w /dev/tty ]; then
  exec /usr/local/bin/bootstrap install </dev/tty >/dev/tty 2>/dev/tty
fi
printf 'Run sudo bootstrap install from a terminal to continue.\n'
