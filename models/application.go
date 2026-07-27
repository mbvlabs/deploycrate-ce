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

type SystemOverview struct {
	ApplicationName          string    `json:"applicationName"     bun:"application_name"`
	ApplicationSlug          string    `json:"applicationSlug"     bun:"application_slug"`
	EnvironmentName          string    `json:"environmentName"     bun:"environment_name"`
	EnvironmentKind          string    `json:"environmentKind"     bun:"environment_kind"`
	ServerName               string    `json:"serverName"          bun:"server_name"`
	ServerAddress            string    `json:"serverAddress"       bun:"server_address"`
	ServerStatus             string    `json:"serverStatus"        bun:"server_status"`
	OperatingSystem          string    `json:"operatingSystem"     bun:"operating_system"`
	Distribution             string    `json:"distribution"        bun:"distribution"`
	DistributionVersion      string    `json:"distributionVersion" bun:"distribution_version"`
	Architecture             string    `json:"architecture"        bun:"architecture"`
	NetworkName              string    `json:"networkName"         bun:"network_name"`
	NetworkDriver            string    `json:"networkDriver"       bun:"network_driver"`
	NetworkState             string    `json:"networkState"        bun:"network_state"`
	DatabaseID               string    `json:"databaseId"          bun:"database_id"`
	DatabaseName             string    `json:"databaseName"        bun:"database_name"`
	DatabaseCategory         string    `json:"databaseCategory"    bun:"database_category"`
	DatabaseKind             string    `json:"databaseKind"        bun:"database_kind"`
	DatabaseSharingScope     string    `json:"databaseSharingScope" bun:"database_sharing_scope"`
	DatabaseBindingAlias     string    `json:"databaseBindingAlias" bun:"database_binding_alias"`
	DatabaseCredentialSource string    `json:"databaseCredentialSource" bun:"database_credential_source"`
	DatabaseHasCredential    bool      `json:"databaseHasCredential" bun:"database_has_credential"`
	DatabaseEndpointName     string    `json:"databaseEndpointName" bun:"database_endpoint_name"`
	DatabaseEndpointRole     string    `json:"databaseEndpointRole" bun:"database_endpoint_role"`
	DatabaseAddress          string    `json:"databaseAddress"     bun:"database_address"`
	DatabasePort             int32     `json:"databasePort"        bun:"database_port"`
	DatabaseProtocol         string    `json:"databaseProtocol"    bun:"database_protocol"`
	DatabaseTLSMode          string    `json:"databaseTlsMode"     bun:"database_tls_mode"`
	DatabaseExternal         bool      `json:"databaseExternal"    bun:"database_external"`
	DatabaseHasInstallation  bool      `json:"databaseHasInstallation" bun:"database_has_installation"`
	DatabaseImageReference   string    `json:"databaseImageReference" bun:"database_image_reference"`
	DatabaseContainerName    string    `json:"databaseContainerName" bun:"database_container_name"`
	DatabaseRestartPolicy    string    `json:"databaseRestartPolicy" bun:"database_restart_policy"`
	DatabaseVolume           string    `json:"databaseVolume"      bun:"database_volume"`
	DatabaseBind             string    `json:"databaseBind"        bun:"database_bind"`
	ReleaseVersion           string    `json:"releaseVersion"      bun:"release_version"`
	ArtifactReference        string    `json:"artifactReference"   bun:"artifact_reference"`
	DeploymentStatus         string    `json:"deploymentStatus"    bun:"deployment_status"`
	DeploymentStep           string    `json:"deploymentStep"      bun:"deployment_step"`
	ActiveSlot               string    `json:"activeSlot"          bun:"active_slot"`
	ActiveService            string    `json:"activeService"       bun:"active_service"`
	ActiveState              string    `json:"activeState"         bun:"active_state"`
	ActivePort               int32     `json:"activePort"          bun:"active_port"`
	Domain                   string    `json:"domain"              bun:"domain"`
	RouteExternalID          string    `json:"routeExternalId"     bun:"route_external_id"`
	RouteState               string    `json:"routeState"          bun:"route_state"`
	ObservedAt               time.Time `json:"observedAt"          bun:"observed_at"`
}

func (a application) FindSystemOverview(
	ctx context.Context,
	db storage.Executor,
) (SystemOverview, error) {
	var overview SystemOverview
	if err := db.NewSelect().
		TableExpr("applications AS application").
		ColumnExpr("application.name AS application_name").
		ColumnExpr("application.slug AS application_slug").
		ColumnExpr("environment.name AS environment_name").
		ColumnExpr("environment.kind AS environment_kind").
		ColumnExpr("server.name AS server_name").
		ColumnExpr("server.address AS server_address").
		ColumnExpr("COALESCE(server_status.state, 'unknown') AS server_status").
		ColumnExpr("COALESCE(server.operating_system, '') AS operating_system").
		ColumnExpr("COALESCE(server.distribution, '') AS distribution").
		ColumnExpr("COALESCE(server.distribution_version, '') AS distribution_version").
		ColumnExpr("COALESCE(server.architecture, '') AS architecture").
		ColumnExpr("COALESCE(network.name, '') AS network_name").
		ColumnExpr("COALESCE(server_network.driver, '') AS network_driver").
		ColumnExpr("COALESCE(server_network.state, '') AS network_state").
		ColumnExpr("COALESCE(resource.id::text, '') AS database_id").
		ColumnExpr("COALESCE(resource.name, '') AS database_name").
		ColumnExpr("COALESCE(resource.category, '') AS database_category").
		ColumnExpr("COALESCE(resource.kind, '') AS database_kind").
		ColumnExpr("COALESCE(resource.sharing_scope, '') AS database_sharing_scope").
		ColumnExpr("COALESCE(environment_resource.alias, '') AS database_binding_alias").
		ColumnExpr("COALESCE(environment_resource.configuration ->> 'credential_source', '') AS database_credential_source").
		ColumnExpr("environment_resource.resource_credential_id IS NOT NULL AS database_has_credential").
		ColumnExpr("COALESCE(endpoint.name, '') AS database_endpoint_name").
		ColumnExpr("COALESCE(endpoint.role, '') AS database_endpoint_role").
		ColumnExpr("COALESCE(endpoint.address, '') AS database_address").
		ColumnExpr("COALESCE(endpoint.port, 0) AS database_port").
		ColumnExpr("COALESCE(endpoint.protocol, '') AS database_protocol").
		ColumnExpr("COALESCE(endpoint.tls_mode, '') AS database_tls_mode").
		ColumnExpr("COALESCE((endpoint.settings ->> 'external')::boolean, FALSE) AS database_external").
		ColumnExpr("installation.id IS NOT NULL AS database_has_installation").
		ColumnExpr("COALESCE(installation.image_reference, '') AS database_image_reference").
		ColumnExpr("COALESCE(installation.container_name, '') AS database_container_name").
		ColumnExpr("COALESCE(installation.restart_policy, '') AS database_restart_policy").
		ColumnExpr("COALESCE(installation.configuration ->> 'volume', '') AS database_volume").
		ColumnExpr("COALESCE(installation.configuration ->> 'bind', '') AS database_bind").
		ColumnExpr("COALESCE(release.version, '') AS release_version").
		ColumnExpr("release.artifact_reference AS artifact_reference").
		ColumnExpr("deployment.status AS deployment_status").
		ColumnExpr("COALESCE(deployment.current_step, '') AS deployment_step").
		ColumnExpr("instance.slot AS active_slot").
		ColumnExpr("instance.external_id AS active_service").
		ColumnExpr("instance.state AS active_state").
		ColumnExpr("COALESCE((instance.ports ->> 'http')::integer, 0) AS active_port").
		ColumnExpr("domain.hostname AS domain").
		ColumnExpr("route.external_id AS route_external_id").
		ColumnExpr("route.state AS route_state").
		ColumnExpr("instance.observed_at AS observed_at").
		Join("JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").
		Join("JOIN environment_targets AS target ON target.environment_id = environment.id AND target.detached_at IS NULL").
		Join("JOIN servers AS server ON server.id = target.server_id AND server.archived_at IS NULL").
		Join("LEFT JOIN LATERAL (SELECT state FROM server_statuses WHERE server_id = server.id ORDER BY observed_at DESC LIMIT 1) AS server_status ON TRUE").
		Join("JOIN environment_domains AS domain ON domain.environment_id = environment.id AND domain.is_primary = TRUE AND domain.archived_at IS NULL").
		Join("JOIN caddy_routes AS route ON route.environment_target_id = target.id AND route.environment_domain_id = domain.id AND route.removed_at IS NULL").
		Join("JOIN caddy_route_backends AS backend ON backend.caddy_route_id = route.id AND backend.removed_at IS NULL AND backend.weight = 100").
		Join("JOIN instances AS instance ON instance.id = backend.instance_id AND instance.removed_at IS NULL").
		Join("JOIN releases AS release ON release.id = instance.release_id").
		Join("JOIN deployments AS deployment ON deployment.id = instance.deployment_id").
		Join("LEFT JOIN environment_networks AS environment_network ON environment_network.environment_id = environment.id AND environment_network.role = 'primary' AND environment_network.removed_at IS NULL").
		Join("LEFT JOIN private_networks AS network ON network.id = environment_network.private_network_id AND network.archived_at IS NULL").
		Join("LEFT JOIN server_networks AS server_network ON server_network.server_id = server.id AND server_network.private_network_id = network.id AND server_network.removed_at IS NULL").
		Join("LEFT JOIN environment_resources AS environment_resource ON environment_resource.environment_id = environment.id AND environment_resource.alias = 'database' AND environment_resource.archived_at IS NULL").
		Join("LEFT JOIN resources AS resource ON resource.id = environment_resource.resource_id AND resource.category = 'database' AND resource.archived_at IS NULL").
		Join("LEFT JOIN resource_endpoints AS endpoint ON endpoint.id = environment_resource.resource_endpoint_id AND endpoint.archived_at IS NULL").
		Join("LEFT JOIN resource_installations AS installation ON installation.id = endpoint.resource_installation_id AND installation.archived_at IS NULL").
		Where("application.slug = ?", SystemApplicationSlug).
		OrderExpr("route.created_at DESC").
		Limit(1).
		Scan(ctx, &overview); err != nil {
		return SystemOverview{}, err
	}
	return overview, nil
}

type MetricRollupIdentities struct {
	Server      string `bun:"server"`
	Environment string `bun:"environment"`
	Deployment  string `bun:"deployment"`
	Target      string `bun:"target"`
}

func (a application) FindMetricRollupIdentities(
	ctx context.Context,
	db storage.Executor,
) (MetricRollupIdentities, error) {
	var identities MetricRollupIdentities
	if err := db.NewSelect().
		TableExpr("applications AS application").
		ColumnExpr("target.server_id::text AS server").
		ColumnExpr("environment.id::text AS environment").
		ColumnExpr("deployment.id::text AS deployment").
		ColumnExpr("target.id::text AS target").
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
