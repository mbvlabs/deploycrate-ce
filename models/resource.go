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

type ResourceEntity struct {
	bun.BaseModel         `bun:"table:resources,alias:resources"`
	ID                    uuid.UUID                  `bun:"id,pk,type:uuid"`
	CreatedAt             time.Time                  `bun:"created_at"`
	UpdatedAt             time.Time                  `bun:"updated_at"`
	Name                  string                     `bun:"name"`
	Slug                  string                     `bun:"slug"`
	ResourceType          ResourceTypeEnum           `bun:"resource_type"`
	Configuration         json.RawMessage            `bun:"configuration,type:jsonb"`
	SystemManaged         bool                       `bun:"system_managed"`
	EnvironmentAttachable bool                       `bun:"environment_attachable"`
	ArchivedAt            sql.NullTime               `bun:"archived_at"`
	Endpoints             []ResourceEndpointEntity   `bun:",rel:has-many,join:id=resource_id"`
	Credentials           []ResourceCredentialEntity `bun:",rel:has-many,join:id=resource_id"`
	Installation          ResourceInstallationEntity `bun:"rel:has-one,join:id=resource_id"`
}

type ResourceConfiguration struct {
	Engine          string                       `json:"engine"`
	Databases       []ResourceDatabaseDefinition `json:"databases,omitempty"`
	EnvironmentKeys map[string]string            `json:"environment_keys"`
}

type ResourceDatabaseDefinition struct {
	Name      string `json:"name"`
	Encoding  string `json:"encoding,omitempty"`
	Collation string `json:"collation,omitempty"`
}

var resourceEnvironmentReservedKeys = map[string]struct{}{
	"PORT":                       {},
	"DEPLOYCRATE_APPLICATION_ID": {},
	"DEPLOYCRATE_ENVIRONMENT_ID": {},
	"DEPLOYCRATE_RELEASE_ID":     {},
}

func (e ResourceEntity) ParsedConfiguration() ResourceConfiguration {
	var configuration ResourceConfiguration
	_ = json.Unmarshal(e.Configuration, &configuration)
	configuration.Engine = strings.ToLower(strings.TrimSpace(configuration.Engine))
	if definition, ok := FindResourceEngine(
		configuration.Engine,
	); ok &&
		len(configuration.EnvironmentKeys) == 0 {
		configuration.EnvironmentKeys = definition.DefaultEnvironmentKeys()
	}
	return configuration
}

func (e ResourceEntity) Engine() string {
	return e.ParsedConfiguration().Engine
}

func (e ResourceEntity) Databases() []ResourceDatabaseDefinition {
	if e.ResourceType != ResourceTypeDatabase {
		return nil
	}
	return e.ParsedConfiguration().Databases
}

func (e ResourceEntity) EnvironmentKeys() map[string]string {
	configuration := e.ParsedConfiguration()
	keys := make(map[string]string, len(configuration.EnvironmentKeys))
	for logicalName, key := range configuration.EnvironmentKeys {
		keys[logicalName] = NormalizeEnvironmentSecretKey(key)
	}
	return keys
}

func validSlug(value string) bool {
	if value == "" || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	previousHyphen := false
	for _, character := range value {
		if character == '-' {
			if previousHyphen {
				return false
			}
			previousHyphen = true
			continue
		}
		previousHyphen = false
		if character < 'a' || character > 'z' {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}

func (e *ResourceEntity) Validate() error {
	e.Name = strings.TrimSpace(e.Name)
	e.Slug = strings.ToLower(strings.TrimSpace(e.Slug))
	e.ResourceType = ResourceTypeEnum(strings.ToLower(strings.TrimSpace(e.ResourceType.String())))
	builder := validation.NewBuilder()
	builder.Required("name", e.Name)
	builder.Required("slug", e.Slug)
	if !validSlug(e.Slug) {
		builder.Add(
			"slug",
			"format",
			"slug must contain lowercase letters, numbers, and single hyphens",
		)
	}
	if !e.ResourceType.IsValid() {
		builder.Add("resourceType", "unsupported", "resource type is not supported")
	}
	if !validJSONObject(e.Configuration) {
		builder.Add("configuration", "invalid", "configuration must be a JSON object")
	} else if resourceConfigurationContainsSecret(e.Configuration) {
		builder.Add("configuration", "secret", "configuration must not contain raw credentials")
	} else {
		configuration := e.ParsedConfiguration()
		definition, ok := FindResourceEngine(configuration.Engine)
		if !ok || definition.ResourceType != e.ResourceType {
			builder.Add(
				"configuration.engine",
				"unsupported",
				"engine is not supported by this resource type",
			)
		} else {
			normalized := make(map[string]string, len(configuration.EnvironmentKeys))
			seen := make(map[string]string, len(configuration.EnvironmentKeys))
			supported := make(map[string]struct{}, len(definition.EnvironmentKeys))
			for _, keyDefinition := range definition.EnvironmentKeys {
				supported[keyDefinition.Name] = struct{}{}
				value := NormalizeEnvironmentSecretKey(
					configuration.EnvironmentKeys[keyDefinition.Name],
				)
				field := "configuration.environment_keys." + keyDefinition.Name
				if value == "" {
					builder.Add(field, "required", keyDefinition.Label+" key is required")
					continue
				}
				if err := ValidateEnvironmentSecretKey(value, false); err != nil {
					builder.Add(
						field,
						"format",
						"key must match [A-Z_][A-Z0-9_]* and must not be reserved",
					)
				}
				if _, reserved := resourceEnvironmentReservedKeys[value]; reserved {
					builder.Add(field, "reserved", "key is reserved by the platform")
				}
				if previous, exists := seen[value]; exists {
					builder.Add(field, "duplicate", "key is already used by "+previous)
				}
				seen[value] = keyDefinition.Label
				normalized[keyDefinition.Name] = value
			}
			for logicalName := range configuration.EnvironmentKeys {
				if _, exists := supported[logicalName]; !exists {
					builder.Add(
						"configuration.environment_keys."+logicalName,
						"unsupported",
						"key role is not supported by this Resource engine",
					)
				}
			}
			if err := builder.Err(); err == nil {
				var raw map[string]any
				_ = json.Unmarshal(e.Configuration, &raw)
				raw["environment_keys"] = normalized
				if encoded, err := json.Marshal(raw); err == nil {
					e.Configuration = encoded
				}
			}
		}
	}

	return builder.Err()
}

func resourceConfigurationContainsSecret(configuration json.RawMessage) bool {
	var values map[string]any
	if json.Unmarshal(configuration, &values) != nil {
		return false
	}
	delete(values, "environment_keys")
	filtered, err := json.Marshal(values)
	return err == nil && settingsContainSecret(filtered)
}

func (r resource) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ResourceEntity, error) {
	var entity ResourceEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceEntity{}, err
	}

	return entity, nil
}

type CreateResourceData struct {
	Name                  string
	Slug                  string
	ResourceType          ResourceTypeEnum
	Configuration         json.RawMessage
	SystemManaged         bool
	EnvironmentAttachable bool
	ArchivedAt            sql.NullTime
}

func (r resource) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceData,
) (ResourceEntity, error) {
	entity := ResourceEntity{
		ID:                    uuid.New(),
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
		Name:                  data.Name,
		Slug:                  data.Slug,
		ResourceType:          data.ResourceType,
		Configuration:         data.Configuration,
		ArchivedAt:            data.ArchivedAt,
		SystemManaged:         data.SystemManaged,
		EnvironmentAttachable: data.EnvironmentAttachable,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := r.ensureActiveNameAvailable(ctx, db, entity.Name, nil); err != nil {
		return ResourceEntity{}, err
	}
	if err := r.ensureActiveSlugAvailable(ctx, db, entity.Slug, nil); err != nil {
		return ResourceEntity{}, err
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceEntity{}, err
	}

	return entity, nil
}

type UpdateResourceData struct {
	ID                    uuid.UUID
	UpdatedAt             time.Time
	Name                  string
	Slug                  string
	ResourceType          ResourceTypeEnum
	Configuration         json.RawMessage
	SystemManaged         bool
	EnvironmentAttachable bool
	ArchivedAt            sql.NullTime
}

func (r resource) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateResourceData,
) (ResourceEntity, error) {
	entity := ResourceEntity{
		ID:                    data.ID,
		UpdatedAt:             time.Now(),
		Name:                  data.Name,
		Slug:                  data.Slug,
		ResourceType:          data.ResourceType,
		Configuration:         data.Configuration,
		ArchivedAt:            data.ArchivedAt,
		SystemManaged:         data.SystemManaged,
		EnvironmentAttachable: data.EnvironmentAttachable,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := r.ensureActiveNameAvailable(ctx, db, entity.Name, &entity.ID); err != nil {
		return ResourceEntity{}, err
	}
	if err := r.ensureActiveSlugAvailable(ctx, db, entity.Slug, &entity.ID); err != nil {
		return ResourceEntity{}, err
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("slug").
		Column("resource_type").
		Column("configuration").
		Column("system_managed").
		Column("environment_attachable").
		Column("archived_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceEntity{}, err
	}

	return entity, nil
}

func (r resource) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ResourceEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (r resource) All(ctx context.Context, db storage.Executor) ([]ResourceEntity, error) {
	var entities []ResourceEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type AttachableResource struct {
	ID            uuid.UUID       `bun:"id"`
	Name          string          `bun:"name"`
	Engine        string          `bun:"engine"`
	Configuration json.RawMessage `bun:"resource_configuration"`
	Database      string          `bun:"database_name"`
	EndpointID    uuid.UUID       `bun:"endpoint_id"`
	Endpoint      string          `bun:"endpoint"`
	CredentialID  *uuid.UUID      `bun:"credential_id"`
	Credential    string          `bun:"credential"`
	ServerID      *uuid.UUID      `bun:"server_id"`
}

func (r resource) AllAttachable(
	ctx context.Context,
	db storage.Executor,
) ([]AttachableResource, error) {
	options := make([]AttachableResource, 0)
	if err := db.NewSelect().
		TableExpr("resources AS resource").
		ColumnExpr("resource.id, resource.name, resource.configuration ->> 'engine' AS engine, resource.configuration AS resource_configuration, COALESCE(credential.metadata ->> 'database', '') AS database_name, endpoint.id AS endpoint_id").
		ColumnExpr("endpoint.address || ':' || endpoint.port::text AS endpoint").
		ColumnExpr("credential.id AS credential_id, COALESCE(credential.name, '') AS credential, installation.server_id AS server_id").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.resource_id = resource.id AND endpoint.archived_at IS NULL").
		Join("LEFT JOIN resource_installations AS installation ON installation.resource_id = resource.id AND installation.archived_at IS NULL").
		Join("LEFT JOIN resource_credentials AS credential ON credential.resource_id = resource.id AND credential.metadata ->> 'purpose' = 'application' AND credential.metadata ->> 'environment_id' IS NULL AND credential.archived_at IS NULL").
		Where("resource.archived_at IS NULL").
		Where("resource.environment_attachable = TRUE").
		Where("resource.configuration ->> 'engine' <> 'opentelemetry' OR endpoint.settings ->> 'exposure' = ?", ResourceEndpointExposureEnvironment).
		OrderExpr("resource.name, endpoint.role, credential.name").
		Scan(ctx, &options); err != nil {
		return nil, err
	}

	return options, nil
}

type PaginatedResources struct {
	Resources  []ResourceEntity
	TotalCount int64
	Page       int64
	PageSize   int64
	TotalPages int64
}

func (r resource) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedResources, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	totalCount, err := db.NewSelect().
		Model(&ResourceEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResources{}, err
	}

	entities := make([]ResourceEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResources{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResources{
		Resources:  entities,
		TotalCount: int64(totalCount),
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (r resource) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceData,
) (ResourceEntity, error) {
	entity := ResourceEntity{
		ID:                    uuid.New(),
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
		Name:                  data.Name,
		Slug:                  data.Slug,
		ResourceType:          data.ResourceType,
		Configuration:         data.Configuration,
		ArchivedAt:            data.ArchivedAt,
		SystemManaged:         data.SystemManaged,
		EnvironmentAttachable: data.EnvironmentAttachable,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := r.ensureActiveNameAvailable(ctx, db, entity.Name, &entity.ID); err != nil {
		return ResourceEntity{}, err
	}
	if err := r.ensureActiveSlugAvailable(ctx, db, entity.Slug, &entity.ID); err != nil {
		return ResourceEntity{}, err
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("slug = excluded.slug").
		Set("resource_type = excluded.resource_type").
		Set("configuration = excluded.configuration").
		Set("system_managed = excluded.system_managed").
		Set("environment_attachable = excluded.environment_attachable").
		Set("archived_at = excluded.archived_at").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceEntity{}, err
	}

	return entity, nil
}

func (r resource) ensureActiveSlugAvailable(
	ctx context.Context,
	db storage.Executor,
	slug string,
	exceptID *uuid.UUID,
) error {
	normalizedSlug := strings.ToLower(strings.TrimSpace(slug))
	if _, err := db.ExecContext(
		ctx,
		"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
		"resource-slug:"+normalizedSlug,
	); err != nil {
		return err
	}
	query := db.NewSelect().Model((*ResourceEntity)(nil)).
		Where("lower(slug) = ?", normalizedSlug).
		Where("archived_at IS NULL")
	if exceptID != nil {
		query = query.Where("id <> ?", *exceptID)
	}
	count, err := query.Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.Join(
			ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "slug",
					Code:    "taken",
					Message: "an active Resource already uses this slug",
				},
			},
		)
	}
	return nil
}

func (r resource) ensureActiveNameAvailable(
	ctx context.Context,
	db storage.Executor,
	name string,
	exceptID *uuid.UUID,
) error {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if _, err := db.ExecContext(
		ctx,
		"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
		"resource-name:"+normalizedName,
	); err != nil {
		return err
	}
	query := db.NewSelect().Model((*ResourceEntity)(nil)).
		Where("lower(name) = ?", normalizedName).
		Where("archived_at IS NULL")
	if exceptID != nil {
		query = query.Where("id <> ?", *exceptID)
	}
	count, err := query.Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.Join(
			ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "name",
					Code:    "taken",
					Message: "an active Resource already uses this name",
				},
			},
		)
	}
	return nil
}
