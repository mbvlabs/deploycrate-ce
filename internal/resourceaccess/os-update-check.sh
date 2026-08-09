#!/usr/bin/env bash
set -euo pipefail

DEBIAN_FRONTEND=noninteractive apt-get update -qq

plan="$(DEBIAN_FRONTEND=noninteractive apt-get -s -y -o APT::Get::Show-Upgraded=true upgrade)"
entries=""
total=0
while IFS= read -r line; do
  case "${line}" in
    "Inst "*)
      fields=(${line#"Inst "})
      name="${fields[0]}"
      installed=""
      available=""
      for field in "${fields[@]:1}"; do
        if [[ "${field}" == "["*"]" ]]; then
          installed="${field#[}"
          installed="${installed%]}"
        elif [[ "${field}" == "("* ]]; then
          available="${field#(}"
          available="${available%% *}"
          break
        fi
      done
      if [ -n "${available}" ]; then
        entry=$(printf '{"name":"%s","installed":"%s","available":"%s"}' "${name}" "${installed}" "${available}")
        if [ -n "${entries}" ]; then
          entries+=","
        fi
        entries+="${entry}"
        total=$((total+1))
      fi
      ;;
  esac
done <<< "${plan}"

reboot_required="false"
if [ -f /var/run/reboot-required ]; then
  reboot_required="true"
fi
printf '{"rebootRequired":%s,"total":%d,"updates":[%s]}\n' "${reboot_required}" "${total}" "${entries}"
