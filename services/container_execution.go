package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	containerclient "deploycrate-ce/clients/container"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

const resourceInstallationLabel = "com.deploycrate.resource-installation"

type ContainerExecution struct {
	local   containerclient.Client
	servers *ServerExecution
}

func NewContainerExecution(servers *ServerExecution) *ContainerExecution {
	return &ContainerExecution{local: containerclient.New(), servers: servers}
}

func (service *ContainerExecution) Run(ctx context.Context, serverID uuid.UUID, capability models.ServerCapability, spec containerclient.RunSpec) error {
	target, err := service.servers.Target(ctx, serverID, capability)
	if err != nil {
		return err
	}
	if !target.Remote {
		return service.local.Run(ctx, spec)
	}
	existing, err := service.inspectRemote(ctx, target, spec.InstallationID, spec.ContainerName)
	if err != nil {
		return err
	}
	if existing.Exists {
		if existing.Running {
			return nil
		}
		_, err := service.remoteDocker(ctx, target, "start", spec.ContainerName)
		return err
	}
	var script strings.Builder
	script.WriteString("set -euo pipefail\n")
	for _, mount := range spec.VolumeMounts {
		script.WriteString(shellJoin(remoteDockerExecutable, "volume", "create", mount.Name))
		script.WriteString(" >/dev/null\n")
	}
	keys := make([]string, 0, len(spec.Environment))
	for key := range spec.Environment {
		if !validRemoteEnvironmentKey(key) {
			return errors.New("container environment variable name is invalid")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		script.WriteString("dc_value=\"$(/usr/bin/printf '%s' ")
		script.WriteString(shellQuote(base64.StdEncoding.EncodeToString([]byte(spec.Environment[key]))))
		script.WriteString(" | /usr/bin/base64 --decode; /usr/bin/printf x)\"\nexport ")
		script.WriteString(key)
		script.WriteString("=\"${dc_value%x}\"\n")
	}
	arguments := []string{"run", "--detach", "--name", spec.ContainerName, "--label", resourceInstallationLabel + "=" + spec.InstallationID, "--restart", spec.RestartPolicy}
	for _, mapping := range spec.PortMappings {
		published := target.Peer.PrivateAddress + ":" + strconv.Itoa(int(mapping.HostPort)) + ":" + strconv.Itoa(int(mapping.ContainerPort)) + "/" + mapping.Protocol
		arguments = append(arguments, "--publish", published)
	}
	for _, mount := range spec.VolumeMounts {
		value := "type=volume,source=" + mount.Name + ",target=" + mount.MountPath
		if mount.ReadOnly {
			value += ",readonly"
		}
		arguments = append(arguments, "--mount", value)
	}
	for _, key := range keys {
		arguments = append(arguments, "--env", key)
	}
	arguments = append(arguments, spec.ImageReference)
	script.WriteString(shellJoin(remoteDockerExecutable, arguments...))
	script.WriteString(" >/dev/null\n")
	payload := []byte(script.String())
	defer clear(payload)
	if _, err = service.servers.RunRootScript(ctx, target, payload); err != nil {
		return err
	}
	if err := service.updateFirewall(ctx, target, spec.PortMappings, true); err != nil {
		_, _ = service.remoteDocker(context.WithoutCancel(ctx), target, "rm", "--force", spec.ContainerName)
		return err
	}
	return nil
}

func (service *ContainerExecution) Inspect(ctx context.Context, serverID uuid.UUID, capability models.ServerCapability, installationID, containerName string) (containerclient.State, error) {
	target, err := service.servers.Target(ctx, serverID, capability)
	if err != nil {
		return containerclient.State{}, err
	}
	if !target.Remote {
		return service.local.Inspect(ctx, installationID, containerName)
	}
	return service.inspectRemote(ctx, target, installationID, containerName)
}

func (service *ContainerExecution) Exec(ctx context.Context, serverID uuid.UUID, capability models.ServerCapability, spec containerclient.ExecSpec) error {
	target, err := service.servers.Target(ctx, serverID, capability)
	if err != nil {
		return err
	}
	if !target.Remote {
		return service.local.Exec(ctx, spec)
	}
	state, err := service.inspectRemote(ctx, target, spec.InstallationID, spec.ContainerName)
	if err != nil || !state.Exists || !state.Running {
		if err != nil {
			return err
		}
		return errors.New("container is not running")
	}
	keys := make([]string, 0, len(spec.Environment))
	for key := range spec.Environment {
		if !validRemoteEnvironmentKey(key) {
			return errors.New("container environment variable name is invalid")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var header strings.Builder
	var script strings.Builder
	script.WriteString("set -euo pipefail\n")
	for _, key := range keys {
		header.WriteString(base64.StdEncoding.EncodeToString([]byte(spec.Environment[key])))
		header.WriteByte('\n')
		script.WriteString("IFS= read -r dc_value\nexport ")
		script.WriteString(key)
		script.WriteString("=\"$(/usr/bin/printf '%s' \"$dc_value\" | /usr/bin/base64 --decode)\"\n")
	}
	arguments := []string{"exec", "--interactive"}
	for _, key := range keys {
		arguments = append(arguments, "--env", key)
	}
	arguments = append(arguments, spec.ContainerName, spec.Executable)
	arguments = append(arguments, spec.Arguments...)
	script.WriteString("exec ")
	script.WriteString(shellJoin(remoteDockerExecutable, arguments...))
	script.WriteByte('\n')
	input := spec.Stdin
	if input == nil {
		input = strings.NewReader("")
	}
	output := spec.Stdout
	if output == nil {
		output = io.Discard
	}
	return service.servers.RunRootCommandStreaming(ctx, target, io.MultiReader(strings.NewReader(header.String()), input), output, "/bin/bash", "-c", script.String())
}

func (service *ContainerExecution) Start(ctx context.Context, serverID uuid.UUID, capability models.ServerCapability, installationID, containerName string) error {
	return service.control(ctx, serverID, capability, installationID, containerName, "start")
}

func (service *ContainerExecution) Stop(ctx context.Context, serverID uuid.UUID, capability models.ServerCapability, installationID, containerName string) error {
	return service.control(ctx, serverID, capability, installationID, containerName, "stop")
}

func (service *ContainerExecution) Restart(ctx context.Context, serverID uuid.UUID, capability models.ServerCapability, installationID, containerName string) error {
	return service.control(ctx, serverID, capability, installationID, containerName, "restart")
}

func (service *ContainerExecution) Remove(ctx context.Context, serverID uuid.UUID, capability models.ServerCapability, installationID, containerName string) error {
	return service.control(ctx, serverID, capability, installationID, containerName, "rm", "--force")
}

func (service *ContainerExecution) control(ctx context.Context, serverID uuid.UUID, capability models.ServerCapability, installationID, containerName, operation string, extra ...string) error {
	target, err := service.servers.Target(ctx, serverID, capability)
	if err != nil {
		return err
	}
	if !target.Remote {
		switch operation {
		case "start":
			return service.local.Start(ctx, installationID, containerName)
		case "stop":
			return service.local.Stop(ctx, installationID, containerName)
		case "restart":
			return service.local.Restart(ctx, installationID, containerName)
		case "rm":
			return service.local.Remove(ctx, installationID, containerName)
		}
	}
	state, err := service.inspectRemote(ctx, target, installationID, containerName)
	if err != nil || !state.Exists {
		return err
	}
	arguments := append([]string{operation}, extra...)
	arguments = append(arguments, containerName)
	var mappings []containerclient.PortMapping
	if operation == "rm" {
		var mappingErr error
		mappings, mappingErr = service.remotePortMappings(ctx, target, containerName)
		if mappingErr != nil {
			return mappingErr
		}
	}
	if _, err = service.remoteDocker(ctx, target, arguments...); err != nil {
		return err
	}
	if operation == "rm" {
		return service.updateFirewall(ctx, target, mappings, false)
	}
	return nil
}

func (service *ContainerExecution) remotePortMappings(ctx context.Context, target ServerExecutionTarget, containerName string) ([]containerclient.PortMapping, error) {
	result, err := service.remoteDocker(ctx, target, "container", "inspect", "--format", "{{json .NetworkSettings.Ports}}", containerName)
	if err != nil {
		return nil, err
	}
	var ports map[string][]struct {
		HostIP   string `json:"HostIp"`
		HostPort string `json:"HostPort"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result)), &ports); err != nil {
		return nil, errors.New("Docker returned invalid published port state")
	}
	mappings := make([]containerclient.PortMapping, 0)
	for containerPort, bindings := range ports {
		portValue, protocol, found := strings.Cut(containerPort, "/")
		if !found {
			continue
		}
		parsedContainerPort, parseErr := strconv.Atoi(portValue)
		if parseErr != nil {
			continue
		}
		for _, binding := range bindings {
			if binding.HostIP != target.Peer.PrivateAddress {
				continue
			}
			hostPort, parseErr := strconv.Atoi(binding.HostPort)
			if parseErr == nil {
				mappings = append(mappings, containerclient.PortMapping{HostPort: int32(hostPort), ContainerPort: int32(parsedContainerPort), Protocol: protocol})
			}
		}
	}
	return mappings, nil
}

func (service *ContainerExecution) updateFirewall(ctx context.Context, target ServerExecutionTarget, mappings []containerclient.PortMapping, allow bool) error {
	for _, mapping := range mappings {
		arguments := []string{"allow", "in", "on", "wg0", "to", target.Peer.PrivateAddress, "port", strconv.Itoa(int(mapping.HostPort)), "proto", mapping.Protocol}
		if !allow {
			arguments = append([]string{"--force", "delete"}, arguments...)
		}
		if _, err := service.servers.RunRootCommand(ctx, target, nil, "/usr/sbin/ufw", arguments...); err != nil {
			if !allow && strings.Contains(strings.ToLower(err.Error()), "non-existent") {
				continue
			}
			return err
		}
	}
	return nil
}

func (service *ContainerExecution) inspectRemote(ctx context.Context, target ServerExecutionTarget, installationID, containerName string) (containerclient.State, error) {
	result, err := service.remoteDocker(ctx, target, "container", "inspect", containerName)
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "no such container") || strings.Contains(message, "no such object") {
			return containerclient.State{Exists: false, Name: containerName}, nil
		}
		return containerclient.State{}, err
	}
	var values []struct {
		ID           string `json:"Id"`
		Name         string `json:"Name"`
		Image        string `json:"Image"`
		RestartCount int    `json:"RestartCount"`
		Config       struct {
			Image  string            `json:"Image"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		State struct {
			Status, Error, StartedAt, FinishedAt string
			Running                              bool
			ExitCode                             int
			Health                               *struct{ Status string }
		} `json:"State"`
	}
	if err := json.Unmarshal([]byte(result), &values); err != nil || len(values) != 1 {
		return containerclient.State{}, errors.New("Docker returned an invalid container inspection")
	}
	value := values[0]
	if value.Config.Labels[resourceInstallationLabel] != installationID {
		return containerclient.State{}, fmt.Errorf("container %q is not owned by this installation", containerName)
	}
	health := ""
	if value.State.Health != nil {
		health = value.State.Health.Status
	}
	return containerclient.State{Exists: true, ID: value.ID, Name: strings.TrimPrefix(value.Name, "/"), ImageReference: value.Config.Image, ImageID: value.Image, Status: value.State.Status, Running: value.State.Running, Health: health, ExitCode: value.State.ExitCode, Error: value.State.Error, StartedAt: value.State.StartedAt, FinishedAt: value.State.FinishedAt, RestartCount: value.RestartCount}, nil
}

func (service *ContainerExecution) remoteDocker(ctx context.Context, target ServerExecutionTarget, arguments ...string) (string, error) {
	result, err := service.servers.RunRootCommand(ctx, target, nil, remoteDockerExecutable, arguments...)
	if err != nil {
		return "", fmt.Errorf("run Docker command on Server %s: %w", target.Server.Name, err)
	}
	return result.Stdout, nil
}
