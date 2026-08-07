package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"encoding/json"
	"errors"
	"fmt"

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
	Build      bool `json:"build"`
	Runtime    bool `json:"runtime"`
	Resource   bool `json:"resource"`
	Database   bool `json:"database"`
	Repository bool `json:"repository"`
	Telemetry  bool `json:"telemetry"`
}

func (capabilities ServerCapabilities) Validate() error {
	if !capabilities.Telemetry {
		return errors.New("managed nodes must collect telemetry")
	}
	if !capabilities.Build && !capabilities.Runtime && !capabilities.Resource &&
		!capabilities.Database &&
		!capabilities.Repository {
		return errors.New("select at least one node workload capability")
	}
	return nil
}

func (capabilities ServerCapabilities) JSON() (json.RawMessage, error) {
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
	return capabilities, capabilities.Validate()
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
