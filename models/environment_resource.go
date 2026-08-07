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

type EnvironmentResourceEntity struct {
	bun.BaseModel        `bun:"table:environment_resources,alias:environment_resources"`
	ID                   uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt            time.Time       `bun:"created_at"`
	UpdatedAt            time.Time       `bun:"updated_at"`
	Alias                string          `bun:"alias"`
	Configuration        json.RawMessage `bun:"configuration,type:jsonb"`
	ArchivedAt           sql.NullTime    `bun:"archived_at"`
	EnvironmentID        uuid.UUID       `bun:"environment_id,type:uuid"`
	ResourceID           uuid.UUID       `bun:"resource_id,type:uuid"`
	ResourceEndpointID   uuid.UUID       `bun:"resource_endpoint_id,type:uuid"`
	ResourceCredentialID *uuid.UUID      `bun:"resource_credential_id,type:uuid"`
}

type EnvironmentResourceConnection struct {
	ResourceType     ResourceTypeEnum `bun:"resource_type"`
	Engine           string           `bun:"engine"`
	ResourceConfig   json.RawMessage  `bun:"resource_configuration"`
	Address          string           `bun:"address"`
	Port             int32            `bun:"port"`
	Protocol         string           `bun:"protocol"`
	TLSMode          string           `bun:"tls_mode"`
	Settings         json.RawMessage  `bun:"settings"`
	CredentialSource string           `bun:"credential_source"`
}

func (e *EnvironmentResourceEntity) Validate() error {
	e.Alias = strings.TrimSpace(e.Alias)
	builder := validation.NewBuilder()
	builder.Required("alias", e.Alias)
	if e.EnvironmentID == uuid.Nil {
		builder.Add("environmentId", "required", "environment is required")
	}
	if e.ResourceID == uuid.Nil {
		builder.Add("resourceId", "required", "Resource is required")
	}
	if e.ResourceEndpointID == uuid.Nil {
		builder.Add("resourceEndpointId", "required", "endpoint is required")
	}
	return builder.Err()
}

func (er environmentResource) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentResourceEntity, error) {
	var entity EnvironmentResourceEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return EnvironmentResourceEntity{}, err
	}

	return entity, nil
}

func (er environmentResource) HasActiveEngineForEnvironment(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
	engine string,
) (bool, error) {
	count, err := db.NewSelect().
		TableExpr("environment_resources AS binding").
		Join("JOIN resources AS resource ON resource.id = binding.resource_id AND resource.archived_at IS NULL").
		Where("binding.environment_id = ?", environmentID).
		Where("lower(resource.configuration ->> 'engine') = ?", strings.ToLower(strings.TrimSpace(engine))).
		Where("binding.archived_at IS NULL").
		Count(ctx)
	return count > 0, err
}

func (er environmentResource) FindConnectionByApplicationAndAlias(
	ctx context.Context,
	db storage.Executor,
	applicationSlug string,
	alias string,
) (EnvironmentResourceConnection, error) {
	var connection EnvironmentResourceConnection
	if err := db.NewSelect().
		TableExpr("environment_resources AS binding").
		ColumnExpr("resource.resource_type, resource.configuration ->> 'engine' AS engine, resource.configuration AS resource_configuration").
		ColumnExpr("endpoint.address AS address").
		ColumnExpr("endpoint.port AS port").
		ColumnExpr("endpoint.protocol AS protocol").
		ColumnExpr("endpoint.tls_mode AS tls_mode").
		ColumnExpr("endpoint.settings AS settings").
		ColumnExpr("COALESCE(binding.configuration ->> 'credential_source', '') AS credential_source").
		Join("JOIN environments AS environment ON environment.id = binding.environment_id AND environment.archived_at IS NULL").
		Join("JOIN applications AS application ON application.id = environment.application_id AND application.archived_at IS NULL").
		Join("JOIN resources AS resource ON resource.id = binding.resource_id AND resource.archived_at IS NULL").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.id = binding.resource_endpoint_id AND endpoint.archived_at IS NULL").
		Where("application.slug = ?", applicationSlug).
		Where("binding.alias = ?", alias).
		Where("binding.archived_at IS NULL").
		OrderExpr("binding.created_at DESC").
		Limit(1).
		Scan(ctx, &connection); err != nil {
		return EnvironmentResourceConnection{}, err
	}
	return connection, nil
}

type CreateEnvironmentResourceData struct {
	ID                   uuid.UUID
	Alias                string
	Configuration        json.RawMessage
	ArchivedAt           sql.NullTime
	EnvironmentID        uuid.UUID
	ResourceID           uuid.UUID
	ResourceEndpointID   uuid.UUID
	ResourceCredentialID *uuid.UUID
}

func (er environmentResource) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentResourceData,
) (EnvironmentResourceEntity, error) {
	entity := EnvironmentResourceEntity{
		ID:                   data.ID,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		Alias:                data.Alias,
		Configuration:        data.Configuration,
		ArchivedAt:           data.ArchivedAt,
		EnvironmentID:        data.EnvironmentID,
		ResourceID:           data.ResourceID,
		ResourceEndpointID:   data.ResourceEndpointID,
		ResourceCredentialID: data.ResourceCredentialID,
	}
	if entity.ID == uuid.Nil {
		entity.ID = uuid.New()
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := er.ensureActiveConnectionAvailable(ctx, db, entity, nil); err != nil {
		return EnvironmentResourceEntity{}, err
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentResourceEntity{}, err
	}

	return entity, nil
}

type UpdateEnvironmentResourceData struct {
	ID                   uuid.UUID
	UpdatedAt            time.Time
	Alias                string
	Configuration        json.RawMessage
	ArchivedAt           sql.NullTime
	EnvironmentID        uuid.UUID
	ResourceID           uuid.UUID
	ResourceEndpointID   uuid.UUID
	ResourceCredentialID *uuid.UUID
}

func (er environmentResource) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateEnvironmentResourceData,
) (EnvironmentResourceEntity, error) {
	entity := EnvironmentResourceEntity{
		ID:                   data.ID,
		UpdatedAt:            time.Now(),
		Alias:                data.Alias,
		Configuration:        data.Configuration,
		ArchivedAt:           data.ArchivedAt,
		EnvironmentID:        data.EnvironmentID,
		ResourceID:           data.ResourceID,
		ResourceEndpointID:   data.ResourceEndpointID,
		ResourceCredentialID: data.ResourceCredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := er.ensureActiveConnectionAvailable(ctx, db, entity, &entity.ID); err != nil {
		return EnvironmentResourceEntity{}, err
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("alias").
		Column("configuration").
		Column("archived_at").
		Column("environment_id").
		Column("resource_id").
		Column("resource_endpoint_id").
		Column("resource_credential_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentResourceEntity{}, err
	}

	return entity, nil
}

func (er environmentResource) Destroy(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) error {
	_, err := db.NewDelete().
		Model((*EnvironmentResourceEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (er environmentResource) Archive(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	now := time.Now().UTC()
	result, err := db.NewUpdate().Model((*EnvironmentResourceEntity)(nil)).
		Set("archived_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Where("archived_at IS NULL").
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (er environmentResource) ensureActiveConnectionAvailable(ctx context.Context, db storage.Executor, entity EnvironmentResourceEntity, exceptID *uuid.UUID) error {
	lockKeys := []string{
		"environment-resource:" + entity.EnvironmentID.String() + ":" + entity.ResourceID.String(),
		"environment-resource-alias:" + entity.EnvironmentID.String() + ":" + strings.ToLower(entity.Alias),
	}
	for _, lockKey := range lockKeys {
		if _, err := db.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", lockKey); err != nil {
			return err
		}
	}
	base := func() *bun.SelectQuery {
		query := db.NewSelect().Model((*EnvironmentResourceEntity)(nil)).Where("archived_at IS NULL")
		if exceptID != nil {
			query = query.Where("id <> ?", *exceptID)
		}
		return query
	}
	count, err := base().Where("environment_id = ?", entity.EnvironmentID).Where("resource_id = ?", entity.ResourceID).Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "environmentId", Code: "taken", Message: "Environment is already connected to this Resource"}})
	}
	count, err = base().Where("environment_id = ?", entity.EnvironmentID).Where("lower(alias) = ?", strings.ToLower(entity.Alias)).Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "alias", Code: "taken", Message: "alias is already in use in this Environment"}})
	}
	return nil
}

func (er environmentResource) All(
	ctx context.Context,
	db storage.Executor,
) ([]EnvironmentResourceEntity, error) {
	var entities []EnvironmentResourceEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedEnvironmentResources struct {
	EnvironmentResources []EnvironmentResourceEntity
	TotalCount           int64
	Page                 int64
	PageSize             int64
	TotalPages           int64
}

func (er environmentResource) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedEnvironmentResources, error) {
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
		Model(&EnvironmentResourceEntity{}).Count(ctx)
	if err != nil {
		return PaginatedEnvironmentResources{}, err
	}

	entities := make([]EnvironmentResourceEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedEnvironmentResources{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedEnvironmentResources{
		EnvironmentResources: entities,
		TotalCount:           int64(totalCount),
		Page:                 page,
		PageSize:             pageSize,
		TotalPages:           totalPages,
	}, nil
}

func (er environmentResource) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentResourceData,
) (EnvironmentResourceEntity, error) {
	entity := EnvironmentResourceEntity{
		ID:                   uuid.New(),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		Alias:                data.Alias,
		Configuration:        data.Configuration,
		ArchivedAt:           data.ArchivedAt,
		EnvironmentID:        data.EnvironmentID,
		ResourceID:           data.ResourceID,
		ResourceEndpointID:   data.ResourceEndpointID,
		ResourceCredentialID: data.ResourceCredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return EnvironmentResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := er.ensureActiveConnectionAvailable(ctx, db, entity, &entity.ID); err != nil {
		return EnvironmentResourceEntity{}, err
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("alias = excluded.alias").
		Set("configuration = excluded.configuration").
		Set("archived_at = excluded.archived_at").
		Set("environment_id = excluded.environment_id").
		Set("resource_id = excluded.resource_id").
		Set("resource_endpoint_id = excluded.resource_endpoint_id").
		Set("resource_credential_id = excluded.resource_credential_id").
		Returning("*").
		Scan(ctx); err != nil {
		return EnvironmentResourceEntity{}, err
	}

	return entity, nil
}
