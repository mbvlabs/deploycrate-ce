#!/usr/bin/env bash
set -euo pipefail

dev_base_url="${DEPLOYCRATE_DEV_RELEASE_BASE_URL:-https://get-dev.deploycrate.com}"
remote="${DEPLOYCRATE_DEV_RELEASE_REMOTE:-dc-ce-dev:deploycrate-development}"
version="development-$(git describe --always --dirty)"
output_root="dist/deploycrate-ce-development"
app_output="${output_root}/dc-ce-app"

command -v rclone >/dev/null 2>&1 || {
  echo "rclone is required to publish development releases" >&2
  exit 1
}

rm -rf "${output_root}"
install -d "${app_output}"

pnpm install --frozen-lockfile
pnpm build

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.appVersion=${version}" \
  -o "${app_output}/deploycrate-ce" ./cmd/app

(
  cd "${app_output}"
  sha256sum deploycrate-ce > deploycrate-ce.sha256
)

rclone sync "${output_root}/" "${remote}/" \
  --progress \
  --exclude '.git/**'

printf 'Published %s to %s (%s/dc-ce-app/deploycrate-ce)\n' \
  "${version}" "${remote}" "${dev_base_url}"
