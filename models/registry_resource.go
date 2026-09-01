package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type RegistryResourceEntity struct {
	bun.BaseModel `bun:"table:registry_resources,alias:registry_resource"`
	ResourceID    uuid.UUID       `bun:"resource_id,pk,type:uuid"`
	CreatedAt     time.Time       `bun:"created_at"`
	UpdatedAt     time.Time       `bun:"updated_at"`
	Provider      string          `bun:"provider"`
	Configuration json.RawMessage `bun:"configuration,type:jsonb"`
}

type RegistryResourceSnapshot struct {
	Provider        string `bun:"provider"`
	Endpoint        string `bun:"endpoint"`
	EndpointCount   int    `bun:"endpoint_count"`
	CredentialCount int    `bun:"credential_count"`
}

type ManagedRegistryAccess struct {
	ResourceID         uuid.UUID       `bun:"resource_id"`
	ResourceEndpointID uuid.UUID       `bun:"resource_endpoint_id"`
	Configuration      json.RawMessage `bun:"configuration"`
	Username           string          `bun:"username"`
	CredentialMetadata json.RawMessage `bun:"credential_metadata"`
}

type RegistryResourceSummary struct {
	ID             uuid.UUID `json:"id"             bun:"id"`
	Name           string    `json:"name"           bun:"name"`
	Slug           string    `json:"slug"           bun:"slug"`
	Provider       string    `json:"provider"       bun:"provider"`
	Endpoint       string    `json:"endpoint"       bun:"endpoint"`
	Username       string    `json:"username"       bun:"username"`
	CredentialName string    `json:"credentialName" bun:"credential_name"`
	Managed        bool      `json:"managed"        bun:"managed"`
	CreatedAt      time.Time `json:"createdAt"      bun:"created_at"`
}

type RegistryPublisherCredential struct {
	Endpoint   string         `bun:"endpoint"`
	Username   sql.NullString `bun:"username"`
	EncPayload []byte         `bun:"enc_payload"`
}

func registryResourceSummaryQuery(db storage.Executor) *bun.SelectQuery {
	return db.NewSelect().
		TableExpr("registry_resources AS registry").
		ColumnExpr("resource.id, resource.name, resource.slug, registry.provider, resource.created_at").
		ColumnExpr("CASE WHEN resource.system_managed THEN COALESCE(NULLIF(registry.configuration ->> 'route_host', ''), endpoint.address) WHEN endpoint.port IN (80, 443) THEN endpoint.address ELSE endpoint.address || ':' || endpoint.port::text END AS endpoint").
		ColumnExpr("credential.name AS credential_name, COALESCE(credential.username, '') AS username").
		ColumnExpr("resource.system_managed AS managed").
		Join("JOIN resources AS resource ON resource.id = registry.resource_id AND resource.configuration ->> 'engine' = 'registry' AND resource.archived_at IS NULL").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.resource_id = resource.id AND endpoint.role = 'primary' AND endpoint.archived_at IS NULL").
		Join("JOIN resource_credentials AS credential ON credential.resource_id = resource.id AND credential.archived_at IS NULL")
}

func (registryResource) Summaries(
	ctx context.Context,
	db storage.Executor,
) ([]RegistryResourceSummary, error) {
	items := make([]RegistryResourceSummary, 0)
	err := registryResourceSummaryQuery(db).OrderExpr("resource.name ASC").Scan(ctx, &items)
	return items, err
}

func (registryResource) Summary(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
) (RegistryResourceSummary, error) {
	var item RegistryResourceSummary
	err := registryResourceSummaryQuery(db).
		Where("resource.id = ?", resourceID).
		Limit(1).
		Scan(ctx, &item)
	return item, err
}

func (registryResource) PublisherCredential(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
	managedOnly bool,
) (RegistryPublisherCredential, error) {
	query := db.NewSelect().
		TableExpr("registry_resources AS registry").
		ColumnExpr("CASE WHEN resource.system_managed THEN COALESCE(NULLIF(registry.configuration ->> 'route_host', ''), endpoint.address) WHEN endpoint.port IN (80, 443) THEN endpoint.address ELSE endpoint.address || ':' || endpoint.port::text END AS endpoint").
		ColumnExpr("credential.username, credential.enc_payload").
		Join("JOIN resources AS resource ON resource.id = registry.resource_id AND resource.archived_at IS NULL").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.resource_id = resource.id AND endpoint.role = 'primary' AND endpoint.archived_at IS NULL").
		Join("JOIN resource_credentials AS credential ON credential.resource_id = resource.id AND credential.archived_at IS NULL").
		Where("registry.resource_id = ?", resourceID).
		Where("credential.name = ?", "Registry publisher")
	if managedOnly {
		query = query.Where("resource.system_managed = TRUE")
	}
	var credential RegistryPublisherCredential
	err := query.Limit(1).Scan(ctx, &credential)
	return credential, err
}

func (registryResource) LockEndpoint(
	ctx context.Context,
	db storage.Executor,
	endpoint string,
) error {
	_, err := db.ExecContext(
		ctx,
		"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
		"registry-endpoint:"+endpoint,
	)
	return err
}

func (registryResource) EndpointConflictCount(
	ctx context.Context,
	db storage.Executor,
	host string,
	port int32,
) (int, error) {
	return db.NewSelect().
		TableExpr("resource_endpoints AS endpoint").
		Join("JOIN resources AS resource ON resource.id = endpoint.resource_id AND resource.configuration ->> 'engine' = 'registry' AND resource.archived_at IS NULL").
		Where("lower(endpoint.address) = lower(?)", host).
		Where("endpoint.port = ?", port).
		Where("endpoint.archived_at IS NULL").
		Count(ctx)
}

func (registryResource) ActiveSourceReferenceCount(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
) (int, error) {
	return db.NewSelect().
		TableExpr("environment_sources AS source").
		Join("LEFT JOIN buildpack_configurations AS buildpack ON buildpack.environment_source_id = source.id").
		Join("LEFT JOIN image_configurations AS image ON image.environment_source_id = source.id").
		Where("source.archived_at IS NULL").
		Where("COALESCE(buildpack.registry_resource_id, image.registry_resource_id) = ?", resourceID).
		Count(ctx)
}

func (registryResource) ArchiveDependents(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
	at time.Time,
) error {
	for _, table := range []string{"resource_credentials", "resource_endpoints"} {
		if _, err := db.NewUpdate().
			TableExpr(table).
			Set("archived_at = ?", at).
			Set("updated_at = ?", at).
			Where("resource_id = ?", resourceID).
			Where("archived_at IS NULL").
			Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (registryResource) ApplicationOptions(
	ctx context.Context,
	db storage.Executor,
) ([]ApplicationRegistryOption, error) {
	items := make([]ApplicationRegistryOption, 0)
	err := db.NewSelect().TableExpr("registry_resources AS registry").
		ColumnExpr("resource.id, resource.name").
		ColumnExpr("CASE WHEN endpoint.port IN (80, 443) THEN endpoint.address ELSE endpoint.address || ':' || endpoint.port::text END AS endpoint").
		Join("JOIN resources AS resource ON resource.id = registry.resource_id AND resource.archived_at IS NULL").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.resource_id = resource.id AND endpoint.role = 'primary' AND endpoint.archived_at IS NULL").
		Where("EXISTS (SELECT 1 FROM resource_credentials credential WHERE credential.resource_id = resource.id AND credential.archived_at IS NULL)").
		OrderExpr("resource.name ASC").Scan(ctx, &items)
	return items, err
}

func (registryResource) ValidApplicationSelection(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
) (bool, error) {
	var selection struct {
		Engine          string `bun:"engine"`
		EndpointCount   int    `bun:"endpoint_count"`
		CredentialCount int    `bun:"credential_count"`
	}
	err := db.NewSelect().TableExpr("registry_resources AS registry").
		ColumnExpr("resource.configuration ->> 'engine' AS engine").
		ColumnExpr("(SELECT count(*) FROM resource_endpoints endpoint WHERE endpoint.resource_id = resource.id AND endpoint.role = 'primary' AND endpoint.archived_at IS NULL) AS endpoint_count").
		ColumnExpr("(SELECT count(*) FROM resource_credentials credential WHERE credential.resource_id = resource.id AND credential.archived_at IS NULL) AS credential_count").
		Join("JOIN resources AS resource ON resource.id = registry.resource_id AND resource.archived_at IS NULL").
		Where("registry.resource_id = ?", resourceID).Scan(ctx, &selection)
	return selection.Engine == "registry" && selection.EndpointCount == 1 &&
		selection.CredentialCount == 1, err
}

func (registryResource) FindManagedAccess(
	ctx context.Context,
	db storage.Executor,
) (ManagedRegistryAccess, error) {
	var access ManagedRegistryAccess
	err := db.NewSelect().
		TableExpr("registry_resources AS registry").
		ColumnExpr("registry.resource_id, registry.configuration").
		ColumnExpr("endpoint.id AS resource_endpoint_id").
		ColumnExpr("credential.username, credential.metadata AS credential_metadata").
		Join("JOIN resources AS resource ON resource.id = registry.resource_id AND resource.configuration ->> 'engine' = 'registry' AND resource.system_managed = TRUE AND resource.archived_at IS NULL").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.resource_id = resource.id AND endpoint.role = 'primary' AND endpoint.archived_at IS NULL").
		Join("JOIN resource_credentials AS credential ON credential.resource_id = resource.id AND credential.archived_at IS NULL").
		Scan(ctx, &access)
	return access, err
}

func (registryResource) Snapshot(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
) (RegistryResourceSnapshot, error) {
	var snapshot RegistryResourceSnapshot
	err := db.NewSelect().
		TableExpr("registry_resources AS registry").
		ColumnExpr("registry.provider").
		ColumnExpr("CASE WHEN resource.system_managed THEN COALESCE(NULLIF(registry.configuration ->> 'route_host', ''), endpoint.address) WHEN endpoint.port IN (80, 443) THEN endpoint.address ELSE endpoint.address || ':' || endpoint.port::text END AS endpoint").
		ColumnExpr("(SELECT count(*) FROM resource_endpoints candidate WHERE candidate.resource_id = registry.resource_id AND candidate.archived_at IS NULL) AS endpoint_count").
		ColumnExpr("(SELECT count(*) FROM resource_credentials candidate WHERE candidate.resource_id = registry.resource_id AND candidate.archived_at IS NULL) AS credential_count").
		Join("JOIN resources AS resource ON resource.id = registry.resource_id AND resource.archived_at IS NULL").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.resource_id = registry.resource_id AND endpoint.role = 'primary' AND endpoint.archived_at IS NULL").
		Where("registry.resource_id = ?", resourceID).
		Scan(ctx, &snapshot)
	return snapshot, err
}

func (entity *RegistryResourceEntity) Validate() error {
	entity.Provider = strings.ToLower(strings.TrimSpace(entity.Provider))
	builder := validation.NewBuilder()
	if entity.ResourceID == uuid.Nil {
		builder.Add("resourceId", "required", "Registry Resource is required")
	}
	if entity.Provider != "distribution" {
		builder.Add("provider", "unsupported", "Registry provider must be distribution")
	}
	if !validJSONObject(entity.Configuration) {
		builder.Add("configuration", "invalid", "Registry configuration must be a JSON object")
	}
	return builder.Err()
}

func (registryResource) Find(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
) (RegistryResourceEntity, error) {
	var entity RegistryResourceEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("registry_resource.resource_id = ?", resourceID).
		Scan(ctx); err != nil {
		return RegistryResourceEntity{}, err
	}
	return entity, nil
}

type CreateRegistryResourceData struct {
	ResourceID    uuid.UUID
	Provider      string
	Configuration json.RawMessage
}

func (registryResource) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateRegistryResourceData,
) (RegistryResourceEntity, error) {
	now := time.Now().UTC()
	entity := RegistryResourceEntity{
		ResourceID: data.ResourceID, CreatedAt: now, UpdatedAt: now,
		Provider: data.Provider, Configuration: data.Configuration,
	}
	if err := validation.Validate(&entity); err != nil {
		return RegistryResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	var engine string
	if err := db.NewSelect().
		TableExpr("resources").
		ColumnExpr("configuration ->> 'engine'").
		Where("id = ?", entity.ResourceID).
		Where("archived_at IS NULL").
		Scan(ctx, &engine); err != nil {
		return RegistryResourceEntity{}, err
	}
	if engine != "registry" {
		return RegistryResourceEntity{}, errors.Join(
			ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "resourceId",
					Code:    "kind",
					Message: "Registry backing requires a Registry Resource",
				},
			},
		)
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return RegistryResourceEntity{}, err
	}
	return entity, nil
}

func (registryResource) Update(
	ctx context.Context,
	db storage.Executor,
	data CreateRegistryResourceData,
) (RegistryResourceEntity, error) {
	entity := RegistryResourceEntity{
		ResourceID:    data.ResourceID,
		UpdatedAt:     time.Now().UTC(),
		Provider:      data.Provider,
		Configuration: data.Configuration,
	}
	if err := validation.Validate(&entity); err != nil {
		return RegistryResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at", "provider", "configuration").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return RegistryResourceEntity{}, err
	}
	return entity, nil
}

func (registryResource) Destroy(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
) error {
	_, err := db.NewDelete().
		Model((*RegistryResourceEntity)(nil)).
		Where("resource_id = ?", resourceID).
		Exec(ctx)
	return err
}
