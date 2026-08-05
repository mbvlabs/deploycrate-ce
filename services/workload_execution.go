package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"strconv"
	"strings"
	"time"

	containerclient "deploycrate-ce/clients/container"
	registryclient "deploycrate-ce/clients/registry"
	sshclient "deploycrate-ce/clients/ssh"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

const remoteDockerExecutable = "/usr/bin/docker"

type workloadExecutionTarget struct {
	server     models.ServerEntity
	credential models.ServerSSHCredentialEntity
	peer       models.WireGuardPeerEntity
	remote     bool
}

type WorkloadExecution struct {
	db       storage.Pool
	ssh      sshclient.Client
	sshCA    SSHCAService
	local    containerclient.WorkloadClient
	registry registryclient.Client
}

func NewWorkloadExecution(db storage.Pool, sshCA SSHCAService) *WorkloadExecution {
	return &WorkloadExecution{
		db: db, ssh: sshclient.New(), sshCA: sshCA,
		local: containerclient.NewWorkload(), registry: registryclient.New(),
	}
}

func (service *WorkloadExecution) ReconcileNetwork(ctx context.Context, serverID, environmentID uuid.UUID) (string, error) {
	target, err := service.target(ctx, serverID)
	if err != nil {
		return "", err
	}
	if !target.remote {
		return service.local.ReconcileNetwork(ctx, environmentID)
	}
	return service.reconcileRemoteNetwork(ctx, target, environmentID)
}

func (service *WorkloadExecution) ConnectResourceContainer(ctx context.Context, serverID, environmentID uuid.UUID, attachment containerclient.ResourceContainerAttachment) error {
	target, err := service.target(ctx, serverID)
	if err != nil {
		return err
	}
	if !target.remote {
		return service.local.ConnectResourceContainer(ctx, environmentID, attachment)
	}
	return service.connectRemoteResource(ctx, target, environmentID, attachment)
}

func (service *WorkloadExecution) PruneResourceContainers(ctx context.Context, serverID, environmentID uuid.UUID, keep []containerclient.ResourceContainerAttachment) error {
	target, err := service.target(ctx, serverID)
	if err != nil {
		return err
	}
	if !target.remote {
		return service.local.PruneResourceContainers(ctx, environmentID, keep)
	}
	return service.pruneRemoteResources(ctx, target, environmentID, keep)
}

func (service *WorkloadExecution) Find(ctx context.Context, serverID, deploymentID, instanceID uuid.UUID) (containerclient.WorkloadState, error) {
	target, err := service.target(ctx, serverID)
	if err != nil {
		return containerclient.WorkloadState{}, err
	}
	if !target.remote {
		state, findErr := service.local.Find(ctx, deploymentID, instanceID)
		return state, validateWorkloadTargetState(target, state, findErr)
	}
	state, findErr := service.findRemote(ctx, target, deploymentID, instanceID)
	return state, validateWorkloadTargetState(target, state, findErr)
}

func (service *WorkloadExecution) Run(ctx context.Context, serverID uuid.UUID, spec containerclient.WorkloadRunSpec, credentials registryclient.Credentials) (containerclient.WorkloadState, error) {
	target, err := service.target(ctx, serverID)
	if err != nil {
		return containerclient.WorkloadState{}, err
	}
	spec.Environment = workloadTelemetryEnvironment(spec, target)
	if !target.remote {
		authentication, err := service.registry.Authenticate(ctx, credentials)
		if err != nil {
			return containerclient.WorkloadState{}, err
		}
		defer authentication.Close()
		if err := service.registry.Pull(ctx, authentication, spec.ImageReference); err != nil {
			return containerclient.WorkloadState{}, err
		}
		spec.PublishAddress = "127.0.0.1"
		spec.DockerEnvironment = authentication.Environment()
		state, runErr := service.local.Run(ctx, spec)
		return state, validateWorkloadTargetState(target, state, runErr)
	}
	return service.runRemote(ctx, target, spec, credentials)
}

func (service *WorkloadExecution) Remove(ctx context.Context, serverID, deploymentID, instanceID uuid.UUID) error {
	target, err := service.target(ctx, serverID)
	if err != nil {
		return err
	}
	if !target.remote {
		return service.local.Remove(ctx, deploymentID, instanceID)
	}
	return service.removeRemote(ctx, target, deploymentID, instanceID)
}

func (service *WorkloadExecution) Start(ctx context.Context, serverID, deploymentID, instanceID uuid.UUID) (containerclient.WorkloadState, error) {
	target, err := service.target(ctx, serverID)
	if err != nil {
		return containerclient.WorkloadState{}, err
	}
	if !target.remote {
		state, startErr := service.local.Start(ctx, deploymentID, instanceID)
		return state, validateWorkloadTargetState(target, state, startErr)
	}
	state, err := service.findRemote(ctx, target, deploymentID, instanceID)
	if err != nil || !state.Exists {
		return state, err
	}
	if !state.Running {
		if _, err := service.remoteDocker(ctx, target, "start", state.ID); err != nil {
			return containerclient.WorkloadState{}, err
		}
		state, err = service.findRemote(ctx, target, deploymentID, instanceID)
		if err != nil {
			return containerclient.WorkloadState{}, err
		}
	}
	if err := validateWorkloadTargetState(target, state, nil); err != nil {
		return containerclient.WorkloadState{}, err
	}
	if err := service.updateRemoteFirewall(ctx, target, state, true); err != nil {
		return containerclient.WorkloadState{}, err
	}
	return state, validateWorkloadTargetState(target, state, nil)
}

func (service *WorkloadExecution) DeleteEnvironment(ctx context.Context, serverID, environmentID uuid.UUID) error {
	target, err := service.target(ctx, serverID)
	if err != nil {
		return err
	}
	if !target.remote {
		return service.local.DeleteEnvironment(ctx, environmentID)
	}
	states, err := service.findRemoteEnvironment(ctx, target, environmentID)
	if err != nil {
		return err
	}
	identifiers := make([]string, 0, len(states))
	for _, state := range states {
		if err := validateWorkloadTargetState(target, state, nil); err != nil {
			return err
		}
		if err := service.updateRemoteFirewall(ctx, target, state, false); err != nil {
			return err
		}
		identifiers = append(identifiers, state.ID)
	}
	if len(identifiers) > 0 {
		if _, err := service.remoteDocker(ctx, target, append([]string{"rm", "--force"}, identifiers...)...); err != nil {
			return err
		}
	}
	if err := service.pruneRemoteResources(ctx, target, environmentID, nil); err != nil {
		return err
	}
	_, err = service.remoteDocker(ctx, target, "network", "rm", remoteNetworkName(environmentID))
	if err != nil && !remoteDockerNotFound(err) {
		return err
	}
	return nil
}

func (service *WorkloadExecution) target(ctx context.Context, serverID uuid.UUID) (workloadExecutionTarget, error) {
	server, err := models.Server.Find(ctx, service.db.Executor(), serverID)
	if err != nil || server.ArchivedAt.Valid || !server.IsConfigured {
		return workloadExecutionTarget{}, errors.New("workload target Server is unavailable")
	}
	capabilities, err := models.ParseServerCapabilities(server.Capabilities)
	if err != nil || !capabilities.Runtime {
		return workloadExecutionTarget{}, errors.New("workload target Server does not support application runtime execution")
	}
	target := workloadExecutionTarget{server: server, remote: server.Kind == "worker"}
	if !target.remote {
		if server.Kind != "self_hosted" {
			return workloadExecutionTarget{}, errors.New("workload target Server kind is unsupported")
		}
	}
	target.peer, err = models.WireGuardPeer.FindActiveForServer(ctx, service.db.Executor(), server.ID)
	if err != nil {
		return workloadExecutionTarget{}, errors.New("workload target Server has no active WireGuard peer")
	}
	if !target.remote {
		return target, nil
	}
	target.credential, err = models.ServerSSHCredential.FindForServer(ctx, service.db.Executor(), server.ID)
	if err != nil || !target.credential.HostKeyConfirmedAt.Valid || strings.TrimSpace(target.credential.KnownHostKey) == "" {
		return workloadExecutionTarget{}, errors.New("workload target Server has no trusted SSH identity")
	}
	return target, nil
}

func workloadTelemetryEnvironment(spec containerclient.WorkloadRunSpec, target workloadExecutionTarget) map[string]string {
	environment := make(map[string]string, len(spec.Environment)+1)
	maps.Copy(environment, spec.Environment)
	resourceAttributes := []string{
		"deploycrate.application.id=" + spec.ApplicationID.String(),
		"deploycrate.environment.id=" + spec.EnvironmentID.String(),
		"deploycrate.target.id=" + spec.TargetID.String(),
		"deploycrate.deployment.id=" + spec.DeploymentID.String(),
		"deploycrate.instance.id=" + spec.InstanceID.String(),
		"deploycrate.release.id=" + spec.ReleaseID.String(),
		"deploycrate.server.id=" + target.server.ID.String(),
	}
	if configured := strings.TrimSpace(environment["OTEL_RESOURCE_ATTRIBUTES"]); configured != "" {
		resourceAttributes = append([]string{configured}, resourceAttributes...)
	}
	environment["OTEL_RESOURCE_ATTRIBUTES"] = strings.Join(resourceAttributes, ",")
	return environment
}

func (service *WorkloadExecution) reconcileRemoteNetwork(ctx context.Context, target workloadExecutionTarget, environmentID uuid.UUID) (string, error) {
	name := remoteNetworkName(environmentID)
	output, err := service.remoteDocker(ctx, target, "network", "inspect", name)
	if err != nil {
		if _, createErr := service.remoteDocker(ctx, target, "network", "create", "--driver", "bridge", "--label", containerclient.WorkloadLabelNetworkOwner+"="+environmentID.String(), name); createErr != nil {
			return "", createErr
		}
		return name, nil
	}
	var networks []struct {
		Driver string            `json:"Driver"`
		Labels map[string]string `json:"Labels"`
	}
	if json.Unmarshal([]byte(output), &networks) != nil || len(networks) != 1 || networks[0].Driver != "bridge" || networks[0].Labels[containerclient.WorkloadLabelNetworkOwner] != environmentID.String() {
		return "", errors.New("existing Environment network has invalid ownership")
	}
	return name, nil
}

func (service *WorkloadExecution) connectRemoteResource(ctx context.Context, target workloadExecutionTarget, environmentID uuid.UUID, attachment containerclient.ResourceContainerAttachment) error {
	if environmentID == uuid.Nil || attachment.InstallationID == uuid.Nil || !validRemoteContainerName(attachment.ContainerName) {
		return errors.New("Resource container attachment is invalid")
	}
	output, err := service.remoteDocker(ctx, target, "container", "inspect", attachment.ContainerName)
	if err != nil {
		return err
	}
	var containers []struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
		NetworkSettings struct {
			Networks map[string]json.RawMessage `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	if json.Unmarshal([]byte(output), &containers) != nil || len(containers) != 1 {
		return errors.New("Docker returned invalid Resource container state")
	}
	container := containers[0]
	if container.Config.Labels[containerclient.WorkloadLabelResource] != attachment.InstallationID.String() {
		return errors.New("Resource container has invalid installation ownership")
	}
	if !container.State.Running {
		return errors.New("Resource container is not running")
	}
	networkName := remoteNetworkName(environmentID)
	if _, connected := container.NetworkSettings.Networks[networkName]; connected {
		return nil
	}
	_, err = service.remoteDocker(ctx, target, "network", "connect", networkName, attachment.ContainerName)
	return err
}

func (service *WorkloadExecution) pruneRemoteResources(ctx context.Context, target workloadExecutionTarget, environmentID uuid.UUID, keep []containerclient.ResourceContainerAttachment) error {
	networkName := remoteNetworkName(environmentID)
	output, err := service.remoteDocker(ctx, target, "network", "inspect", networkName)
	if err != nil {
		if remoteDockerNotFound(err) {
			return nil
		}
		return err
	}
	var networks []struct {
		Containers map[string]struct {
			Name string `json:"Name"`
		} `json:"Containers"`
	}
	if json.Unmarshal([]byte(output), &networks) != nil || len(networks) != 1 {
		return errors.New("Docker returned invalid Environment network state")
	}
	retained := make(map[string]struct{}, len(keep))
	for _, attachment := range keep {
		retained[attachment.InstallationID.String()+"\x00"+attachment.ContainerName] = struct{}{}
	}
	for identifier, connected := range networks[0].Containers {
		containerOutput, inspectErr := service.remoteDocker(ctx, target, "container", "inspect", connected.Name)
		if inspectErr != nil {
			return inspectErr
		}
		var containers []struct {
			Config struct {
				Labels map[string]string `json:"Labels"`
			} `json:"Config"`
		}
		if json.Unmarshal([]byte(containerOutput), &containers) != nil || len(containers) != 1 {
			return errors.New("Docker returned invalid connected container state")
		}
		installationID, parseErr := uuid.Parse(containers[0].Config.Labels[containerclient.WorkloadLabelResource])
		if parseErr != nil {
			continue
		}
		if _, exists := retained[installationID.String()+"\x00"+connected.Name]; exists {
			continue
		}
		if _, err := service.remoteDocker(ctx, target, "network", "disconnect", "--force", networkName, identifier); err != nil {
			return err
		}
	}
	return nil
}

func (service *WorkloadExecution) findRemote(ctx context.Context, target workloadExecutionTarget, deploymentID, instanceID uuid.UUID) (containerclient.WorkloadState, error) {
	output, err := service.remoteDocker(ctx, target, "ps", "--all", "--quiet",
		"--filter", "label="+containerclient.WorkloadLabelDeployment+"="+deploymentID.String(),
		"--filter", "label="+containerclient.WorkloadLabelInstance+"="+instanceID.String())
	if err != nil {
		return containerclient.WorkloadState{}, err
	}
	identifiers := strings.Fields(output)
	if len(identifiers) == 0 {
		return containerclient.WorkloadState{}, nil
	}
	if len(identifiers) != 1 {
		return containerclient.WorkloadState{}, errors.New("multiple Docker containers claim the same Deployment and Instance")
	}
	return service.inspectRemoteWorkload(ctx, target, identifiers[0])
}

func (service *WorkloadExecution) findRemoteEnvironment(ctx context.Context, target workloadExecutionTarget, environmentID uuid.UUID) ([]containerclient.WorkloadState, error) {
	output, err := service.remoteDocker(ctx, target, "ps", "--all", "--quiet", "--filter", "label="+containerclient.WorkloadLabelEnvironment+"="+environmentID.String())
	if err != nil {
		return nil, err
	}
	identifiers := strings.Fields(output)
	states := make([]containerclient.WorkloadState, 0, len(identifiers))
	for _, identifier := range identifiers {
		state, inspectErr := service.inspectRemoteWorkload(ctx, target, identifier)
		if inspectErr != nil {
			return nil, inspectErr
		}
		states = append(states, state)
	}
	return states, nil
}

func (service *WorkloadExecution) inspectRemoteWorkload(ctx context.Context, target workloadExecutionTarget, identifier string) (containerclient.WorkloadState, error) {
	output, err := service.remoteDocker(ctx, target, "inspect", identifier)
	if err != nil {
		return containerclient.WorkloadState{}, err
	}
	return containerclient.ParseWorkloadInspect([]byte(output))
}

func (service *WorkloadExecution) runRemote(ctx context.Context, target workloadExecutionTarget, spec containerclient.WorkloadRunSpec, credentials registryclient.Credentials) (containerclient.WorkloadState, error) {
	if strings.TrimSpace(credentials.Endpoint) == "" || strings.ContainsAny(credentials.Endpoint, " \t\r\n") || strings.TrimSpace(credentials.Username) == "" || credentials.Password == "" {
		return containerclient.WorkloadState{}, errors.New("registry endpoint and credentials are required")
	}
	existing, err := service.findRemote(ctx, target, spec.DeploymentID, spec.InstanceID)
	if err != nil || existing.Exists {
		return existing, validateWorkloadTargetState(target, existing, err)
	}
	spec.PublishAddress = target.peer.PrivateAddress
	spec.DockerEnvironment = nil
	prepared, err := containerclient.PrepareWorkloadRun(spec)
	if err != nil {
		return containerclient.WorkloadState{}, err
	}
	var script strings.Builder
	script.WriteString("set -euo pipefail\n")
	script.WriteString("dc_auth=\"$(/usr/bin/mktemp -d -p /var/lib/deploycrate-node registry-auth-XXXXXXXXXX)\"\n")
	script.WriteString("trap '/usr/bin/rm -rf -- \"$dc_auth\"' EXIT\n")
	script.WriteString("/usr/bin/chmod 0700 \"$dc_auth\"\n")
	script.WriteString("export HOME=\"$dc_auth\" DOCKER_CONFIG=\"$dc_auth\"\n")
	script.WriteString("/usr/bin/printf '%s' ")
	script.WriteString(shellQuote(base64.StdEncoding.EncodeToString([]byte(credentials.Password))))
	script.WriteString(" | /usr/bin/base64 --decode | ")
	script.WriteString(shellJoin(remoteDockerExecutable, "login", credentials.Endpoint, "--username", credentials.Username, "--password-stdin"))
	script.WriteString(" >/dev/null\n")
	script.WriteString(shellJoin(remoteDockerExecutable, "pull", spec.ImageReference))
	script.WriteString(" >/dev/null\n")
	for _, pair := range prepared.Environment {
		key, value, found := strings.Cut(pair, "=")
		if !found || !validRemoteEnvironmentKey(key) {
			return containerclient.WorkloadState{}, errors.New("workload environment cannot be represented by the remote shell")
		}
		script.WriteString("dc_value=\"$(/usr/bin/printf '%s' ")
		script.WriteString(shellQuote(base64.StdEncoding.EncodeToString([]byte(value))))
		script.WriteString(" | /usr/bin/base64 --decode; /usr/bin/printf x)\"\n")
		script.WriteString("export ")
		script.WriteString(key)
		script.WriteString("=\"${dc_value%x}\"\n")
	}
	script.WriteString(shellJoin(remoteDockerExecutable, prepared.Arguments...))
	script.WriteString(" >/dev/null\n")
	scriptBytes := []byte(script.String())
	defer clear(scriptBytes)
	if _, err := service.remoteRootScript(ctx, target, scriptBytes); err != nil {
		return containerclient.WorkloadState{}, err
	}
	state, err := service.findRemote(ctx, target, spec.DeploymentID, spec.InstanceID)
	if err != nil {
		_ = service.removeRemote(context.WithoutCancel(ctx), target, spec.DeploymentID, spec.InstanceID)
		return containerclient.WorkloadState{}, err
	}
	if err := validateWorkloadTargetState(target, state, nil); err != nil {
		if state.ID != "" {
			_, _ = service.remoteDocker(context.WithoutCancel(ctx), target, "rm", "--force", state.ID)
		}
		return containerclient.WorkloadState{}, err
	}
	if err := service.updateRemoteFirewall(ctx, target, state, true); err != nil {
		_ = service.removeRemote(context.WithoutCancel(ctx), target, spec.DeploymentID, spec.InstanceID)
		return containerclient.WorkloadState{}, err
	}
	return state, nil
}

func (service *WorkloadExecution) removeRemote(ctx context.Context, target workloadExecutionTarget, deploymentID, instanceID uuid.UUID) error {
	state, err := service.findRemote(ctx, target, deploymentID, instanceID)
	if err != nil || !state.Exists {
		return err
	}
	if err := validateWorkloadTargetState(target, state, nil); err != nil {
		return err
	}
	if err := service.updateRemoteFirewall(ctx, target, state, false); err != nil {
		return err
	}
	_, err = service.remoteDocker(ctx, target, "rm", "--force", state.ID)
	return err
}

func (service *WorkloadExecution) updateRemoteFirewall(ctx context.Context, target workloadExecutionTarget, state containerclient.WorkloadState, allow bool) error {
	if state.HostAddress == "" || state.HostPort == 0 {
		return nil
	}
	if state.HostAddress != target.peer.PrivateAddress {
		return errors.New("workload firewall address does not belong to its target Server")
	}
	arguments := []string{"allow", "in", "on", "wg0", "from", WireGuardNodeCIDR, "to", state.HostAddress, "port", fmt.Sprint(state.HostPort), "proto", "tcp"}
	if !allow {
		arguments = append([]string{"--force", "delete"}, arguments...)
	}
	_, err := service.remoteRootCommand(ctx, target, "/usr/sbin/ufw", arguments...)
	if err != nil && !allow && strings.Contains(strings.ToLower(err.Error()), "non-existent") {
		return nil
	}
	return err
}

func (service *WorkloadExecution) remoteDocker(ctx context.Context, target workloadExecutionTarget, arguments ...string) (string, error) {
	result, err := service.remoteRootCommand(ctx, target, remoteDockerExecutable, arguments...)
	if err != nil {
		return "", fmt.Errorf("run Docker command on node: %w", err)
	}
	return result.Stdout, nil
}

func (service *WorkloadExecution) remoteRootCommand(ctx context.Context, target workloadExecutionTarget, executable string, arguments ...string) (sshclient.Result, error) {
	script := []byte("set -euo pipefail\nexec " + shellJoin(executable, arguments...) + "\n")
	return service.remoteRootScript(ctx, target, script)
}

func (service *WorkloadExecution) remoteRootScript(ctx context.Context, target workloadExecutionTarget, script []byte) (sshclient.Result, error) {
	certificate, err := service.sshCA.GenerateUserCertificate(5 * time.Minute)
	if err != nil {
		return sshclient.Result{}, err
	}
	address := net.JoinHostPort(target.peer.PrivateAddress, strconv.Itoa(int(target.credential.Port)))
	return service.ssh.RunWithCertificate(
		ctx, address, "admin", target.credential.KnownHostKey,
		certificate.PrivateKey, certificate.Certificate,
		"sudo -n /bin/bash -s", bytes.NewReader(script),
	)
}

func validateWorkloadTargetState(target workloadExecutionTarget, state containerclient.WorkloadState, operationErr error) error {
	if operationErr != nil || !state.Exists {
		return operationErr
	}
	if state.HostAddress == "" || state.HostPort == 0 {
		return errors.New("workload target address is unavailable")
	}
	expectedAddress := "127.0.0.1"
	if target.remote {
		expectedAddress = target.peer.PrivateAddress
	}
	if state.HostAddress != expectedAddress {
		return errors.New("workload address does not belong to its target Server")
	}
	return nil
}

func remoteNetworkName(environmentID uuid.UUID) string {
	return "dc-env-" + environmentID.String()
}

func remoteDockerNotFound(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") || strings.Contains(message, "no such network")
}

func shellJoin(executable string, arguments ...string) string {
	parts := make([]string, 0, len(arguments)+1)
	parts = append(parts, shellQuote(executable))
	for _, argument := range arguments {
		parts = append(parts, shellQuote(argument))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func validRemoteContainerName(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.HasPrefix(value, "-") && !strings.ContainsAny(value, " \t\r\n\x00")
}

func validRemoteEnvironmentKey(value string) bool {
	if value == "" || !((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z') || value[0] == '_') {
		return false
	}
	for _, character := range value[1:] {
		if !((character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_') {
			return false
		}
	}
	return true
}
