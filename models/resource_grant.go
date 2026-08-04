package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ResourceEnvironmentGrantEntity struct {
	bun.BaseModel `bun:"table:resource_environment_grants,alias:resource_environment_grant"`
	ID            uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time    `bun:"created_at"`
	UpdatedAt     time.Time    `bun:"updated_at"`
	ArchivedAt    sql.NullTime `bun:"archived_at"`
	ResourceID    uuid.UUID    `bun:"resource_id,type:uuid"`
	EnvironmentID uuid.UUID    `bun:"environment_id,type:uuid"`
}

type ResourceApplicationGrantEntity struct {
	bun.BaseModel `bun:"table:resource_application_grants,alias:resource_application_grant"`
	ID            uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time    `bun:"created_at"`
	UpdatedAt     time.Time    `bun:"updated_at"`
	ArchivedAt    sql.NullTime `bun:"archived_at"`
	ResourceID    uuid.UUID    `bun:"resource_id,type:uuid"`
	ApplicationID uuid.UUID    `bun:"application_id,type:uuid"`
}

func (resourceEnvironmentGrant) Create(ctx context.Context, db storage.Executor, resourceID, environmentID uuid.UUID) (ResourceEnvironmentGrantEntity, error) {
	if resourceID == uuid.Nil || environmentID == uuid.Nil {
		return ResourceEnvironmentGrantEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "grant", Code: "required", Message: "Resource and Environment are required"}})
	}
	var scope string
	if err := db.NewSelect().TableExpr("resources").Column("sharing_scope").Where("id = ?", resourceID).Where("archived_at IS NULL").Scan(ctx, &scope); err != nil {
		return ResourceEnvironmentGrantEntity{}, err
	}
	if scope != ResourceSharingEnvironment.String() {
		return ResourceEnvironmentGrantEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "resourceId", Code: "scope", Message: "Environment grants require Environment sharing scope"}})
	}
	now := time.Now().UTC()
	entity := ResourceEnvironmentGrantEntity{ID: uuid.New(), CreatedAt: now, UpdatedAt: now, ResourceID: resourceID, EnvironmentID: environmentID}
	if err := ensureActiveUnique(ctx, db, "resource-environment-grant:"+resourceID.String()+":"+environmentID.String(), entity.ID, db.NewSelect().Model((*ResourceEnvironmentGrantEntity)(nil)).Where("resource_id = ?", resourceID).Where("environment_id = ?", environmentID), "environmentId", "the Environment already has access to this Resource"); err != nil {
		return ResourceEnvironmentGrantEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceEnvironmentGrantEntity{}, err
	}
	return entity, nil
}

func (resourceApplicationGrant) Create(ctx context.Context, db storage.Executor, resourceID, applicationID uuid.UUID) (ResourceApplicationGrantEntity, error) {
	if resourceID == uuid.Nil || applicationID == uuid.Nil {
		return ResourceApplicationGrantEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "grant", Code: "required", Message: "Resource and Application are required"}})
	}
	var scope string
	if err := db.NewSelect().TableExpr("resources").Column("sharing_scope").Where("id = ?", resourceID).Where("archived_at IS NULL").Scan(ctx, &scope); err != nil {
		return ResourceApplicationGrantEntity{}, err
	}
	if scope != ResourceSharingApplication.String() {
		return ResourceApplicationGrantEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "resourceId", Code: "scope", Message: "Application grants require Application sharing scope"}})
	}
	now := time.Now().UTC()
	entity := ResourceApplicationGrantEntity{ID: uuid.New(), CreatedAt: now, UpdatedAt: now, ResourceID: resourceID, ApplicationID: applicationID}
	if err := ensureActiveUnique(ctx, db, "resource-application-grant:"+resourceID.String()+":"+applicationID.String(), entity.ID, db.NewSelect().Model((*ResourceApplicationGrantEntity)(nil)).Where("resource_id = ?", resourceID).Where("application_id = ?", applicationID), "applicationId", "the Application already has access to this Resource"); err != nil {
		return ResourceApplicationGrantEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceApplicationGrantEntity{}, err
	}
	return entity, nil
}

func ResourceSelectableByEnvironment(ctx context.Context, db storage.Executor, resourceID, environmentID uuid.UUID) (bool, error) {
	var allowed bool
	err := db.NewSelect().TableExpr("resources AS resource").ColumnExpr(`CASE
		WHEN resource.system_managed = TRUE THEN FALSE
		WHEN resource.sharing_scope = 'global' THEN TRUE
		WHEN resource.sharing_scope = 'environment' THEN EXISTS (
				SELECT 1 FROM resource_environment_grants access_grant
				WHERE access_grant.resource_id = resource.id AND access_grant.environment_id = ? AND access_grant.archived_at IS NULL)
			WHEN resource.sharing_scope = 'application' THEN EXISTS (
				SELECT 1 FROM resource_application_grants access_grant
				JOIN environments environment ON environment.application_id = access_grant.application_id
				WHERE access_grant.resource_id = resource.id AND environment.id = ?
				AND access_grant.archived_at IS NULL AND environment.archived_at IS NULL)
		ELSE FALSE END`, environmentID, environmentID).Where("resource.id = ?", resourceID).Where("resource.archived_at IS NULL").Scan(ctx, &allowed)
	return allowed, err
}
