package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ResourceEntity struct {
	bun.BaseModel  `bun:"table:resources,alias:resources"`
	ID             uuid.UUID                  `bun:"id,pk,type:uuid"`
	CreatedAt      time.Time                  `bun:"created_at"`
	UpdatedAt      time.Time                  `bun:"updated_at"`
	Name           string                     `bun:"name"`
	Category       string                     `bun:"category"`
	Kind           string                     `bun:"kind"`
	DatabaseName   string                     `bun:"database_name"`
	ManagementMode ResourceManagementModeEnum `bun:"management_mode"`
	SharingScope   ResourceSharingScopeEnum   `bun:"sharing_scope"`
	SystemManaged  bool                       `bun:"system_managed"`
	ArchivedAt     sql.NullTime               `bun:"archived_at"`
}

func (e *ResourceEntity) Validate() error {
	e.Name = strings.TrimSpace(e.Name)
	e.Category = strings.ToLower(strings.TrimSpace(e.Category))
	e.Kind = strings.ToLower(strings.TrimSpace(e.Kind))
	e.DatabaseName = strings.TrimSpace(e.DatabaseName)
	e.ManagementMode = ResourceManagementModeEnum(strings.ToLower(strings.TrimSpace(e.ManagementMode.String())))
	e.SharingScope = ResourceSharingScopeEnum(strings.ToLower(strings.TrimSpace(e.SharingScope.String())))
	builder := validation.NewBuilder()
	builder.Required("name", e.Name)
	if !ResourceCategoryKindSupported(e.Category, e.Kind) {
		builder.Add("kind", "unsupported", "category and kind must match a supported resource kind")
	}
	if e.Kind == "postgresql" {
		builder.Required("databaseName", e.DatabaseName)
		if len([]byte(e.DatabaseName)) > 63 || strings.ContainsRune(e.DatabaseName, '\x00') {
			builder.Add("databaseName", "format", "PostgreSQL database name must be at most 63 bytes and cannot contain null bytes")
		}
	}
	if !e.ManagementMode.IsValid() {
		builder.Add("managementMode", "unsupported", "management mode must be managed or external")
	}
	if !e.SharingScope.IsValid() {
		builder.Add("sharingScope", "unsupported", "sharing scope is not supported")
	}
	return builder.Err()
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
	Name           string
	Category       string
	Kind           string
	DatabaseName   string
	ManagementMode ResourceManagementModeEnum
	SharingScope   ResourceSharingScopeEnum
	SystemManaged  bool
	ArchivedAt     sql.NullTime
}

func (r resource) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceData,
) (ResourceEntity, error) {
	entity := ResourceEntity{
		ID:             uuid.New(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Name:           data.Name,
		Category:       data.Category,
		Kind:           data.Kind,
		DatabaseName:   data.DatabaseName,
		ManagementMode: data.ManagementMode,
		SharingScope:   data.SharingScope,
		ArchivedAt:     data.ArchivedAt,
		SystemManaged:  data.SystemManaged,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := r.ensureActiveNameAvailable(ctx, db, entity.Name, nil); err != nil {
		return ResourceEntity{}, err
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceEntity{}, err
	}

	return entity, nil
}

type UpdateResourceData struct {
	ID             uuid.UUID
	UpdatedAt      time.Time
	Name           string
	Category       string
	Kind           string
	DatabaseName   string
	ManagementMode ResourceManagementModeEnum
	SharingScope   ResourceSharingScopeEnum
	SystemManaged  bool
	ArchivedAt     sql.NullTime
}

func (r resource) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateResourceData,
) (ResourceEntity, error) {
	entity := ResourceEntity{
		ID:             data.ID,
		UpdatedAt:      time.Now(),
		Name:           data.Name,
		Category:       data.Category,
		Kind:           data.Kind,
		DatabaseName:   data.DatabaseName,
		ManagementMode: data.ManagementMode,
		SharingScope:   data.SharingScope,
		ArchivedAt:     data.ArchivedAt,
		SystemManaged:  data.SystemManaged,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := r.ensureActiveNameAvailable(ctx, db, entity.Name, &entity.ID); err != nil {
		return ResourceEntity{}, err
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("category").
		Column("kind").
		Column("database_name").
		Column("management_mode").
		Column("sharing_scope").
		Column("system_managed").
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
		ID:             uuid.New(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Name:           data.Name,
		Category:       data.Category,
		Kind:           data.Kind,
		DatabaseName:   data.DatabaseName,
		ManagementMode: data.ManagementMode,
		SharingScope:   data.SharingScope,
		ArchivedAt:     data.ArchivedAt,
		SystemManaged:  data.SystemManaged,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := r.ensureActiveNameAvailable(ctx, db, entity.Name, &entity.ID); err != nil {
		return ResourceEntity{}, err
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("category = excluded.category").
		Set("kind = excluded.kind").
		Set("database_name = excluded.database_name").
		Set("management_mode = excluded.management_mode").
		Set("sharing_scope = excluded.sharing_scope").
		Set("system_managed = excluded.system_managed").
		Set("archived_at = excluded.archived_at").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceEntity{}, err
	}

	return entity, nil
}

func (r resource) ensureActiveNameAvailable(ctx context.Context, db storage.Executor, name string, exceptID *uuid.UUID) error {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "resource-name:"+normalizedName); err != nil {
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
		return errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "name", Code: "taken", Message: "an active Resource already uses this name"}})
	}
	return nil
}
