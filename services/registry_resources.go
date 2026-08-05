package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	registryclient "deploycrate-ce/clients/registry"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"

	"github.com/google/uuid"
	"github.com/gosimple/slug"
)

const registryCredentialPurpose = "resource-credential/v1"

var ErrManagedRegistryUnavailable = errors.New("managed Registry is unavailable")

type RegistryResources struct {
	db       storage.Pool
	config   config.Config
	registry registryclient.Client
	identity Identity
}

func NewRegistryResources(db storage.Pool, cfg config.Config, identity Identity) *RegistryResources {
	return &RegistryResources{db: db, config: cfg, registry: registryclient.New(), identity: identity}
}

type ExternalRegistryResourceInput struct {
	Name        string
	Endpoint    string
	Username    string
	AccessToken string
}

type RegistryResourceSummary struct {
	ID             uuid.UUID `json:"id" bun:"id"`
	Name           string    `json:"name" bun:"name"`
	Slug           string    `json:"slug" bun:"slug"`
	Provider       string    `json:"provider" bun:"provider"`
	Endpoint       string    `json:"endpoint" bun:"endpoint"`
	Username       string    `json:"username" bun:"username"`
	CredentialName string    `json:"credentialName" bun:"credential_name"`
	Managed        bool      `json:"managed" bun:"managed"`
	CreatedAt      time.Time `json:"createdAt" bun:"created_at"`
}

type ManagedRegistryCredentials struct {
	Endpoint string `json:"endpoint"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type RegistryRepositorySummary struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

func (service *RegistryResources) List(ctx context.Context) ([]RegistryResourceSummary, error) {
	registries := make([]RegistryResourceSummary, 0)
	err := service.db.Executor().NewSelect().TableExpr("registry_resources AS registry").
		ColumnExpr("resource.id, resource.name, resource.slug, registry.provider, resource.created_at").
		ColumnExpr("CASE WHEN resource.system_managed THEN COALESCE(NULLIF(registry.configuration ->> 'route_host', ''), endpoint.address) WHEN endpoint.port IN (80, 443) THEN endpoint.address ELSE endpoint.address || ':' || endpoint.port::text END AS endpoint").
		ColumnExpr("credential.name AS credential_name, COALESCE(credential.username, '') AS username").
		ColumnExpr("resource.system_managed AS managed").
		Join("JOIN resources AS resource ON resource.id = registry.resource_id AND resource.configuration ->> 'engine' = 'registry' AND resource.archived_at IS NULL").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.resource_id = resource.id AND endpoint.role = 'primary' AND endpoint.archived_at IS NULL").
		Join("JOIN resource_credentials AS credential ON credential.resource_id = resource.id AND credential.archived_at IS NULL").
		OrderExpr("resource.name ASC").Scan(ctx, &registries)
	return registries, err
}

func (service *RegistryResources) Find(ctx context.Context, resourceID uuid.UUID) (RegistryResourceSummary, error) {
	var registry RegistryResourceSummary
	err := service.db.Executor().NewSelect().TableExpr("registry_resources AS registry").
		ColumnExpr("resource.id, resource.name, resource.slug, registry.provider, resource.created_at").
		ColumnExpr("CASE WHEN resource.system_managed THEN COALESCE(NULLIF(registry.configuration ->> 'route_host', ''), endpoint.address) WHEN endpoint.port IN (80, 443) THEN endpoint.address ELSE endpoint.address || ':' || endpoint.port::text END AS endpoint").
		ColumnExpr("credential.name AS credential_name, COALESCE(credential.username, '') AS username").
		ColumnExpr("resource.system_managed AS managed").
		Join("JOIN resources AS resource ON resource.id = registry.resource_id AND resource.configuration ->> 'engine' = 'registry' AND resource.archived_at IS NULL").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.resource_id = resource.id AND endpoint.role = 'primary' AND endpoint.archived_at IS NULL").
		Join("JOIN resource_credentials AS credential ON credential.resource_id = resource.id AND credential.archived_at IS NULL").
		Where("resource.id = ?", resourceID).
		Limit(1).
		Scan(ctx, &registry)
	if errors.Is(err, sql.ErrNoRows) {
		return RegistryResourceSummary{}, models.ErrNotFound
	}
	return registry, err
}

func (service *RegistryResources) RevealManagedCredentials(
	ctx context.Context,
	resourceID, userID uuid.UUID,
	password string,
) (ManagedRegistryCredentials, error) {
	if err := service.identity.VerifyUserPassword(ctx, userID, password); err != nil {
		return ManagedRegistryCredentials{}, err
	}
	return service.managedCredentials(ctx, resourceID)
}

func (service *RegistryResources) Inventory(ctx context.Context, resourceID uuid.UUID) ([]RegistryRepositorySummary, error) {
	credentials, err := service.managedCredentials(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	repositories, err := service.registry.ListRepositories(ctx, registryclient.Credentials{
		Endpoint: credentials.Endpoint,
		Username: credentials.Username,
		Password: credentials.Password,
	})
	if err != nil {
		return nil, err
	}
	summaries := make([]RegistryRepositorySummary, 0, len(repositories))
	for _, repository := range repositories {
		summaries = append(summaries, RegistryRepositorySummary{Name: repository.Name, Tags: repository.Tags})
	}
	return summaries, nil
}

func (service *RegistryResources) managedCredentials(ctx context.Context, resourceID uuid.UUID) (ManagedRegistryCredentials, error) {
	var row struct {
		Endpoint   string         `bun:"endpoint"`
		Username   sql.NullString `bun:"username"`
		EncPayload []byte         `bun:"enc_payload"`
	}
	err := service.db.Executor().NewSelect().TableExpr("registry_resources AS registry").
		ColumnExpr("registry.configuration ->> 'route_host' AS endpoint").
		ColumnExpr("credential.username, credential.enc_payload").
		Join("JOIN resources AS resource ON resource.id = registry.resource_id AND resource.system_managed = TRUE AND resource.archived_at IS NULL").
		Join("JOIN resource_credentials AS credential ON credential.resource_id = resource.id AND credential.archived_at IS NULL").
		Where("registry.resource_id = ?", resourceID).
		Where("credential.name = ?", "Registry publisher").
		Limit(1).
		Scan(ctx, &row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ManagedRegistryCredentials{}, ErrManagedRegistryUnavailable
		}
		return ManagedRegistryCredentials{}, fmt.Errorf("load managed Registry credential: %w", err)
	}
	if strings.TrimSpace(row.Endpoint) == "" || !row.Username.Valid || strings.TrimSpace(row.Username.String) == "" {
		return ManagedRegistryCredentials{}, ErrManagedRegistryUnavailable
	}

	plaintext, err := secretcrypto.DecryptForPurpose(
		row.EncPayload,
		service.config.App.SessionEncryptionKey,
		registryCredentialPurpose,
	)
	if err != nil {
		return ManagedRegistryCredentials{}, errors.New("managed Registry credential cannot be decrypted")
	}
	var payload struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}
	if json.Unmarshal(plaintext, &payload) != nil || payload.SchemaVersion != 1 || payload.Values["password"] == "" {
		return ManagedRegistryCredentials{}, errors.New("managed Registry credential payload is invalid")
	}

	return ManagedRegistryCredentials{
		Endpoint: strings.TrimSpace(row.Endpoint),
		Username: strings.TrimSpace(row.Username.String),
		Password: payload.Values["password"],
	}, nil
}

func (service *RegistryResources) CreateExternal(ctx context.Context, input ExternalRegistryResourceInput) (models.RegistryResourceEntity, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Endpoint = normalizeRegistryEndpoint(input.Endpoint)
	input.Username = strings.TrimSpace(input.Username)
	if err := validateExternalRegistryInput(input); err != nil {
		return models.RegistryResourceEntity{}, err
	}
	authentication, err := service.registry.Authenticate(ctx, registryclient.Credentials{Endpoint: input.Endpoint, Username: input.Username, Password: input.AccessToken})
	if err != nil {
		return models.RegistryResourceEntity{}, domainError("accessToken", "unverified", "Registry credentials could not be verified")
	}
	if err := authentication.Close(); err != nil {
		return models.RegistryResourceEntity{}, domainError("accessToken", "unverified", "Registry credentials could not be verified")
	}

	host, port, protocol, tlsMode, err := registryEndpointParts(input.Endpoint)
	if err != nil {
		return models.RegistryResourceEntity{}, err
	}
	payload, err := json.Marshal(struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}{SchemaVersion: 1, Values: map[string]string{"password": input.AccessToken}})
	if err != nil {
		return models.RegistryResourceEntity{}, err
	}
	encrypted, err := secretcrypto.EncryptForPurpose(payload, service.config.App.SessionEncryptionKey, registryCredentialPurpose)
	if err != nil {
		return models.RegistryResourceEntity{}, fmt.Errorf("encrypt external Registry credential: %w", err)
	}
	key, err := hex.DecodeString(service.config.App.SessionEncryptionKey)
	if err != nil || len(key) != 32 {
		return models.RegistryResourceEntity{}, errors.New("Registry credential digest key is invalid")
	}
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write(payload)
	metadata := json.RawMessage(`{"schema_version":1,"roles":["push","pull"]}`)

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.RegistryResourceEntity{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "registry-endpoint:"+input.Endpoint); err != nil {
		return models.RegistryResourceEntity{}, err
	}
	count, err := tx.NewSelect().TableExpr("resource_endpoints AS endpoint").Join("JOIN resources AS resource ON resource.id = endpoint.resource_id AND resource.configuration ->> 'engine' = 'registry' AND resource.archived_at IS NULL").Where("lower(endpoint.address) = lower(?)", host).Where("endpoint.port = ?", port).Where("endpoint.archived_at IS NULL").Count(ctx)
	if err != nil {
		return models.RegistryResourceEntity{}, err
	}
	if count > 0 {
		return models.RegistryResourceEntity{}, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: "endpoint", Code: "taken", Message: "Registry endpoint is already connected"}})
	}
	resource, err := models.Resource.Create(ctx, tx, models.CreateResourceData{Name: input.Name, Slug: slug.Make(input.Name), ResourceType: models.ResourceTypeService, Configuration: json.RawMessage(`{"engine":"registry"}`), EnvironmentAttachable: false})
	if err != nil {
		return models.RegistryResourceEntity{}, err
	}
	registry, err := models.RegistryResource.Create(ctx, tx, models.CreateRegistryResourceData{ResourceID: resource.ID, Provider: "distribution", Configuration: json.RawMessage(`{"schema_version":1}`)})
	if err != nil {
		return models.RegistryResourceEntity{}, err
	}
	if _, err := models.ResourceEndpoint.Create(ctx, tx, models.CreateResourceEndpointData{Name: "Registry API", Role: "primary", Address: host, Port: port, Protocol: protocol, TlsMode: tlsMode, Settings: json.RawMessage(`{"health_path":"/v2/"}`), ResourceID: resource.ID}); err != nil {
		return models.RegistryResourceEntity{}, err
	}
	if _, err := models.ResourceCredential.Create(ctx, tx, models.CreateResourceCredentialData{Name: "Registry publisher", Username: sql.NullString{String: input.Username, Valid: true}, Metadata: metadata, EncPayload: encrypted, Digest: digest.Sum(nil), ResourceID: resource.ID}); err != nil {
		return models.RegistryResourceEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.RegistryResourceEntity{}, err
	}
	return registry, nil
}

func (service *RegistryResources) ArchiveExternal(ctx context.Context, resourceID uuid.UUID) error {
	resource, err := models.Resource.Find(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return err
	}
	if resource.Engine() != "registry" || resource.SystemManaged {
		return errors.New("the DeployCrate-managed Registry cannot be archived here")
	}
	references, err := service.db.Executor().NewSelect().TableExpr("environment_sources AS source").
		Join("LEFT JOIN buildpack_configurations AS buildpack ON buildpack.environment_source_id = source.id").
		Join("LEFT JOIN image_configurations AS image ON image.environment_source_id = source.id").
		Where("source.archived_at IS NULL").Where("COALESCE(buildpack.registry_resource_id, image.registry_resource_id) = ?", resource.ID).Count(ctx)
	if err != nil {
		return err
	}
	if references > 0 {
		return errors.New("Registry is still selected by an Application deployment source")
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	if _, err := tx.NewUpdate().TableExpr("resource_credentials").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("resource_id = ?", resource.ID).Where("archived_at IS NULL").Exec(ctx); err != nil {
		return err
	}
	if _, err := tx.NewUpdate().TableExpr("resource_endpoints").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("resource_id = ?", resource.ID).Where("archived_at IS NULL").Exec(ctx); err != nil {
		return err
	}
	resource.ArchivedAt = sql.NullTime{Time: now, Valid: true}
	if _, err := models.Resource.Update(ctx, tx, models.UpdateResourceData{ID: resource.ID, Name: resource.Name, Slug: resource.Slug, ResourceType: resource.ResourceType, Configuration: resource.Configuration, SystemManaged: resource.SystemManaged, EnvironmentAttachable: resource.EnvironmentAttachable, ArchivedAt: resource.ArchivedAt}); err != nil {
		return err
	}
	return tx.Commit()
}

func normalizeRegistryEndpoint(endpoint string) string {
	endpoint = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(endpoint, "/")))
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	switch endpoint {
	case "index.docker.io", "registry-1.docker.io":
		return "docker.io"
	default:
		return endpoint
	}
}

func registryEndpointParts(endpoint string) (string, int32, string, string, error) {
	host := endpoint
	port := int32(443)
	if parsedHost, parsedPort, err := net.SplitHostPort(endpoint); err == nil {
		value, conversionErr := strconv.Atoi(parsedPort)
		if conversionErr != nil || value < 1 || value > 65535 {
			return "", 0, "", "", errors.New("Registry endpoint port is invalid")
		}
		host = parsedHost
		port = int32(value)
	}
	protocol, tlsMode := "https", "require"
	if port == 80 {
		protocol, tlsMode = "http", "disable"
	}
	return host, port, protocol, tlsMode, nil
}

func validateExternalRegistryInput(input ExternalRegistryResourceInput) error {
	builder := validation.NewBuilder()
	builder.Required("name", input.Name)
	builder.Required("endpoint", input.Endpoint)
	builder.Required("username", input.Username)
	builder.Required("accessToken", input.AccessToken)
	if len(input.Name) > 120 {
		builder.Add("name", "too_long", "Registry name must be 120 characters or fewer")
	}
	if len(input.Endpoint) > 255 {
		builder.Add("endpoint", "too_long", "Registry endpoint must be 255 characters or fewer")
	}
	if len(input.Username) > 255 {
		builder.Add("username", "too_long", "Registry username must be 255 characters or fewer")
	}
	if len(input.AccessToken) > 16384 {
		builder.Add("accessToken", "too_long", "Registry access token is too large")
	}
	parsed, err := url.Parse("https://" + input.Endpoint)
	if err != nil || parsed.Host != input.Endpoint || parsed.Path != "" || parsed.User != nil || strings.ContainsAny(input.Endpoint, " \t\r\n") {
		builder.Add("endpoint", "invalid", "Registry endpoint must be a hostname with an optional port")
	}
	return builder.Err()
}
