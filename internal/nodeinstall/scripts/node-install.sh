#!/usr/bin/env bash
set -euo pipefail

: "${DEPLOYCRATE_SERVER_ID:?DEPLOYCRATE_SERVER_ID is required}"
: "${DEPLOYCRATE_NODE_NAME:?DEPLOYCRATE_NODE_NAME is required}"
: "${DEPLOYCRATE_NODE_NAME_YAML:?DEPLOYCRATE_NODE_NAME_YAML is required}"
: "${DEPLOYCRATE_PRIVATE_ADDRESS:?DEPLOYCRATE_PRIVATE_ADDRESS is required}"
: "${DEPLOYCRATE_WIREGUARD_PORT:?DEPLOYCRATE_WIREGUARD_PORT is required}"
: "${DEPLOYCRATE_SSH_PORT:?DEPLOYCRATE_SSH_PORT is required}"
: "${DEPLOYCRATE_CONTROL_PUBLIC_KEY:?DEPLOYCRATE_CONTROL_PUBLIC_KEY is required}"
: "${DEPLOYCRATE_CONTROL_ADDRESS:?DEPLOYCRATE_CONTROL_ADDRESS is required}"
: "${DEPLOYCRATE_CONTROL_ENDPOINT:?DEPLOYCRATE_CONTROL_ENDPOINT is required}"
: "${DEPLOYCRATE_SSH_USER_CA:?DEPLOYCRATE_SSH_USER_CA is required}"
: "${DEPLOYCRATE_OTLP_ENDPOINT:?DEPLOYCRATE_OTLP_ENDPOINT is required}"
: "${DEPLOYCRATE_TELEMETRY_ISSUER:?DEPLOYCRATE_TELEMETRY_ISSUER is required}"
: "${DEPLOYCRATE_TELEMETRY_JWKS:?DEPLOYCRATE_TELEMETRY_JWKS is required}"
: "${DEPLOYCRATE_TELEMETRY_NODE_TOKEN:?DEPLOYCRATE_TELEMETRY_NODE_TOKEN is required}"
: "${DEPLOYCRATE_CAPABILITY_BUILD:?DEPLOYCRATE_CAPABILITY_BUILD is required}"
: "${DEPLOYCRATE_CAPABILITY_RUNTIME:?DEPLOYCRATE_CAPABILITY_RUNTIME is required}"
: "${DEPLOYCRATE_CAPABILITY_RESOURCE:?DEPLOYCRATE_CAPABILITY_RESOURCE is required}"
: "${DEPLOYCRATE_CAPABILITY_DATABASE:?DEPLOYCRATE_CAPABILITY_DATABASE is required}"
: "${DEPLOYCRATE_CAPABILITY_REPOSITORY:?DEPLOYCRATE_CAPABILITY_REPOSITORY is required}"
: "${DEPLOYCRATE_WIREGUARD_PEERS:?DEPLOYCRATE_WIREGUARD_PEERS is required}"

. /etc/os-release
[ "${ID}" = "debian" ] || { printf 'Only Debian is supported\n' >&2; exit 1; }
[ "${VERSION_ID}" = "13" ] || { printf 'Debian 13 is required\n' >&2; exit 1; }

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq ca-certificates curl docker.io openssh-server openssl sudo tar ufw wireguard-tools

install -d -m 0755 /etc/systemd/journald.conf.d
install -d -m 2755 /var/log/journal
cat > /etc/systemd/journald.conf.d/deploycrate-node.conf <<'EOF'
[Journal]
Storage=persistent
Compress=yes
SystemMaxUse=1G
SystemKeepFree=1G
RuntimeMaxUse=256M
RuntimeKeepFree=256M
MaxRetentionSec=14day
EOF
systemctl restart systemd-journald

install -d -m 0755 /etc/docker
cat > /etc/docker/daemon.json <<'EOF'
{
  "log-driver": "journald",
  "log-opts": {
    "tag": "{{.Name}}",
    "labels": "com.deploycrate.application,com.deploycrate.environment,com.deploycrate.target,com.deploycrate.deployment,com.deploycrate.instance,com.deploycrate.release,com.deploycrate.resource-installation,com.deploycrate.component"
  },
  "live-restore": true
}
EOF
install -d -m 0755 /etc/systemd/system/docker.service.d
cat > /etc/systemd/system/docker.service.d/deploycrate-node-accounting.conf <<'EOF'
[Service]
CPUAccounting=yes
MemoryAccounting=yes
IOAccounting=yes
TasksAccounting=yes
EOF

if ! id admin >/dev/null 2>&1; then
  useradd --create-home --shell /bin/bash admin
fi
if ! id deploycrate >/dev/null 2>&1; then
  useradd --system --create-home --home-dir /var/lib/deploycrate --shell /usr/sbin/nologin deploycrate
fi
install -d -o root -g root -m 0755 /etc/ssh/auth_principals
printf 'admin\n' > /etc/ssh/auth_principals/admin
chmod 0644 /etc/ssh/auth_principals/admin
printf '%s\n' "${DEPLOYCRATE_SSH_USER_CA}" > /etc/ssh/deploycrate-user-ca.pub
chmod 0644 /etc/ssh/deploycrate-user-ca.pub
cat > /etc/ssh/sshd_config.d/98-deploycrate-node.conf <<'EOF'
TrustedUserCAKeys /etc/ssh/deploycrate-user-ca.pub
AuthorizedPrincipalsFile /etc/ssh/auth_principals/%u
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
EOF
printf 'admin ALL=(ALL) NOPASSWD:ALL\n' > /etc/sudoers.d/admin
chmod 0440 /etc/sudoers.d/admin
systemctl reload ssh.service

install -d -o root -g root -m 0700 /etc/wireguard
if [ ! -s /etc/wireguard/deploycrate-node.key ]; then
  umask 077
  wg genkey > /etc/wireguard/deploycrate-node.key
fi
wireguard_private_key="$(cat /etc/wireguard/deploycrate-node.key)"
wireguard_public_key="$(printf '%s' "${wireguard_private_key}" | wg pubkey)"
cat > /etc/wireguard/wg0.conf <<EOF
[Interface]
Address = ${DEPLOYCRATE_PRIVATE_ADDRESS}/16
ListenPort = ${DEPLOYCRATE_WIREGUARD_PORT}
PrivateKey = ${wireguard_private_key}
EOF
printf '%s' "${DEPLOYCRATE_WIREGUARD_PEERS}" | base64 --decode >> /etc/wireguard/wg0.conf
chmod 0600 /etc/wireguard/wg0.conf
systemctl enable --now wg-quick@wg0.service

architecture="$(dpkg --print-architecture)"
case "${architecture}" in
  amd64)
    node_checksum="9f5ea48e5bc7b656f8a91a32e7d7deb89f70f73dabd0d974418aca15f37d6810"
    cadvisor_checksum="9359a1192775eafeead41941690f7d94fb55f5f85833071b70593f8e7eae31ec"
    otel_checksum="d33177515a244a2393f03ffd66ab3e68a8fc11a56bc145ec4d0ca2644ee95504"
    ;;
  arm64)
    node_checksum="ba1886efbd76cb96b0087c695ea8d1b9cb6e8aa946c996d744e9ee16c8e3591a"
    cadvisor_checksum="38477947aab2dc5ff0288d4ee59e2ddb351d3a627140b707416f4b4ee91c1b85"
    otel_checksum="34eb82390c462c877dd60ec5ec84de899088916facd07306ec988e4c34bd05b3"
    ;;
  *) printf 'Unsupported architecture: %s\n' "${architecture}" >&2; exit 1 ;;
esac

temporary_directory="$(mktemp -d)"
trap 'rm -rf "${temporary_directory}"' EXIT

node_archive="node_exporter-1.11.1.linux-${architecture}.tar.gz"
curl -fsSL --retry 3 -o "${temporary_directory}/${node_archive}" "https://github.com/prometheus/node_exporter/releases/download/v1.11.1/${node_archive}"
printf '%s  %s\n' "${node_checksum}" "${temporary_directory}/${node_archive}" | sha256sum --check --status
tar -xzf "${temporary_directory}/${node_archive}" -C "${temporary_directory}"
install -m 0755 "${temporary_directory}/node_exporter-1.11.1.linux-${architecture}/node_exporter" /usr/local/bin/node_exporter
id node_exporter >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin node_exporter
cat > /etc/systemd/system/node-exporter.service <<EOF
[Unit]
After=network-online.target wg-quick@wg0.service
[Service]
User=node_exporter
ExecStart=/usr/local/bin/node_exporter --web.listen-address=${DEPLOYCRATE_PRIVATE_ADDRESS}:9100
Restart=on-failure
NoNewPrivileges=true
[Install]
WantedBy=multi-user.target
EOF

cadvisor_artifact="cadvisor-v0.57.0-linux-${architecture}"
curl -fsSL --retry 3 -o "${temporary_directory}/${cadvisor_artifact}" "https://github.com/google/cadvisor/releases/download/v0.57.0/${cadvisor_artifact}"
printf '%s  %s\n' "${cadvisor_checksum}" "${temporary_directory}/${cadvisor_artifact}" | sha256sum --check --status
install -m 0755 "${temporary_directory}/${cadvisor_artifact}" /usr/local/bin/cadvisor
cat > /etc/systemd/system/cadvisor.service <<EOF
[Unit]
After=network-online.target docker.service wg-quick@wg0.service
[Service]
ExecStart=/usr/local/bin/cadvisor --listen_ip=${DEPLOYCRATE_PRIVATE_ADDRESS} --port=9101 --docker_only=true --store_container_labels=false --whitelisted_container_labels=com.deploycrate.application,com.deploycrate.environment,com.deploycrate.target,com.deploycrate.deployment,com.deploycrate.instance,com.deploycrate.release,com.deploycrate.resource-installation,com.deploycrate.component
Restart=on-failure
[Install]
WantedBy=multi-user.target
EOF

otel_archive="otelcol-contrib_0.157.0_linux_${architecture}.tar.gz"
curl -fsSL --retry 3 -o "${temporary_directory}/${otel_archive}" "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.157.0/${otel_archive}"
printf '%s  %s\n' "${otel_checksum}" "${temporary_directory}/${otel_archive}" | sha256sum --check --status
tar -xzf "${temporary_directory}/${otel_archive}" -C "${temporary_directory}" otelcol-contrib
install -m 0755 "${temporary_directory}/otelcol-contrib" /usr/local/bin/otelcol-contrib
id otelcol-contrib >/dev/null 2>&1 || useradd --system --no-create-home --home-dir /var/lib/otelcol-contrib --shell /usr/sbin/nologin otelcol-contrib
usermod --append --groups systemd-journal otelcol-contrib
install -d -o root -g otelcol-contrib -m 0750 /etc/otelcol-contrib
install -d -o otelcol-contrib -g otelcol-contrib -m 0750 /var/lib/otelcol-contrib
printf '%s' "${DEPLOYCRATE_TELEMETRY_JWKS}" | base64 --decode > /etc/otelcol-contrib/telemetry-jwks.json
chown root:otelcol-contrib /etc/otelcol-contrib/telemetry-jwks.json
chmod 0640 /etc/otelcol-contrib/telemetry-jwks.json
printf 'DEPLOYCRATE_TELEMETRY_NODE_TOKEN=%s\n' "${DEPLOYCRATE_TELEMETRY_NODE_TOKEN}" > /etc/otelcol-contrib/environment
chown root:otelcol-contrib /etc/otelcol-contrib/environment
chmod 0640 /etc/otelcol-contrib/environment
cat > /etc/otelcol-contrib/config.yaml <<EOF
receivers:
  journald:
    start_at: end
    units: [docker.service, node-exporter.service, cadvisor.service, wg-quick@wg0.service]
    storage: file_storage
    retry_on_failure:
      enabled: true
      max_elapsed_time: 0
  otlp:
    protocols:
      http:
        endpoint: ${DEPLOYCRATE_PRIVATE_ADDRESS}:4318
        auth:
          authenticator: oidc
processors:
  batch: {}
  transform/workload_logs:
    error_mode: ignore
    log_statements:
      - set(log.attributes["deploycrate.application.id"], log.body["COM_DEPLOYCRATE_APPLICATION"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["deploycrate.environment.id"], log.body["COM_DEPLOYCRATE_ENVIRONMENT"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["deploycrate.target.id"], log.body["COM_DEPLOYCRATE_TARGET"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["deploycrate.deployment.id"], log.body["COM_DEPLOYCRATE_DEPLOYMENT"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["deploycrate.instance.id"], log.body["COM_DEPLOYCRATE_INSTANCE"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["deploycrate.release.id"], log.body["COM_DEPLOYCRATE_RELEASE"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["deploycrate.log.epoch"], log.body["CONTAINER_LOG_EPOCH"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["deploycrate.log.ordinal"], log.body["CONTAINER_LOG_ORDINAL"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["container.id"], log.body["CONTAINER_ID_FULL"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["container.name"], log.body["CONTAINER_NAME"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil
      - set(log.attributes["log.iostream"], "stderr") where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil and log.body["PRIORITY"] == "3"
      - set(log.attributes["log.iostream"], "stdout") where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil and log.body["PRIORITY"] != "3"
      - set(log.body, log.body["MESSAGE"]) where IsMap(log.body) and log.body["COM_DEPLOYCRATE_ENVIRONMENT"] != nil and log.body["MESSAGE"] != nil
      - replace_pattern(log.body, "\\\\x1B\\\\[[0-?]*[ -/]*[@-~]", "") where log.attributes["deploycrate.environment.id"] != nil and IsString(log.body)
  resource/node:
    attributes:
      - key: deploycrate.server.id
        value: "${DEPLOYCRATE_SERVER_ID}"
        action: upsert
      - key: server.id
        value: "${DEPLOYCRATE_SERVER_ID}"
        action: upsert
      - key: host.id
        value: "${DEPLOYCRATE_SERVER_ID}"
        action: insert
      - key: host.name
        value: ${DEPLOYCRATE_NODE_NAME_YAML}
        action: upsert
  resource/authenticated_identity:
    attributes:
      - key: deploycrate.environment.id
        from_context: auth.claims.deploycrate_environment_id
        action: upsert
exporters:
  otlphttp/control-plane:
    endpoint: ${DEPLOYCRATE_OTLP_ENDPOINT}
    headers:
      Authorization: "Bearer \${env:DEPLOYCRATE_TELEMETRY_NODE_TOKEN}"
    sending_queue:
      enabled: true
      storage: file_storage
      queue_size: 10000
    retry_on_failure:
      enabled: true
      max_elapsed_time: 0
extensions:
  file_storage:
    directory: /var/lib/otelcol-contrib
  health_check:
    endpoint: 127.0.0.1:13133
  oidc:
    providers:
      - issuer_url: ${DEPLOYCRATE_TELEMETRY_ISSUER}
        audience: deploycrate-telemetry
        public_keys_file: /etc/otelcol-contrib/telemetry-jwks.json
service:
  extensions: [file_storage, health_check, oidc]
  pipelines:
    logs/journald:
      receivers: [journald]
      processors: [transform/workload_logs, resource/node, batch]
      exporters: [otlphttp/control-plane]
    logs/workloads:
      receivers: [otlp]
      processors: [resource/authenticated_identity, resource/node, batch]
      exporters: [otlphttp/control-plane]
    metrics:
      receivers: [otlp]
      processors: [resource/authenticated_identity, resource/node, batch]
      exporters: [otlphttp/control-plane]
    traces:
      receivers: [otlp]
      processors: [resource/authenticated_identity, resource/node, batch]
      exporters: [otlphttp/control-plane]
EOF
chown root:otelcol-contrib /etc/otelcol-contrib/config.yaml
chmod 0640 /etc/otelcol-contrib/config.yaml
cat > /etc/systemd/system/otelcol-contrib.service <<'EOF'
[Unit]
After=network-online.target wg-quick@wg0.service
[Service]
User=otelcol-contrib
Group=otelcol-contrib
SupplementaryGroups=systemd-journal
EnvironmentFile=/etc/otelcol-contrib/environment
ExecStart=/usr/local/bin/otelcol-contrib --config=/etc/otelcol-contrib/config.yaml
Restart=on-failure
NoNewPrivileges=true
[Install]
WantedBy=multi-user.target
EOF

ufw allow "${DEPLOYCRATE_WIREGUARD_PORT}/udp"
ufw allow "${DEPLOYCRATE_SSH_PORT}/tcp"
ufw allow in on wg0 to "${DEPLOYCRATE_PRIVATE_ADDRESS}" port 9100 proto tcp
ufw allow in on wg0 to "${DEPLOYCRATE_PRIVATE_ADDRESS}" port 9101 proto tcp
ufw allow in on wg0 to "${DEPLOYCRATE_PRIVATE_ADDRESS}" port 4318 proto tcp
ufw allow from 172.16.0.0/12 to "${DEPLOYCRATE_PRIVATE_ADDRESS}" port 4318 proto tcp
ufw --force enable
systemctl daemon-reload
systemctl enable docker.service node-exporter.service cadvisor.service otelcol-contrib.service
systemctl restart docker.service
systemctl start node-exporter.service cadvisor.service otelcol-contrib.service
usermod --append --groups docker deploycrate

if [ "${DEPLOYCRATE_CAPABILITY_RUNTIME}" = "true" ]; then
  install -d -o deploycrate -g deploycrate -m 0750 /var/lib/deploycrate-applications
fi
if [ "${DEPLOYCRATE_CAPABILITY_RESOURCE}" = "true" ]; then
  install -d -o deploycrate -g deploycrate -m 0750 /var/lib/deploycrate-resources
fi
if [ "${DEPLOYCRATE_CAPABILITY_DATABASE}" = "true" ]; then
  install -d -o deploycrate -g deploycrate -m 0750 /var/lib/deploycrate-databases
fi
if [ "${DEPLOYCRATE_CAPABILITY_REPOSITORY}" = "true" ]; then
  install -d -o deploycrate -g deploycrate -m 0750 /var/lib/deploycrate-repositories
fi

if [ "${DEPLOYCRATE_CAPABILITY_BUILD}" = "true" ]; then
  case "${architecture}" in
    amd64) pack_archive="pack-v0.40.6-linux.tgz"; pack_checksum="49fb874f7a930653834e67c16917369f9438080440194a6418421b1711421028" ;;
    arm64) pack_archive="pack-v0.40.6-linux-arm64.tgz"; pack_checksum="6ccff07f190a0ac5edec9cd3c1bc0a7192a9b5138147544adcdf2491efab0946" ;;
  esac
  curl -fsSL --retry 3 -o "${temporary_directory}/${pack_archive}" "https://github.com/buildpacks/pack/releases/download/v0.40.6/${pack_archive}"
  printf '%s  %s\n' "${pack_checksum}" "${temporary_directory}/${pack_archive}" | sha256sum --check --status
  tar -xzf "${temporary_directory}/${pack_archive}" -C "${temporary_directory}" pack
  install -m 0755 "${temporary_directory}/pack" /usr/local/bin/pack
  install -d -o deploycrate -g deploycrate -m 0750 /var/lib/deploycrate-builds
fi

for attempt in $(seq 1 30); do
  if curl -fsS "http://${DEPLOYCRATE_PRIVATE_ADDRESS}:9100/metrics" >/dev/null && \
    curl -fsS "http://${DEPLOYCRATE_PRIVATE_ADDRESS}:9101/healthz" >/dev/null && \
    ss -lnt | grep -Fq "${DEPLOYCRATE_PRIVATE_ADDRESS}:4318" && \
    curl -fsS http://127.0.0.1:13133/ >/dev/null; then
    break
  fi
  [ "${attempt}" -lt 30 ] || { printf 'Node telemetry services did not become healthy\n' >&2; exit 1; }
  sleep 1
done

ssh_host_public_key="$(cat /etc/ssh/ssh_host_ed25519_key.pub)"
rm -f /usr/local/bin/bootstrap
printf '{"wireguard_public_key":"%s","ssh_host_public_key":"%s","operating_system":"linux","distribution":"%s","distribution_version":"%s","architecture":"%s"}\n' \
  "${wireguard_public_key}" "${ssh_host_public_key}" "${ID}" "${VERSION_ID}" "${architecture}"
