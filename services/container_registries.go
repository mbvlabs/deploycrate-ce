package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	registryclient "deploycrate-ce/clients/registry"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

const registryCredentialPurpose = "registry-credential/v1"

type ContainerRegistries struct {
	db       storage.Pool
	config   config.Config
	registry registryclient.Client
}

func NewContainerRegistries(db storage.Pool, cfg config.Config) *ContainerRegistries {
	return &ContainerRegistries{db: db, config: cfg, registry: registryclient.New()}
}

type ExternalContainerRegistryInput struct {
	Name        string
	Endpoint    string
	Username    string
	AccessToken string
}

type ContainerRegistrySummary struct {
	ID             uuid.UUID `json:"id" bun:"id"`
	Name           string    `json:"name" bun:"name"`
	Provider       string    `json:"provider" bun:"provider"`
	Endpoint       string    `json:"endpoint" bun:"endpoint"`
	Username       string    `json:"username" bun:"username"`
	CredentialName string    `json:"credentialName" bun:"credential_name"`
	Managed        bool      `json:"managed" bun:"managed"`
	CreatedAt      time.Time `json:"createdAt" bun:"created_at"`
}

func (service *ContainerRegistries) List(ctx context.Context) ([]ContainerRegistrySummary, error) {
	registries := make([]ContainerRegistrySummary, 0)
	err := service.db.Executor().NewSelect().TableExpr("container_registries AS registry").
		ColumnExpr("registry.id, registry.name, registry.provider, registry.endpoint, registry.created_at").
		ColumnExpr("credential.name AS credential_name").
		ColumnExpr("COALESCE(credential.metadata ->> 'username', '') AS username").
		ColumnExpr("COALESCE(credential.metadata ->> 'credential_kind', '') <> 'external_registry' AS managed").
		Join("JOIN credentials AS credential ON credential.id = registry.credential_id AND credential.archived_at IS NULL").
		Where("registry.archived_at IS NULL").
		OrderExpr("registry.name ASC").
		Scan(ctx, &registries)
	return registries, err
}

func (service *ContainerRegistries) CreateExternal(
	ctx context.Context,
	input ExternalContainerRegistryInput,
) (models.ContainerRegistryEntity, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Endpoint = normalizeRegistryEndpoint(input.Endpoint)
	input.Username = strings.TrimSpace(input.Username)
	if err := validateExternalRegistryInput(input); err != nil {
		return models.ContainerRegistryEntity{}, err
	}

	authentication, err := service.registry.Authenticate(ctx, registryclient.Credentials{
		Endpoint: input.Endpoint, Username: input.Username, Password: input.AccessToken,
	})
	if err != nil {
		return models.ContainerRegistryEntity{}, fmt.Errorf("verify external registry credentials: %w", err)
	}
	if err := authentication.Close(); err != nil {
		return models.ContainerRegistryEntity{}, fmt.Errorf("remove registry verification credentials: %w", err)
	}

	payload, err := json.Marshal(map[string]any{
		"schema_version": 1, "username": input.Username, "password": input.AccessToken,
	})
	if err != nil {
		return models.ContainerRegistryEntity{}, fmt.Errorf("encode external registry credential: %w", err)
	}
	encrypted, err := secretcrypto.EncryptForPurpose(payload, service.config.App.SessionEncryptionKey, registryCredentialPurpose)
	if err != nil {
		return models.ContainerRegistryEntity{}, fmt.Errorf("encrypt external registry credential: %w", err)
	}
	metadata, err := json.Marshal(map[string]any{
		"schema_version": 1, "credential_kind": "external_registry", "username": input.Username,
	})
	if err != nil {
		return models.ContainerRegistryEntity{}, fmt.Errorf("encode external registry metadata: %w", err)
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ContainerRegistryEntity{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "container-registry:"+input.Endpoint); err != nil {
		return models.ContainerRegistryEntity{}, err
	}
	count, err := tx.NewSelect().TableExpr("container_registries").Where("lower(endpoint) = lower(?)", input.Endpoint).Where("archived_at IS NULL").Count(ctx)
	if err != nil {
		return models.ContainerRegistryEntity{}, err
	}
	if count > 0 {
		return models.ContainerRegistryEntity{}, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: "endpoint", Code: "taken", Message: "registry endpoint is already connected"}})
	}
	now := time.Now().UTC()
	credential, err := models.Credential.Create(ctx, tx, models.CreateCredentialData{
		Name: input.Name, Provider: "container_registry", Metadata: metadata, EncPayload: encrypted,
		VerifiedAt: sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		return models.ContainerRegistryEntity{}, err
	}
	registry, err := models.ContainerRegistry.Create(ctx, tx, models.CreateContainerRegistryData{
		Name: input.Name, Provider: "distribution", Endpoint: input.Endpoint, CredentialID: credential.ID,
	})
	if err != nil {
		return models.ContainerRegistryEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ContainerRegistryEntity{}, err
	}
	return registry, nil
}

func (service *ContainerRegistries) ArchiveExternal(ctx context.Context, registryID uuid.UUID) error {
	registry, err := models.ContainerRegistry.Find(ctx, service.db.Executor(), registryID)
	if err != nil {
		return err
	}
	credential, err := models.Credential.Find(ctx, service.db.Executor(), registry.CredentialID)
	if err != nil {
		return err
	}
	var metadata struct {
		CredentialKind string `json:"credential_kind"`
	}
	if json.Unmarshal(credential.Metadata, &metadata) != nil || metadata.CredentialKind != "external_registry" {
		return errors.New("the DeployCrate-managed registry cannot be archived here")
	}
	references, err := service.db.Executor().NewSelect().TableExpr("buildpack_configurations").Where("container_registry_id = ?", registry.ID).Count(ctx)
	if err != nil {
		return err
	}
	if references > 0 {
		return errors.New("registry is still selected by an Application source")
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := sql.NullTime{Time: time.Now().UTC(), Valid: true}
	if _, err := models.ContainerRegistry.Update(ctx, tx, models.UpdateContainerRegistryData{
		ID: registry.ID, ArchivedAt: now, Name: registry.Name, Provider: registry.Provider,
		Endpoint: registry.Endpoint, CredentialID: registry.CredentialID,
	}); err != nil {
		return err
	}
	if _, err := models.Credential.Update(ctx, tx, models.UpdateCredentialData{
		ID: credential.ID, Name: credential.Name, Provider: credential.Provider, Metadata: credential.Metadata,
		EncPayload: credential.EncPayload, VerifiedAt: credential.VerifiedAt, LastUsedAt: credential.LastUsedAt, ArchivedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func normalizeRegistryEndpoint(endpoint string) string {
	endpoint = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(endpoint, "/")))
	switch endpoint {
	case "index.docker.io", "registry-1.docker.io":
		return "docker.io"
	default:
		return endpoint
	}
}

func validateExternalRegistryInput(input ExternalContainerRegistryInput) error {
	builder := validation.NewBuilder()
	builder.Required("name", input.Name)
	builder.Required("endpoint", input.Endpoint)
	builder.Required("username", input.Username)
	builder.Required("accessToken", input.AccessToken)
	if len(input.Name) > 120 {
		builder.Add("name", "too_long", "registry name must be 120 characters or fewer")
	}
	if len(input.Endpoint) > 255 {
		builder.Add("endpoint", "too_long", "registry endpoint must be 255 characters or fewer")
	}
	if len(input.Username) > 255 {
		builder.Add("username", "too_long", "registry username must be 255 characters or fewer")
	}
	if len(input.AccessToken) > 16384 {
		builder.Add("accessToken", "too_long", "registry access token is too large")
	}
	parsed, err := url.Parse("https://" + input.Endpoint)
	if err != nil || parsed.Host != input.Endpoint || parsed.Path != "" || parsed.User != nil || strings.ContainsAny(input.Endpoint, " \t\r\n") {
		builder.Add("endpoint", "invalid", "registry endpoint must be a hostname with an optional port")
	}
	return builder.Err()
}
