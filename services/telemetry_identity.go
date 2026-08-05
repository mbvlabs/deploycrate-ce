package services

import (
	"deploycrate-ce/config"
	"deploycrate-ce/internal/telemetry"
	"strings"

	"github.com/google/uuid"
)

type TelemetryIdentity struct {
	identity telemetry.Identity
}

func NewTelemetryIdentity(cfg config.Config) (*TelemetryIdentity, error) {
	issuer := strings.TrimRight(config.BaseURL, "/") + "/telemetry"
	identity, err := telemetry.New(cfg.App.TokenSigningKey, issuer)
	if err != nil {
		return nil, err
	}
	return &TelemetryIdentity{identity: identity}, nil
}

func (service *TelemetryIdentity) EnvironmentToken(environmentID uuid.UUID) (string, error) {
	return service.identity.EnvironmentToken(environmentID)
}

func (service *TelemetryIdentity) NodeToken(serverID uuid.UUID) (string, error) {
	return service.identity.NodeToken(serverID)
}

func (service *TelemetryIdentity) Issuer() string {
	return service.identity.Issuer()
}

func (service *TelemetryIdentity) PublicJWKSet() (string, error) {
	return service.identity.PublicJWKSet()
}
