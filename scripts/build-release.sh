#!/usr/bin/env bash
set -euo pipefail

version="${1:?usage: scripts/build-release.sh VERSION CHANNEL BASE_URL}"
channel="${2:?usage: scripts/build-release.sh VERSION CHANNEL BASE_URL}"
base_url="${3:?usage: scripts/build-release.sh VERSION CHANNEL BASE_URL}"

case "${channel}" in stable|edge) ;; *) echo "channel must be stable or edge" >&2; exit 1 ;; esac
if [ "${channel}" = stable ] && [[ ! "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "stable versions must use MAJOR.MINOR.PATCH" >&2
  exit 1
fi
base_url="${base_url%/}"
output="dist/release-${channel}"
rm -rf "${output}"
mkdir -p "${output}/releases/${version}/linux"

pnpm install --frozen-lockfile
pnpm build

for arch in amd64 arm64; do
  platform_dir="${output}/releases/${version}/linux/${arch}"
  mkdir -p "${platform_dir}"

  CGO_ENABLED=0 GOOS=linux GOARCH="${arch}" go build -trimpath \
    -ldflags "-s -w -X main.appVersion=${version}" \
    -o "${platform_dir}/deploycrate-ce" ./cmd/app
  CGO_ENABLED=0 GOOS=linux GOARCH="${arch}" go build -trimpath \
    -ldflags "-s -w -X main.version=${version}" \
    -o "${platform_dir}/bootstrap" ./cmd/bootstrap

  (cd "${platform_dir}" && sha256sum deploycrate-ce > deploycrate-ce.sha256 && sha256sum bootstrap > bootstrap.sha256)
done

sed "s|https://get.deploycrate.com|${base_url}|g" scripts/install-channel.sh > "${output}/install.sh"
chmod 0755 "${output}/install.sh"
amd64_sha="$(sha256sum "${output}/releases/${version}/linux/amd64/deploycrate-ce" | awk '{print $1}')"
arm64_sha="$(sha256sum "${output}/releases/${version}/linux/arm64/deploycrate-ce" | awk '{print $1}')"
cat > "${output}/manifest.json" <<JSON
{
  "schemaVersion": 1,
  "channel": "${channel}",
  "version": "${version}",
  "releaseBaseUrl": "${base_url}/releases/${version}",
  "artifacts": {
    "linux/amd64": {
      "url": "${base_url}/releases/${version}/linux/amd64/deploycrate-ce",
      "sha256": "${amd64_sha}"
    },
    "linux/arm64": {
      "url": "${base_url}/releases/${version}/linux/arm64/deploycrate-ce",
      "sha256": "${arm64_sha}"
    }
  }
}
JSON
printf 'Built %s %s release in %s\n' "${channel}" "${version}" "${output}"
