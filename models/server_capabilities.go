package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
)

type ServerCapability string

const (
	ServerCapabilityBuild      ServerCapability = "build"
	ServerCapabilityRuntime    ServerCapability = "runtime"
	ServerCapabilityResource   ServerCapability = "resource"
	ServerCapabilityDatabase   ServerCapability = "database"
	ServerCapabilityRepository ServerCapability = "repository"
)

type ServerCapabilities struct {
	Build      bool                       `json:"build"`
	Runtime    bool                       `json:"runtime"`
	Resource   bool                       `json:"resource"`
	Database   bool                       `json:"database"`
	Repository bool                       `json:"repository"`
	Telemetry  bool                       `json:"telemetry"`
	Buildpacks ServerBuildpacksCapability `json:"buildpacks"`
}

type ServerBuildpacksCapability struct {
	Tool     string             `json:"tool,omitempty"`
	Version  string             `json:"version,omitempty"`
	Runtimes []BuildpackRuntime `json:"runtimes,omitempty"`
}

func (capabilities ServerCapabilities) normalized() ServerCapabilities {
	if capabilities.Build && len(capabilities.Buildpacks.Runtimes) == 0 {
		capabilities.Buildpacks.Runtimes = []BuildpackRuntime{BuildpackRuntimeGo}
	}
	runtimes := make([]BuildpackRuntime, 0, len(capabilities.Buildpacks.Runtimes))
	seen := make(map[BuildpackRuntime]struct{}, len(capabilities.Buildpacks.Runtimes))
	for _, runtime := range capabilities.Buildpacks.Runtimes {
		runtime = BuildpackRuntime(strings.ToLower(strings.TrimSpace(string(runtime))))
		if _, exists := seen[runtime]; exists {
			continue
		}
		seen[runtime] = struct{}{}
		runtimes = append(runtimes, runtime)
	}
	slices.Sort(runtimes)
	capabilities.Buildpacks.Runtimes = runtimes
	return capabilities
}

func (capabilities ServerCapabilities) Validate() error {
	capabilities = capabilities.normalized()
	if !capabilities.Telemetry {
		return errors.New("managed nodes must collect telemetry")
	}
	if !capabilities.Build && !capabilities.Runtime && !capabilities.Resource &&
		!capabilities.Database &&
		!capabilities.Repository {
		return errors.New("select at least one node workload capability")
	}
	for _, runtime := range capabilities.Buildpacks.Runtimes {
		if !IsSupportedBuildpackRuntime(runtime) {
			return fmt.Errorf("unsupported Buildpack runtime %q", runtime)
		}
	}
	if !capabilities.Build && len(capabilities.Buildpacks.Runtimes) > 0 {
		return errors.New("Buildpack runtimes require the build capability")
	}
	return nil
}

func (capabilities ServerCapabilities) JSON() (json.RawMessage, error) {
	capabilities = capabilities.normalized()
	if err := capabilities.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(capabilities)
}

func ParseServerCapabilities(value json.RawMessage) (ServerCapabilities, error) {
	var capabilities ServerCapabilities
	if err := json.Unmarshal(value, &capabilities); err != nil {
		return ServerCapabilities{}, err
	}
	capabilities = capabilities.normalized()
	return capabilities, capabilities.Validate()
}

func (capabilities ServerCapabilities) SupportsBuildpack(runtime BuildpackRuntime) bool {
	capabilities = capabilities.normalized()
	return capabilities.Build && slices.Contains(capabilities.Buildpacks.Runtimes, runtime)
}

func (capabilities ServerCapabilities) Supports(capability ServerCapability) bool {
	switch capability {
	case ServerCapabilityBuild:
		return capabilities.Build
	case ServerCapabilityRuntime:
		return capabilities.Runtime
	case ServerCapabilityResource:
		return capabilities.Resource
	case ServerCapabilityDatabase:
		return capabilities.Database
	case ServerCapabilityRepository:
		return capabilities.Repository
	default:
		return false
	}
}

func RequireServerBuildpackCapability(
	ctx context.Context,
	db storage.Executor,
	serverID uuid.UUID,
	runtime BuildpackRuntime,
) (ServerEntity, error) {
	server, err := RequireServerCapability(ctx, db, serverID, ServerCapabilityBuild)
	if err != nil {
		return ServerEntity{}, err
	}
	capabilities, err := ParseServerCapabilities(server.Capabilities)
	if err != nil || !capabilities.SupportsBuildpack(runtime) {
		return ServerEntity{}, errors.Join(
			ErrDomainValidation,
			fmt.Errorf("Server does not support the %s Buildpack runtime", runtime),
		)
	}
	return server, nil
}

func RequireServerCapability(
	ctx context.Context,
	db storage.Executor,
	serverID uuid.UUID,
	capability ServerCapability,
) (ServerEntity, error) {
	server, err := Server.Find(ctx, db, serverID)
	if errors.Is(err, sql.ErrNoRows) ||
		err == nil && (server.ArchivedAt.Valid || !server.IsConfigured) {
		return ServerEntity{}, errors.Join(
			ErrDomainValidation,
			fmt.Errorf("Server is unavailable for %s placement", capability),
		)
	}
	if err != nil {
		return ServerEntity{}, err
	}
	if server.Kind != "self_hosted" && server.Kind != "worker" {
		return ServerEntity{}, errors.Join(
			ErrDomainValidation,
			fmt.Errorf("Server kind does not support %s placement", capability),
		)
	}
	capabilities, err := ParseServerCapabilities(server.Capabilities)
	if err != nil || !capabilities.Supports(capability) {
		return ServerEntity{}, errors.Join(
			ErrDomainValidation,
			fmt.Errorf("Server does not have the %s capability", capability),
		)
	}
	return server, nil
}

func ServerOriginAddress(
	ctx context.Context,
	db storage.Executor,
	serverID uuid.UUID,
) (string, error) {
	server, err := Server.Find(ctx, db, serverID)
	if err != nil {
		return "", err
	}
	if server.Kind == "self_hosted" {
		return "127.0.0.1", nil
	}
	if server.Kind != "worker" {
		return "", errors.New("Server kind has no managed service origin")
	}
	peer, err := WireGuardPeer.FindActiveForServer(ctx, db, serverID)
	if err != nil || peer.PrivateAddress == "" {
		return "", errors.New("worker Server has no active WireGuard service address")
	}
	return peer.PrivateAddress, nil
}
