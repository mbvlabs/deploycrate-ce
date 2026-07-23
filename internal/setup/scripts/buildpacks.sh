#!/usr/bin/env bash
set -euo pipefail

: "${USERNAME:?USERNAME is required}"

pack_version="0.40.6"
architecture="$(dpkg --print-architecture)"

case "${architecture}" in
  amd64)
    archive="pack-v${pack_version}-linux.tgz"
    checksum="49fb874f7a930653834e67c16917369f9438080440194a6418421b1711421028"
    ;;
  arm64)
    archive="pack-v${pack_version}-linux-arm64.tgz"
    checksum="6ccff07f190a0ac5edec9cd3c1bc0a7192a9b5138147544adcdf2491efab0946"
    ;;
  *)
    printf 'Unsupported architecture for pack: %s\n' "${architecture}" >&2
    exit 1
    ;;
esac

if [ ! -x /usr/local/bin/pack ] || ! /usr/local/bin/pack version 2>/dev/null | grep -Fq "${pack_version}"; then
  temporary_directory="$(mktemp -d)"
  trap 'rm -rf "${temporary_directory}"' EXIT

  download_url="https://github.com/buildpacks/pack/releases/download/v${pack_version}/${archive}"
  curl --fail --silent --show-error --location --retry 3 \
    --output "${temporary_directory}/${archive}" \
    "${download_url}"

  printf '%s  %s\n' "${checksum}" "${temporary_directory}/${archive}" | sha256sum --check --status
  tar --extract --gzip --file "${temporary_directory}/${archive}" --directory "${temporary_directory}" pack
  install -m 0755 "${temporary_directory}/pack" /usr/local/bin/pack
fi

install -d -m 0750 -o "${USERNAME}" -g "${USERNAME}" /var/lib/deploycrate-builds

runuser -u "${USERNAME}" -- /usr/local/bin/pack version
runuser -u "${USERNAME}" -- docker info >/dev/null
