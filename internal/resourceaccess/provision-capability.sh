#!/usr/bin/env bash
set -euo pipefail

[ "$(id -u)" = "0" ] || { printf 'server capability provisioning must run as root\n' >&2; exit 1; }
[ -r /etc/os-release ] && . /etc/os-release
[ "${ID:-}" = "debian" ] || { printf 'Only Debian hosts are supported\n' >&2; exit 1; }
[ -n "${DEPLOYCRATE_CAPABILITIES:-}" ] || { printf 'DEPLOYCRATE_CAPABILITIES is required\n' >&2; exit 1; }

if ! id deploycrate >/dev/null 2>&1; then
  useradd --system --create-home --home-dir /var/lib/deploycrate --shell /usr/sbin/nologin deploycrate
fi
if getent group docker >/dev/null 2>&1; then
  usermod --append --groups docker deploycrate
fi

provision_build() {
  architecture="$(dpkg --print-architecture)"
  case "${architecture}" in
    amd64) pack_archive="pack-v0.40.6-linux.tgz"; pack_checksum="49fb874f7a930653834e67c16917369f9438080440194a6418421b1711421028" ;;
    arm64) pack_archive="pack-v0.40.6-linux-arm64.tgz"; pack_checksum="6ccff07f190a0ac5edec9cd3c1bc0a7192a9b5138147544adcdf2491efab0946" ;;
    *) printf 'Unsupported architecture: %s\n' "${architecture}" >&2; exit 1 ;;
  esac
  temporary_directory="$(mktemp -d)"
  trap 'rm -rf "${temporary_directory}"' EXIT
  curl -fsSL --retry 3 -o "${temporary_directory}/${pack_archive}" "https://github.com/buildpacks/pack/releases/download/v0.40.6/${pack_archive}"
  printf '%s  %s\n' "${pack_checksum}" "${temporary_directory}/${pack_archive}" | sha256sum --check --status
  tar -xzf "${temporary_directory}/${pack_archive}" -C "${temporary_directory}" pack
  install -m 0755 "${temporary_directory}/pack" /usr/local/bin/pack
  install -d -o deploycrate -g deploycrate -m 0750 /var/lib/deploycrate-builds
}

provision_runtime() {
  install -d -o deploycrate -g deploycrate -m 0750 /var/lib/deploycrate-applications
}

provision_resource() {
  install -d -o deploycrate -g deploycrate -m 0750 /var/lib/deploycrate-resources
}

provision_database() {
  install -d -o deploycrate -g deploycrate -m 0750 /var/lib/deploycrate-databases
}

provision_repository() {
  install -d -o deploycrate -g deploycrate -m 0750 /var/lib/deploycrate-repositories
}

enabled=0
IFS=',' read -r -a requested <<< "${DEPLOYCRATE_CAPABILITIES}"
for capability in "${requested[@]}"; do
  case "${capability}" in
    build) provision_build ;;
    runtime) provision_runtime ;;
    resource) provision_resource ;;
    database) provision_database ;;
    repository) provision_repository ;;
    *) printf 'Unknown capability: %s\n' "${capability}" >&2; exit 1 ;;
  esac
  enabled=1
done
[ "${enabled}" = "1" ] || { printf 'at least one server capability is required\n' >&2; exit 1; }

printf 'capability provisioning completed\n'
