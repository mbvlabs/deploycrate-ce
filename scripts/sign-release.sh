#!/usr/bin/env bash
set -euo pipefail

release_root="${1:?usage: scripts/sign-release.sh RELEASE_ROOT}"
issuer="https://token.actions.githubusercontent.com"
workflow_ref="${GITHUB_WORKFLOW_REF:?GITHUB_WORKFLOW_REF is required}"
identity="https://github.com/${workflow_ref}"

shopt -s nullglob
release_directories=("${release_root}"/releases/*)
if [ "${#release_directories[@]}" -ne 1 ] || [ ! -d "${release_directories[0]}" ]; then
  echo "release root must contain exactly one version directory" >&2
  exit 1
fi
version_root="${release_directories[0]}"

artifacts=("${release_root}/manifest.json")
for arch in amd64 arm64; do
  artifacts+=(
    "${version_root}/linux/${arch}/deploycrate-ce"
    "${version_root}/linux/${arch}/bootstrap"
  )
done

for artifact in "${artifacts[@]}"; do
  [ -f "${artifact}" ] || { echo "release artifact is missing: ${artifact}" >&2; exit 1; }
  cosign sign-blob --yes --bundle "${artifact}.sigstore.json" "${artifact}"
  cosign verify-blob "${artifact}" \
    --bundle "${artifact}.sigstore.json" \
    --certificate-identity "${identity}" \
    --certificate-oidc-issuer "${issuer}"
done
