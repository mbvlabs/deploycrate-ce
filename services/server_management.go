package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"deploycrate-ce/internal/hostcommand"
	"deploycrate-ce/internal/resourceaccess"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

const (
	managementContainerLogTail = 200
	dockerListFormat           = "{{json .}}"
)

type ServerContainer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	State  string `json:"state"`
	Status string `json:"status"`
	Ports  string `json:"ports"`
}

type ServerImage struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Size       string `json:"size"`
}

type ServerUpdate struct {
	Name      string `json:"name"`
	Installed string `json:"installed"`
	Available string `json:"available"`
}

type ServerUpdateState struct {
	RebootRequired bool           `json:"rebootRequired"`
	Total          int            `json:"total"`
	Updates        []ServerUpdate `json:"updates"`
}

type dockerContainerListing struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Image  string `json:"Image"`
	State  string `json:"State"`
	Status string `json:"Status"`
	Ports  string `json:"Ports"`
}

type dockerImageListing struct {
	ID         string `json:"ID"`
	Repository string `json:"Repository"`
	Tag        string `json:"Tag"`
	Size       string `json:"Size"`
}

type ServerManagement struct {
	db      storage.Pool
	servers *ServerExecution
}

func NewServerManagement(db storage.Pool, servers *ServerExecution) *ServerManagement {
	return &ServerManagement{db: db, servers: servers}
}

func (service *ServerManagement) All(ctx context.Context) ([]models.ServerEntity, error) {
	return models.Server.All(ctx, service.db.Executor())
}

func (service *ServerManagement) Find(
	ctx context.Context,
	serverID uuid.UUID,
) (models.ServerEntity, error) {
	return models.Server.Find(ctx, service.db.Executor(), serverID)
}

func (service *ServerManagement) ListContainers(
	ctx context.Context,
	serverID uuid.UUID,
) ([]ServerContainer, error) {
	raw, err := service.dockerList(ctx, serverID)
	if err != nil {
		return nil, err
	}
	containers := make([]ServerContainer, 0)
	for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var value dockerContainerListing
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			return nil, errors.New("Server returned an invalid container listing")
		}
		containers = append(containers, ServerContainer{
			ID:     value.ID,
			Name:   value.Names,
			Image:  value.Image,
			State:  value.State,
			Status: value.Status,
			Ports:  value.Ports,
		})
	}
	return containers, nil
}

func (service *ServerManagement) ListImages(
	ctx context.Context,
	serverID uuid.UUID,
) ([]ServerImage, error) {
	raw, err := service.dockerListImages(ctx, serverID)
	if err != nil {
		return nil, err
	}
	images := make([]ServerImage, 0)
	for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var value dockerImageListing
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			return nil, errors.New("Server returned an invalid image listing")
		}
		images = append(images, ServerImage{
			ID:         value.ID,
			Repository: value.Repository,
			Tag:        value.Tag,
			Size:       value.Size,
		})
	}
	return images, nil
}

func (service *ServerManagement) ContainerControl(
	ctx context.Context,
	serverID uuid.UUID,
	operation, containerName string,
) error {
	target, err := service.servers.TargetAny(ctx, serverID)
	if err != nil {
		return err
	}
	if target.Remote {
		arguments := []string{operation, containerName}
		if operation == "remove" {
			arguments = []string{"rm", "--force", containerName}
		}
		_, err := service.servers.RunRootCommand(
			ctx,
			target,
			nil,
			remoteDockerExecutable,
			arguments...)
		return err
	}
	_, err = hostcommand.Run(ctx, "container-operate", operation, containerName)
	return err
}

func (service *ServerManagement) ContainerLogs(
	ctx context.Context,
	serverID uuid.UUID,
	containerName string,
	tail int,
) (string, error) {
	target, err := service.servers.TargetAny(ctx, serverID)
	if err != nil {
		return "", err
	}
	if target.Remote {
		result, err := service.servers.RunRootCommand(
			ctx,
			target,
			nil,
			remoteDockerExecutable,
			"logs",
			"--tail",
			strconv.Itoa(tail),
			containerName,
		)
		if err != nil {
			return "", err
		}
		return result.Stdout, nil
	}
	return hostcommand.Run(
		ctx,
		"container-logs-all",
		containerName,
		strconv.Itoa(tail),
	)
}

func (service *ServerManagement) ImageRemove(
	ctx context.Context,
	serverID uuid.UUID,
	reference string,
) error {
	target, err := service.servers.TargetAny(ctx, serverID)
	if err != nil {
		return err
	}
	if target.Remote {
		_, err := service.servers.RunRootCommand(
			ctx,
			target,
			nil,
			remoteDockerExecutable,
			"rmi",
			reference,
		)
		return err
	}
	_, err = hostcommand.Run(ctx, "image-remove", reference)
	return err
}

func (service *ServerManagement) Prune(
	ctx context.Context,
	serverID uuid.UUID,
	scopes []string,
) error {
	if len(scopes) == 0 {
		return errors.New("at least one prune scope is required")
	}
	resources := make([]string, len(scopes))
	for index, scope := range scopes {
		var ok bool
		resources[index], ok = dockerPruneResource(scope)
		if !ok {
			return fmt.Errorf("unsupported prune scope %q", scope)
		}
	}
	target, err := service.servers.TargetAny(ctx, serverID)
	if err != nil {
		return err
	}
	for _, resource := range resources {
		if target.Remote {
			arguments := []string{resource, "prune", "--force"}
			if resource == "image" {
				arguments = append(arguments, "--all")
			}
			if _, err := service.servers.RunRootCommand(
				ctx,
				target,
				nil,
				remoteDockerExecutable,
				arguments...,
			); err != nil {
				return err
			}
			continue
		}
		if _, err := hostcommand.Run(ctx, resource+"-prune"); err != nil {
			return err
		}
	}
	return nil
}

func dockerPruneResource(scope string) (string, bool) {
	switch scope {
	case "containers":
		return "container", true
	case "images":
		return "image", true
	case "volumes":
		return "volume", true
	default:
		return "", false
	}
}

func (service *ServerManagement) CheckUpdates(
	ctx context.Context,
	serverID uuid.UUID,
) (ServerUpdateState, error) {
	target, err := service.servers.TargetAny(ctx, serverID)
	if err != nil {
		return ServerUpdateState{}, err
	}
	raw, err := service.runOSUpdateScript(
		ctx,
		target,
		"host-update-check",
		resourceaccess.OSUpdateCheckScript(),
	)
	if err != nil {
		return ServerUpdateState{}, err
	}
	return parseUpdateState(raw)
}

func (service *ServerManagement) ApplyUpdates(
	ctx context.Context,
	serverID uuid.UUID,
) (ServerUpdateState, error) {
	target, err := service.servers.TargetAny(ctx, serverID)
	if err != nil {
		return ServerUpdateState{}, err
	}
	raw, err := service.runOSUpdateScript(
		ctx,
		target,
		"host-update-apply",
		resourceaccess.OSUpdateApplyScript(),
	)
	if err != nil {
		return ServerUpdateState{}, err
	}
	return parseUpdateState(raw)
}

func (service *ServerManagement) Reboot(ctx context.Context, serverID uuid.UUID) error {
	target, err := service.servers.TargetAny(ctx, serverID)
	if err != nil {
		return err
	}
	if target.Remote {
		_, err := service.servers.RunRootCommand(
			ctx,
			target,
			nil,
			"/usr/bin/systemctl",
			"reboot",
			"--no-block",
		)
		return err
	}
	_, err = hostcommand.Run(ctx, "host-reboot")
	return err
}

func (service *ServerManagement) ProvisionCapabilities(
	ctx context.Context,
	serverID uuid.UUID,
	capabilities models.ServerCapabilities,
) error {
	if err := capabilities.Validate(); err != nil {
		return errors.Join(models.ErrDomainValidation, err)
	}
	target, err := service.servers.TargetAny(ctx, serverID)
	if err != nil {
		return err
	}
	current, err := models.ParseServerCapabilities(target.Server.Capabilities)
	if err != nil {
		return fmt.Errorf("parse current Server capabilities: %w", err)
	}
	removed := make([]string, 0, 5)
	for _, capability := range []struct {
		key     string
		current bool
		next    bool
	}{
		{"build", current.Build, capabilities.Build},
		{"runtime", current.Runtime, capabilities.Runtime},
		{"resource", current.Resource, capabilities.Resource},
		{"database", current.Database, capabilities.Database},
		{"repository", current.Repository, capabilities.Repository},
	} {
		if capability.current && !capability.next {
			removed = append(removed, capability.key)
		}
	}
	if len(removed) > 0 {
		return errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf(
				"provisioned Server capabilities cannot be removed: %s",
				strings.Join(removed, ", "),
			),
		)
	}
	for _, runtime := range current.Buildpacks.Runtimes {
		if !capabilities.SupportsBuildpack(runtime) {
			return errors.Join(
				models.ErrDomainValidation,
				fmt.Errorf("provisioned Buildpack runtime cannot be removed: %s", runtime),
			)
		}
	}
	capabilities.Buildpacks.Tool = current.Buildpacks.Tool
	capabilities.Buildpacks.Version = current.Buildpacks.Version
	requested := make([]string, 0, 5)
	for _, capability := range []struct {
		key   string
		value bool
	}{
		{"build", capabilities.Build},
		{"runtime", capabilities.Runtime},
		{"resource", capabilities.Resource},
		{"database", capabilities.Database},
		{"repository", capabilities.Repository},
	} {
		if capability.value {
			requested = append(requested, capability.key)
		}
	}
	if target.Remote {
		script := append(
			[]byte("export DEPLOYCRATE_CAPABILITIES="),
			[]byte(shellQuote(strings.Join(requested, ","))+"\n")...,
		)
		script = append(script, resourceaccess.CapabilityProvisionScript()...)
		defer clear(script)
		if _, err := service.servers.RunRootScript(ctx, target, script); err != nil {
			return err
		}
	} else {
		arguments := append([]string{"capability-apply"}, requested...)
		if _, err := hostcommand.RunWithInput(
			ctx,
			resourceaccess.CapabilityProvisionScript(),
			arguments...,
		); err != nil {
			return err
		}
	}
	encoded, err := capabilities.JSON()
	if err != nil {
		return err
	}
	var currentDocument, nextDocument map[string]json.RawMessage
	if err := json.Unmarshal(target.Server.Capabilities, &currentDocument); err != nil {
		return fmt.Errorf("decode current Server capability metadata: %w", err)
	}
	if err := json.Unmarshal(encoded, &nextDocument); err != nil {
		return fmt.Errorf("decode requested Server capabilities: %w", err)
	}
	for key, value := range currentDocument {
		if _, managed := nextDocument[key]; !managed {
			nextDocument[key] = value
		}
	}
	encoded, err = json.Marshal(nextDocument)
	if err != nil {
		return fmt.Errorf("encode merged Server capabilities: %w", err)
	}
	return models.Server.UpdateCapabilities(ctx, service.db.Executor(), serverID, encoded)
}

func (service *ServerManagement) dockerList(
	ctx context.Context,
	serverID uuid.UUID,
) (string, error) {
	target, err := service.servers.TargetAny(ctx, serverID)
	if err != nil {
		return "", err
	}
	if target.Remote {
		result, err := service.servers.RunRootCommand(
			ctx,
			target,
			nil,
			remoteDockerExecutable,
			"ps",
			"-a",
			"--format",
			dockerListFormat,
		)
		if err != nil {
			return "", err
		}
		return result.Stdout, nil
	}
	return hostcommand.Run(ctx, "container-list")
}

func (service *ServerManagement) dockerListImages(
	ctx context.Context,
	serverID uuid.UUID,
) (string, error) {
	target, err := service.servers.TargetAny(ctx, serverID)
	if err != nil {
		return "", err
	}
	if target.Remote {
		result, err := service.servers.RunRootCommand(
			ctx,
			target,
			nil,
			remoteDockerExecutable,
			"images",
			"--format",
			dockerListFormat,
		)
		if err != nil {
			return "", err
		}
		return result.Stdout, nil
	}
	return hostcommand.Run(ctx, "image-list")
}

func (service *ServerManagement) runOSUpdateScript(
	ctx context.Context,
	target ServerExecutionTarget,
	localSubcommand string,
	script []byte,
) (string, error) {
	if target.Remote {
		result, err := service.servers.RunRootScript(ctx, target, script)
		if err != nil {
			return "", err
		}
		return result.Stdout, nil
	}
	return hostcommand.Run(ctx, localSubcommand)
}

func parseUpdateState(raw string) (ServerUpdateState, error) {
	var state ServerUpdateState
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &state); err != nil {
		return ServerUpdateState{}, fmt.Errorf("Server returned an invalid update state: %w", err)
	}
	return state, nil
}
