package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

var resourceContainerNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

type ResourceInstallationPortMapping struct {
	HostPort      int32  `json:"hostPort"`
	ContainerPort int32  `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

type ResourceInstallationEntity struct {
	bun.BaseModel        `bun:"table:resource_installations,alias:resource_installations"`
	ID                   uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt            time.Time       `bun:"created_at"`
	UpdatedAt            time.Time       `bun:"updated_at"`
	ImageReference       string          `bun:"image_reference"`
	ImageDigest          sql.NullString  `bun:"image_digest"`
	ContainerName        string          `bun:"container_name"`
	RestartPolicy        string          `bun:"restart_policy"`
	Configuration        json.RawMessage `bun:"configuration,type:jsonb"`
	ArchivedAt           sql.NullTime    `bun:"archived_at"`
	ResourceID           uuid.UUID       `bun:"resource_id,type:uuid"`
	ServerID             uuid.UUID       `bun:"server_id,type:uuid"`
	RegistryCredentialID *uuid.UUID      `bun:"registry_credential_id,type:uuid"`
}

func (e *ResourceInstallationEntity) Validate() error {
	e.ImageReference = strings.TrimSpace(e.ImageReference)
	e.ImageDigest.String = strings.TrimSpace(e.ImageDigest.String)
	e.ImageDigest.Valid = e.ImageDigest.String != ""
	e.ContainerName = strings.TrimSpace(e.ContainerName)
	e.RestartPolicy = strings.ToLower(strings.TrimSpace(e.RestartPolicy))
	builder := validation.NewBuilder()
	builder.Required("imageReference", e.ImageReference)
	if strings.ContainsAny(e.ImageReference, " \t\r\n") {
		builder.Add("imageReference", "format", "image reference must not contain whitespace")
	}
	builder.Required("containerName", e.ContainerName)
	if e.ContainerName != "" && !resourceContainerNamePattern.MatchString(e.ContainerName) {
		builder.Add("containerName", "format", "container name contains unsupported characters")
	}
	if !slices.Contains([]string{"no", "always", "on-failure", "unless-stopped"}, e.RestartPolicy) {
		builder.Add("restartPolicy", "unsupported", "restart policy is not supported")
	}
	if len(e.Configuration) == 0 || !json.Valid(e.Configuration) {
		builder.Add("configuration", "invalid", "configuration must be valid JSON")
	} else if settingsContainSecret(e.Configuration) {
		builder.Add("configuration", "secret", "configuration must not contain raw credentials")
	}
	if e.ResourceID == uuid.Nil {
		builder.Add("resourceId", "required", "resource is required")
	}
	if e.ServerID == uuid.Nil {
		builder.Add("serverId", "required", "server is required")
	}
	return builder.Err()
}

func (ri resourceInstallation) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ResourceInstallationEntity, error) {
	var entity ResourceInstallationEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceInstallationEntity{}, err
	}

	return entity, nil
}

type CreateResourceInstallationData struct {
	ImageReference       string
	ImageDigest          sql.NullString
	ContainerName        string
	RestartPolicy        string
	Configuration        json.RawMessage
	ArchivedAt           sql.NullTime
	ResourceID           uuid.UUID
	ServerID             uuid.UUID
	RegistryCredentialID *uuid.UUID
}

func (ri resourceInstallation) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceInstallationData,
) (ResourceInstallationEntity, error) {
	entity := ResourceInstallationEntity{
		ID:                   uuid.New(),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		ImageReference:       data.ImageReference,
		ImageDigest:          data.ImageDigest,
		ContainerName:        data.ContainerName,
		RestartPolicy:        data.RestartPolicy,
		Configuration:        data.Configuration,
		ArchivedAt:           data.ArchivedAt,
		ResourceID:           data.ResourceID,
		ServerID:             data.ServerID,
		RegistryCredentialID: data.RegistryCredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceInstallationEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "resource-installation:"+entity.ServerID.String()+":"+strings.ToLower(entity.ContainerName), entity.ID, db.NewSelect().Model((*ResourceInstallationEntity)(nil)).Where("server_id = ?", entity.ServerID).Where("lower(container_name) = ?", strings.ToLower(entity.ContainerName)), "containerName", "an active installation already uses this container name on the Server"); err != nil {
		return ResourceInstallationEntity{}, err
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceInstallationEntity{}, err
	}

	return entity, nil
}

type UpdateResourceInstallationData struct {
	ID                   uuid.UUID
	UpdatedAt            time.Time
	ImageReference       string
	ImageDigest          sql.NullString
	ContainerName        string
	RestartPolicy        string
	Configuration        json.RawMessage
	ArchivedAt           sql.NullTime
	ResourceID           uuid.UUID
	ServerID             uuid.UUID
	RegistryCredentialID *uuid.UUID
}

func (ri resourceInstallation) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateResourceInstallationData,
) (ResourceInstallationEntity, error) {
	entity := ResourceInstallationEntity{
		ID:                   data.ID,
		UpdatedAt:            time.Now(),
		ImageReference:       data.ImageReference,
		ImageDigest:          data.ImageDigest,
		ContainerName:        data.ContainerName,
		RestartPolicy:        data.RestartPolicy,
		Configuration:        data.Configuration,
		ArchivedAt:           data.ArchivedAt,
		ResourceID:           data.ResourceID,
		ServerID:             data.ServerID,
		RegistryCredentialID: data.RegistryCredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceInstallationEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "resource-installation:"+entity.ServerID.String()+":"+strings.ToLower(entity.ContainerName), entity.ID, db.NewSelect().Model((*ResourceInstallationEntity)(nil)).Where("server_id = ?", entity.ServerID).Where("lower(container_name) = ?", strings.ToLower(entity.ContainerName)), "containerName", "an active installation already uses this container name on the Server"); err != nil {
		return ResourceInstallationEntity{}, err
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("image_reference").
		Column("image_digest").
		Column("container_name").
		Column("restart_policy").
		Column("configuration").
		Column("archived_at").
		Column("resource_id").
		Column("server_id").
		Column("registry_credential_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceInstallationEntity{}, err
	}

	return entity, nil
}

func (ri resourceInstallation) Destroy(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) error {
	_, err := db.NewDelete().
		Model((*ResourceInstallationEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (ri resourceInstallation) All(
	ctx context.Context,
	db storage.Executor,
) ([]ResourceInstallationEntity, error) {
	var entities []ResourceInstallationEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceInstallations struct {
	ResourceInstallations []ResourceInstallationEntity
	TotalCount            int64
	Page                  int64
	PageSize              int64
	TotalPages            int64
}

func (ri resourceInstallation) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedResourceInstallations, error) {
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
		Model(&ResourceInstallationEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceInstallations{}, err
	}

	entities := make([]ResourceInstallationEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceInstallations{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceInstallations{
		ResourceInstallations: entities,
		TotalCount:            int64(totalCount),
		Page:                  page,
		PageSize:              pageSize,
		TotalPages:            totalPages,
	}, nil
}

func (ri resourceInstallation) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceInstallationData,
) (ResourceInstallationEntity, error) {
	entity := ResourceInstallationEntity{
		ID:                   uuid.New(),
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		ImageReference:       data.ImageReference,
		ImageDigest:          data.ImageDigest,
		ContainerName:        data.ContainerName,
		RestartPolicy:        data.RestartPolicy,
		Configuration:        data.Configuration,
		ArchivedAt:           data.ArchivedAt,
		ResourceID:           data.ResourceID,
		ServerID:             data.ServerID,
		RegistryCredentialID: data.RegistryCredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceInstallationEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "resource-installation:"+entity.ServerID.String()+":"+strings.ToLower(entity.ContainerName), entity.ID, db.NewSelect().Model((*ResourceInstallationEntity)(nil)).Where("server_id = ?", entity.ServerID).Where("lower(container_name) = ?", strings.ToLower(entity.ContainerName)), "containerName", "an active installation already uses this container name on the Server"); err != nil {
		return ResourceInstallationEntity{}, err
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("image_reference = excluded.image_reference").
		Set("image_digest = excluded.image_digest").
		Set("container_name = excluded.container_name").
		Set("restart_policy = excluded.restart_policy").
		Set("configuration = excluded.configuration").
		Set("archived_at = excluded.archived_at").
		Set("resource_id = excluded.resource_id").
		Set("server_id = excluded.server_id").
		Set("registry_credential_id = excluded.registry_credential_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceInstallationEntity{}, err
	}

	return entity, nil
}
