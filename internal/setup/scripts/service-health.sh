#!/usr/bin/env bash
set -euo pipefail

: "${DATABASE_EXTERNAL:?DATABASE_EXTERNAL is required}"
: "${CLICKHOUSE_PASSWORD:?CLICKHOUSE_PASSWORD is required}"
: "${ADMIN_USER:?ADMIN_USER is required}"
: "${SERVICE_USER:?SERVICE_USER is required}"

if [ "$(getent passwd "${SERVICE_USER}" | cut -d: -f7)" != /usr/sbin/nologin ]; then
  printf 'Service account must use /usr/sbin/nologin: %s\n' "${SERVICE_USER}" >&2
  exit 1
fi
if [ "$(passwd --status "${SERVICE_USER}" | awk '{print $2}')" != L ]; then
  printf 'Service account password is not locked: %s\n' "${SERVICE_USER}" >&2
  exit 1
fi
if ! sudo --user "${SERVICE_USER}" --non-interactive sudo --non-interactive true; then
  printf 'Service account does not have unrestricted passwordless sudo: %s\n' "${SERVICE_USER}" >&2
  exit 1
fi
sshd_config="$(sshd -T)"
grep -Eq "^allowusers ([^ ]+ )*${ADMIN_USER}( |$)" <<<"${sshd_config}"
if grep -Eq "^allowusers ([^ ]+ )*${SERVICE_USER}( |$)" <<<"${sshd_config}"; then
  printf 'Service account is unexpectedly allowed through SSH: %s\n' "${SERVICE_USER}" >&2
  exit 1
fi

for unit in wg-quick@wg0.service node-exporter.service docker.service caddy.service prometheus.service cadvisor.service deploycrate-ce@blue.service; do
  systemctl is-active --quiet "${unit}" || {
    printf 'Required service is not active: %s\n' "${unit}" >&2
    exit 1
  }
done

if [ "${DATABASE_EXTERNAL}" = false ]; then
  docker exec deploycrate-ce-postgres pg_isready >/dev/null
fi
docker exec deploycrate-ce-clickhouse clickhouse-client \
  --user deploycrate --password "${CLICKHOUSE_PASSWORD}" --query 'SELECT 1' >/dev/null

ip -4 address show dev wg0 | grep -Fq 'inet 10.99.0.1/16'
wg show wg0 >/dev/null
ss -lnt | grep -Fq '10.99.0.1:9100'
if ss -lnt | grep -Eq '(^|[[:space:]])(0\.0\.0\.0|\*):9100([[:space:]]|$)'; then
  printf 'node-exporter is exposed outside WireGuard\n' >&2
  exit 1
fi
ss -lnt | grep -Fq '127.0.0.1:9090'
ss -lnt | grep -Fq '127.0.0.1:9101'
if ss -lnt | grep -Eq '(^|[[:space:]])(0\.0\.0\.0|\*):9101([[:space:]]|$)'; then
  printf 'cAdvisor is exposed outside localhost\n' >&2
  exit 1
fi
ss -lnt | grep -Fq '127.0.0.1:8123'
curl --fail --silent http://10.99.0.1:9100/metrics >/dev/null
curl --fail --silent http://127.0.0.1:9090/-/ready >/dev/null
curl --fail --silent http://127.0.0.1:9101/healthz >/dev/null
prometheus_targets="$(curl --fail --silent http://127.0.0.1:9090/api/v1/targets)"
grep -Fq '"health":"up"' <<<"${prometheus_targets}"
if grep -Fq '"health":"down"' <<<"${prometheus_targets}"; then
  printf 'One or more Prometheus targets are down\n' >&2
  exit 1
fi
curl --fail --silent --user "deploycrate:${CLICKHOUSE_PASSWORD}" http://127.0.0.1:8123/ping | grep -Fxq 'Ok.'
curl --fail --silent http://127.0.0.1:8080/api/health >/dev/null
