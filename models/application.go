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

const SystemApplicationSlug = "deploycrate-ce"

var ErrSystemApplicationImmutable = errors.New(
	"the DeployCrate CE system application cannot be modified",
)

type ApplicationEntity struct {
	bun.BaseModel `bun:"table:applications,alias:applications"`
	ID            uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time    `bun:"created_at"`
	UpdatedAt     time.Time    `bun:"updated_at"`
	Name          string       `bun:"name"`
	Slug          string       `bun:"slug"`
	ArchivedAt    sql.NullTime `bun:"archived_at"`
}

func (e *ApplicationEntity) Validate() error {
	return nil
}

func (e ApplicationEntity) IsSystem() bool {
	return e.Slug == SystemApplicationSlug
}

func (a application) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ApplicationEntity, error) {
	var entity ApplicationEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Where("slug <> ?", SystemApplicationSlug).
		Scan(ctx); err != nil {
		return ApplicationEntity{}, err
	}

	return entity, nil
}

func (a application) FindIncludingSystem(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ApplicationEntity, error) {
	return a.findIncludingSystem(ctx, db, id)
}

func (a application) findIncludingSystem(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ApplicationEntity, error) {
	var entity ApplicationEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ApplicationEntity{}, err
	}

	return entity, nil
}

func (a application) FindSystem(
	ctx context.Context,
	db storage.Executor,
) (ApplicationEntity, error) {
	var entity ApplicationEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("slug = ?", SystemApplicationSlug).
		Limit(1).
		Scan(ctx); err != nil {
		return ApplicationEntity{}, err
	}

	return entity, nil
}

type CreateApplicationData struct {
	Name       string
	Slug       string
	ArchivedAt sql.NullTime
}

func (a application) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateApplicationData,
) (ApplicationEntity, error) {
	entity := ApplicationEntity{
		ID:         uuid.New(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Name:       data.Name,
		Slug:       data.Slug,
		ArchivedAt: data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ApplicationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ApplicationEntity{}, err
	}

	return entity, nil
}

type UpdateApplicationData struct {
	ID         uuid.UUID
	UpdatedAt  time.Time
	Name       string
	Slug       string
	ArchivedAt sql.NullTime
}

func (a application) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateApplicationData,
) (ApplicationEntity, error) {
	existing, err := a.findIncludingSystem(ctx, db, data.ID)
	if err != nil {
		return ApplicationEntity{}, err
	}
	if existing.IsSystem() || data.Slug == SystemApplicationSlug {
		return ApplicationEntity{}, ErrSystemApplicationImmutable
	}

	entity := ApplicationEntity{
		ID:         data.ID,
		UpdatedAt:  time.Now(),
		Name:       data.Name,
		Slug:       data.Slug,
		ArchivedAt: data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ApplicationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("slug").
		Column("archived_at").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ApplicationEntity{}, err
	}

	return entity, nil
}

func (a application) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	existing, err := a.findIncludingSystem(ctx, db, id)
	if err != nil {
		return err
	}
	if existing.IsSystem() {
		return ErrSystemApplicationImmutable
	}

	if err := deleteReleaseCommandExecutionsForEnvironmentTargets(
		ctx,
		db,
		`SELECT target.id
		 FROM environment_targets AS target
		 JOIN environments AS environment ON environment.id = target.environment_id
		 WHERE environment.application_id = ?`,
		id,
	); err != nil {
		return err
	}

	_, err = db.NewDelete().
		Model((*ApplicationEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (a application) All(ctx context.Context, db storage.Executor) ([]ApplicationEntity, error) {
	var entities []ApplicationEntity
	if err := db.NewSelect().
		Model(&entities).
		Where("slug <> ?", SystemApplicationSlug).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedApplications struct {
	Applications []ApplicationEntity
	TotalCount   int64
	Page         int64
	PageSize     int64
	TotalPages   int64
}

func (a application) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedApplications, error) {
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
		Model(&ApplicationEntity{}).
		Where("slug <> ?", SystemApplicationSlug).
		Count(ctx)
	if err != nil {
		return PaginatedApplications{}, err
	}

	entities := make([]ApplicationEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Where("slug <> ?", SystemApplicationSlug).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedApplications{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedApplications{
		Applications: entities,
		TotalCount:   int64(totalCount),
		Page:         page,
		PageSize:     pageSize,
		TotalPages:   totalPages,
	}, nil
}

func (a application) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateApplicationData,
) (ApplicationEntity, error) {
	entity := ApplicationEntity{
		ID:         uuid.New(),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Name:       data.Name,
		Slug:       data.Slug,
		ArchivedAt: data.ArchivedAt,
	}

	if err := validation.Validate(&entity); err != nil {
		return ApplicationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("slug = excluded.slug").
		Set("archived_at = excluded.archived_at").
		Returning("*").
		Scan(ctx); err != nil {
		return ApplicationEntity{}, err
	}

	return entity, nil
}

type SystemApplicationState struct {
	ApplicationID        uuid.UUID `bun:"application_id"`
	EnvironmentID        uuid.UUID `bun:"environment_id"`
	EnvironmentTargetID  uuid.UUID `bun:"environment_target_id"`
	CaddyRouteID         uuid.UUID `bun:"caddy_route_id"`
	CaddyRouteExternalID string    `bun:"caddy_route_external_id"`
	ActiveInstanceID     uuid.UUID `bun:"active_instance_id"`
	ActiveInstanceSlot   string    `bun:"active_instance_slot"`
	ActiveBackendID      int32     `bun:"active_backend_id"`
	ActiveReleaseID      uuid.UUID `bun:"active_release_id"`
}

func (a application) FindSystemState(
	ctx context.Context,
	db storage.Executor,
) (SystemApplicationState, error) {
	var state SystemApplicationState
	if err := db.NewSelect().
		TableExpr("applications AS application").
		ColumnExpr("application.id AS application_id").
		ColumnExpr("environment.id AS environment_id").
		ColumnExpr("target.id AS environment_target_id").
		ColumnExpr("route.id AS caddy_route_id").
		ColumnExpr("route.external_id AS caddy_route_external_id").
		ColumnExpr("instance.id AS active_instance_id").
		ColumnExpr("instance.slot AS active_instance_slot").
		ColumnExpr("backend.id AS active_backend_id").
		ColumnExpr("instance.release_id AS active_release_id").
		Join("JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").
		Join("JOIN environment_targets AS target ON target.environment_id = environment.id AND target.detached_at IS NULL").
		Join("JOIN caddy_routes AS route ON route.environment_target_id = target.id AND route.removed_at IS NULL").
		Join("JOIN caddy_route_backends AS backend ON backend.caddy_route_id = route.id AND backend.removed_at IS NULL AND backend.weight = 100").
		Join("JOIN instances AS instance ON instance.id = backend.instance_id AND instance.removed_at IS NULL").
		Where("application.slug = ?", SystemApplicationSlug).
		OrderExpr("route.created_at DESC").
		Limit(1).
		Scan(ctx, &state); err != nil {
		return SystemApplicationState{}, err
	}
	return state, nil
}

type MetricRollupIdentities struct {
	Server       string `bun:"server"`
	Application  string `bun:"application"`
	Environment  string `bun:"environment"`
	Release      string `bun:"release"`
	Deployment   string `bun:"deployment"`
	Target       string `bun:"target"`
	Instance     string `bun:"instance"`
	Resource     string `bun:"resource"`
	Installation string `bun:"installation"`
}

func (a application) FindMetricDatabaseInstallationIdentities(
	ctx context.Context,
	db storage.Executor,
	installationID uuid.UUID,
) (MetricRollupIdentities, error) {
	var identities MetricRollupIdentities
	err := db.NewSelect().
		TableExpr("resource_installations AS installation").
		ColumnExpr("installation.server_id::text AS server").
		ColumnExpr("installation.id::text AS installation").
		ColumnExpr("installation.resource_id::text AS resource").
		Where("installation.id = ?", installationID).
		Where("installation.archived_at IS NULL").
		Limit(1).
		Scan(ctx, &identities)
	return identities, err
}

func (a application) FindMetricRollupIdentities(
	ctx context.Context,
	db storage.Executor,
) (MetricRollupIdentities, error) {
	var identities MetricRollupIdentities
	if err := db.NewSelect().
		TableExpr("applications AS application").
		ColumnExpr("target.server_id::text AS server").
		ColumnExpr("application.id::text AS application").
		ColumnExpr("environment.id::text AS environment").
		ColumnExpr("instance.release_id::text AS release").
		ColumnExpr("deployment.id::text AS deployment").
		ColumnExpr("target.id::text AS target").
		ColumnExpr("instance.id::text AS instance").
		Join("JOIN environments AS environment ON environment.application_id = application.id").
		Join("JOIN environment_targets AS target ON target.environment_id = environment.id").
		Join("JOIN instances AS instance ON instance.environment_target_id = target.id AND instance.state = 'serving'").
		Join("JOIN deployments AS deployment ON deployment.id = instance.deployment_id").
		Where("application.slug = ?", SystemApplicationSlug).
		OrderExpr("instance.observed_at DESC").
		Limit(1).
		Scan(ctx, &identities); err != nil {
		return MetricRollupIdentities{}, err
	}
	return identities, nil
}

func (a application) FindMetricWorkloadIdentities(
	ctx context.Context,
	db storage.Executor,
	instanceID uuid.UUID,
) (MetricRollupIdentities, error) {
	var identities MetricRollupIdentities
	err := db.NewSelect().
		TableExpr("instances AS instance").
		ColumnExpr("target.server_id::text AS server").
		ColumnExpr("application.id::text AS application").
		ColumnExpr("environment.id::text AS environment").
		ColumnExpr("instance.release_id::text AS release").
		ColumnExpr("instance.deployment_id::text AS deployment").
		ColumnExpr("target.id::text AS target").
		ColumnExpr("instance.id::text AS instance").
		Join("JOIN environment_targets AS target ON target.id = instance.environment_target_id").
		Join("JOIN environments AS environment ON environment.id = target.environment_id").
		Join("JOIN applications AS application ON application.id = environment.application_id").
		Where("instance.id = ?", instanceID).
		Limit(1).
		Scan(ctx, &identities)
	return identities, err
}

func (a application) FindMetricResourceIdentities(
	ctx context.Context,
	db storage.Executor,
	installationID uuid.UUID,
) (MetricRollupIdentities, error) {
	var identities MetricRollupIdentities
	err := db.NewSelect().
		TableExpr("resource_installations AS installation").
		ColumnExpr("installation.server_id::text AS server").
		ColumnExpr("installation.resource_id::text AS resource").
		ColumnExpr("installation.id::text AS installation").
		Join("JOIN resources AS resource ON resource.id = installation.resource_id").
		Where("installation.id = ?", installationID).
		Limit(1).
		Scan(ctx, &identities)
	return identities, err
}
