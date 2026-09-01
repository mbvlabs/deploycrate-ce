#!/usr/bin/env bash
set -euo pipefail

release_root="${1:?usage: scripts/upload-release.sh RELEASE_ROOT BUCKET ENDPOINT}"
bucket="${2:?usage: scripts/upload-release.sh RELEASE_ROOT BUCKET ENDPOINT}"
endpoint="${3:?usage: scripts/upload-release.sh RELEASE_ROOT BUCKET ENDPOINT}"

aws s3 cp "${release_root}/releases" "s3://${bucket}/releases" --recursive \
  --cache-control 'public,max-age=31536000,immutable' --endpoint-url "${endpoint}"
aws s3 cp "${release_root}/install.sh" "s3://${bucket}/install.sh" \
  --cache-control no-cache --content-type text/x-shellscript --endpoint-url "${endpoint}"
aws s3 cp "${release_root}/manifest.json.sigstore.json" \
  "s3://${bucket}/manifest.json.sigstore.json" \
  --content-type application/json --cache-control no-cache \
  --endpoint-url "${endpoint}"
aws s3 cp "${release_root}/manifest.json" "s3://${bucket}/manifest.json" \
  --content-type application/json --cache-control no-cache \
  --endpoint-url "${endpoint}"
