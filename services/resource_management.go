package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

const resourceCredentialPurpose = "resource-credential/v1"

var ErrSystemResourceImmutable = errors.New("DeployCrate system Resources cannot be modified")

type ResourceManagement struct {
	db     storage.Pool
	config config.Config
}

func NewResourceManagement(db storage.Pool, cfg config.Config) *ResourceManagement {
	return &ResourceManagement{db: db, config: cfg}
}

type ResourceInput struct {
	Name           string
	Category       string
	Kind           string
	ManagementMode models.ResourceManagementModeEnum
	SharingScope   models.ResourceSharingScopeEnum
}

type ResourceConnectionInput struct {
	EnvironmentID        uuid.UUID
	Alias                string
	Configuration        json.RawMessage
	ResourceEndpointID   uuid.UUID
	ResourceCredentialID *uuid.UUID
}

type CreateResourceInput struct {
	Resource     ResourceInput
	Endpoint     *ResourceEndpointInput
	Credential   *ResourceCredentialInput
	Installation *ResourceInstallationInput
	Volume       *ResourceVolumeInput
	Mount        *ResourceMountInput
	HealthCheck  *ResourceHealthCheckInput
}

type ResourceEndpointInput struct {
	Name                   string
	Role                   string
	Address                string
	Port                   int32
	Protocol               string
	TLSMode                string
	Settings               json.RawMessage
	ResourceInstallationID *uuid.UUID
	PrivateNetworkID       *uuid.UUID
}

type ResourceCredentialInput struct {
	Name                   string
	Role                   string
	Username               string
	Metadata               json.RawMessage
	SecretValues           map[string]string
	ResourceInstallationID *uuid.UUID
}

type ResourceInstallationInput struct {
	ImageReference       string
	ImageDigest          string
	ContainerName        string
	RestartPolicy        string
	Configuration        json.RawMessage
	ServerID             uuid.UUID
	RegistryCredentialID *uuid.UUID
}

type ResourceVolumeInput struct {
	Name          string
	Driver        string
	Configuration json.RawMessage
	ServerID      uuid.UUID
}

type ResourceMountInput struct {
	MountPath              string
	ReadOnly               bool
	ResourceVolumeID       uuid.UUID
	ResourceInstallationID uuid.UUID
}

type ResourceHealthCheckInput struct {
	Name                   string
	Kind                   string
	Configuration          json.RawMessage
	IntervalSeconds        int32
	TimeoutSeconds         int32
	FailureThreshold       int32
	SuccessThreshold       int32
	Enabled                bool
	ResourceInstallationID uuid.UUID
	ResourceEndpointID     *uuid.UUID
	ResourceCredentialID   *uuid.UUID
}

func (service *ResourceManagement) List(ctx context.Context, filters models.ResourceListFilters) ([]models.ResourceListItem, error) {
	items := make([]models.ResourceListItem, 0)
	query := service.db.Executor().NewSelect().
		TableExpr("resources AS resource").
		ColumnExpr("resource.id, resource.name, resource.category, resource.kind, resource.management_mode, resource.sharing_scope").
		ColumnExpr("(SELECT count(*) FROM environment_resources AS connection WHERE connection.resource_id = resource.id AND connection.archived_at IS NULL) AS connection_count").
		ColumnExpr("(SELECT count(*) FROM resource_installations AS installation WHERE installation.resource_id = resource.id AND installation.archived_at IS NULL) AS installation_count").
		ColumnExpr("(SELECT count(*) FROM resource_endpoints AS endpoint WHERE endpoint.resource_id = resource.id AND endpoint.archived_at IS NULL) AS endpoint_count").
		ColumnExpr(`CASE
			WHEN EXISTS (
				SELECT 1 FROM resource_health_checks AS health_check
				JOIN resource_installations AS installation ON installation.id = health_check.resource_installation_id AND installation.archived_at IS NULL
				JOIN resource_health_check_statuses AS status ON status.health_check_id = health_check.id
				WHERE installation.resource_id = resource.id AND health_check.archived_at IS NULL AND status.state IN ('unhealthy', 'failed', 'critical')
			) THEN 'unhealthy'
			WHEN EXISTS (
				SELECT 1 FROM resource_health_checks AS health_check
				JOIN resource_installations AS installation ON installation.id = health_check.resource_installation_id AND installation.archived_at IS NULL
				JOIN resource_health_check_statuses AS status ON status.health_check_id = health_check.id
				WHERE installation.resource_id = resource.id AND health_check.archived_at IS NULL AND status.state IN ('healthy', 'passing')
			) THEN 'healthy'
			ELSE 'unknown'
		END AS health`).
		Where("resource.archived_at IS NULL").
		Where("resource.system_managed = FALSE").
		OrderExpr("resource.name ASC")
	if search := strings.TrimSpace(filters.Search); search != "" {
		query = query.Where("resource.name ILIKE ?", "%"+search+"%")
	}
	if filters.Kind != "" {
		query = query.Where("resource.kind = ?", filters.Kind)
	}
	if filters.Category != "" {
		query = query.Where("resource.category = ?", filters.Category)
	}
	if filters.ManagementMode != "" {
		query = query.Where("resource.management_mode = ?", filters.ManagementMode)
	}
	if filters.SharingScope != "" {
		query = query.Where("resource.sharing_scope = ?", filters.SharingScope)
	}
	return items, query.Scan(ctx, &items)
}

func (service *ResourceManagement) Options(ctx context.Context) (models.ResourceFormOptions, error) {
	options := models.ResourceFormOptions{
		Kinds:               models.ResourceKindCatalog(),
		Environments:        make([]models.ResourceEnvironmentOption, 0),
		Servers:             make([]models.ResourceServerOption, 0),
		PrivateNetworks:     make([]models.ResourceNetworkOption, 0),
		RegistryCredentials: make([]models.ResourceRegistryCredentialOption, 0),
	}
	queries := []func() error{
		func() error {
			return service.db.Executor().NewSelect().TableExpr("environments AS environment").
				ColumnExpr("environment.id, environment.name, environment.kind, application.name AS application_name").
				Join("JOIN applications AS application ON application.id = environment.application_id AND application.archived_at IS NULL").
				Where("environment.archived_at IS NULL").Where("application.slug <> ?", models.SystemApplicationSlug).
				OrderExpr("application.name, environment.name").Scan(ctx, &options.Environments)
		},
		func() error {
			return service.db.Executor().NewSelect().TableExpr("servers AS server").
				ColumnExpr("server.id, server.name, server.address").
				Where("server.archived_at IS NULL").OrderExpr("server.name").Scan(ctx, &options.Servers)
		},
		func() error {
			return service.db.Executor().NewSelect().TableExpr("private_networks AS network").
				ColumnExpr("network.id, network.name").Where("network.archived_at IS NULL").
				OrderExpr("network.name").Scan(ctx, &options.PrivateNetworks)
		},
		func() error {
			return service.db.Executor().NewSelect().TableExpr("credentials AS credential").
				ColumnExpr("credential.id, credential.name").Where("credential.archived_at IS NULL").
				OrderExpr("credential.name").Scan(ctx, &options.RegistryCredentials)
		},
	}
	for _, query := range queries {
		if err := query(); err != nil {
			return models.ResourceFormOptions{}, err
		}
	}
	type networkServer struct {
		NetworkID uuid.UUID `bun:"network_id"`
		ServerID  uuid.UUID `bun:"server_id"`
	}
	var access []networkServer
	if err := service.db.Executor().NewSelect().TableExpr("server_networks").
		ColumnExpr("private_network_id AS network_id, server_id").Where("removed_at IS NULL").Scan(ctx, &access); err != nil {
		return models.ResourceFormOptions{}, err
	}
	networkIndexes := make(map[uuid.UUID]int, len(options.PrivateNetworks))
	for index := range options.PrivateNetworks {
		networkIndexes[options.PrivateNetworks[index].ID] = index
		options.PrivateNetworks[index].ServerIDs = make([]uuid.UUID, 0)
	}
	for _, item := range access {
		if index, ok := networkIndexes[item.NetworkID]; ok {
			options.PrivateNetworks[index].ServerIDs = append(options.PrivateNetworks[index].ServerIDs, item.ServerID)
		}
	}
	return options, nil
}

func (service *ResourceManagement) Details(ctx context.Context, resourceID uuid.UUID) (models.ResourceDetails, error) {
	resource, err := service.loadResource(ctx, service.db.Executor(), resourceID, false)
	if err != nil {
		return models.ResourceDetails{}, err
	}
	if resource.SystemManaged {
		return models.ResourceDetails{}, models.ErrNotFound
	}
	detail := models.ResourceDetails{
		Resource: resource, Connections: make([]models.ResourceConnectionDetail, 0),
		Endpoints: make([]models.ResourceEndpointEntity, 0), Credentials: make([]models.ResourceCredentialEntity, 0),
		Installations: make([]models.ResourceInstallationDetail, 0), Volumes: make([]models.ResourceVolumeDetail, 0),
		Mounts: make([]models.ResourceMountDetail, 0), HealthChecks: make([]models.ResourceHealthCheckDetail, 0),
	}
	queries := []func() error{
		func() error {
			return service.db.Executor().NewSelect().TableExpr("environment_resources AS connection").
				ColumnExpr("connection.*").
				ColumnExpr("environment.name AS environment_name, environment.kind AS environment_kind, environment.archived_at IS NOT NULL AS environment_archived").
				ColumnExpr("application.name AS application_name, application.slug AS application_slug, application.archived_at IS NOT NULL AS application_archived").
				ColumnExpr("endpoint.name AS endpoint_name, COALESCE(credential.name, '') AS credential_name").
				Join("JOIN environments AS environment ON environment.id = connection.environment_id").
				Join("JOIN applications AS application ON application.id = environment.application_id").
				Join("JOIN resource_endpoints AS endpoint ON endpoint.id = connection.resource_endpoint_id AND endpoint.archived_at IS NULL").
				Join("LEFT JOIN resource_credentials AS credential ON credential.id = connection.resource_credential_id AND credential.archived_at IS NULL").
				Where("connection.resource_id = ?", resourceID).Where("connection.archived_at IS NULL").
				OrderExpr("application.name, environment.name").Scan(ctx, &detail.Connections)
		},
		func() error {
			return service.db.Executor().NewSelect().Model(&detail.Endpoints).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").OrderExpr("name").Scan(ctx)
		},
		func() error {
			return service.db.Executor().NewSelect().Model(&detail.Credentials).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").OrderExpr("name").Scan(ctx)
		},
		func() error {
			return service.db.Executor().NewSelect().TableExpr("resource_installations AS installation").
				ColumnExpr("installation.*").ColumnExpr("server.name AS server_name, server.address AS server_address").
				ColumnExpr("COALESCE(status.state, '') AS state, COALESCE(status.service_state, '') AS service_state, COALESCE(status.health, '') AS health, COALESCE(status.health_reason, '') AS health_reason, status.observed_at").
				Join("JOIN servers AS server ON server.id = installation.server_id").
				Join("LEFT JOIN resource_installation_statuses AS status ON status.resource_installation_id = installation.id").
				Where("installation.resource_id = ?", resourceID).Where("installation.archived_at IS NULL").OrderExpr("installation.created_at").Scan(ctx, &detail.Installations)
		},
		func() error {
			return service.db.Executor().NewSelect().TableExpr("resource_volumes AS volume").ColumnExpr("volume.*, server.name AS server_name").
				Join("JOIN servers AS server ON server.id = volume.server_id").Where("volume.resource_id = ?", resourceID).
				Where("volume.archived_at IS NULL").OrderExpr("volume.name").Scan(ctx, &detail.Volumes)
		},
		func() error {
			return service.db.Executor().NewSelect().TableExpr("resource_volume_mounts AS mount").ColumnExpr("mount.*, volume.name AS volume_name, installation.container_name").
				Join("JOIN resource_volumes AS volume ON volume.id = mount.resource_volume_id").
				Join("JOIN resource_installations AS installation ON installation.id = mount.resource_installation_id").
				Where("volume.resource_id = ?", resourceID).Where("mount.archived_at IS NULL").OrderExpr("mount.mount_path").Scan(ctx, &detail.Mounts)
		},
		func() error {
			return service.db.Executor().NewSelect().TableExpr("resource_health_checks AS health_check").ColumnExpr("health_check.*").
				ColumnExpr("COALESCE(status.state, '') AS state, COALESCE(status.message, '') AS message, status.observed_at").
				Join("JOIN resource_installations AS installation ON installation.id = health_check.resource_installation_id").
				Join("LEFT JOIN resource_health_check_statuses AS status ON status.health_check_id = health_check.id").
				Where("installation.resource_id = ?", resourceID).Where("health_check.archived_at IS NULL").OrderExpr("health_check.name").Scan(ctx, &detail.HealthChecks)
		},
	}
	for _, query := range queries {
		if err := query(); err != nil {
			return models.ResourceDetails{}, err
		}
	}
	return detail, nil
}

func (service *ResourceManagement) CreateResource(ctx context.Context, input CreateResourceInput) (models.ResourceEntity, error) {
	if input.Resource.ManagementMode == models.ResourceManagementExternal && input.Endpoint == nil {
		return models.ResourceEntity{}, domainError("endpoint", "required", "external Resources require an active endpoint")
	}
	if input.Resource.ManagementMode == models.ResourceManagementExternal && (input.Installation != nil || input.Volume != nil || input.Mount != nil || input.HealthCheck != nil) {
		return models.ResourceEntity{}, domainError("managementMode", "topology", "external Resources cannot have managed placement topology")
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	defer tx.Rollback()
	resource, err := models.Resource.Create(ctx, tx, models.CreateResourceData{
		Name: input.Resource.Name, Category: input.Resource.Category, Kind: input.Resource.Kind,
		ManagementMode: input.Resource.ManagementMode, SharingScope: input.Resource.SharingScope,
		SystemManaged: false,
	})
	if err != nil {
		return models.ResourceEntity{}, mapResourceConflict(err)
	}
	var installation *models.ResourceInstallationEntity
	if input.Installation != nil {
		created, createErr := service.createInstallation(ctx, tx, resource, *input.Installation)
		if createErr != nil {
			return models.ResourceEntity{}, createErr
		}
		installation = &created
	}
	var volume *models.ResourceVolumeEntity
	if input.Volume != nil {
		created, createErr := service.createVolume(ctx, tx, resource, *input.Volume)
		if createErr != nil {
			return models.ResourceEntity{}, createErr
		}
		volume = &created
	}
	if input.Mount != nil {
		if installation == nil || volume == nil {
			return models.ResourceEntity{}, domainError("mount", "topology", "an initial mount requires both an installation and volume")
		}
		mountInput := *input.Mount
		mountInput.ResourceInstallationID = installation.ID
		mountInput.ResourceVolumeID = volume.ID
		if _, err := service.createMount(ctx, tx, resource, mountInput); err != nil {
			return models.ResourceEntity{}, err
		}
	}
	var endpoint *models.ResourceEndpointEntity
	if input.Endpoint != nil {
		endpointInput := *input.Endpoint
		if installation != nil {
			endpointInput.ResourceInstallationID = &installation.ID
		}
		created, createErr := service.createEndpoint(ctx, tx, resource, endpointInput)
		if createErr != nil {
			return models.ResourceEntity{}, createErr
		}
		endpoint = &created
	}
	var credential *models.ResourceCredentialEntity
	if input.Credential != nil {
		credentialInput := *input.Credential
		if installation != nil {
			credentialInput.ResourceInstallationID = &installation.ID
		}
		created, createErr := service.createCredential(ctx, tx, resource, credentialInput)
		if createErr != nil {
			return models.ResourceEntity{}, createErr
		}
		credential = &created
	}
	if input.HealthCheck != nil {
		if installation == nil {
			return models.ResourceEntity{}, domainError("healthCheck", "topology", "an initial health check requires an installation")
		}
		healthInput := *input.HealthCheck
		healthInput.ResourceInstallationID = installation.ID
		if endpoint != nil {
			healthInput.ResourceEndpointID = &endpoint.ID
		}
		if credential != nil {
			healthInput.ResourceCredentialID = &credential.ID
		}
		if _, err := service.createHealthCheck(ctx, tx, resource, healthInput); err != nil {
			return models.ResourceEntity{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceEntity{}, mapResourceConflict(err)
	}
	return resource, nil
}

func (service *ResourceManagement) UpdateResource(ctx context.Context, resourceID uuid.UUID, input ResourceInput) (models.ResourceEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	if err := service.validateResourceTransition(ctx, tx, resource, input); err != nil {
		return models.ResourceEntity{}, err
	}
	updated, err := models.Resource.Update(ctx, tx, models.UpdateResourceData{
		ID: resource.ID, Name: input.Name, Category: input.Category, Kind: input.Kind,
		ManagementMode: input.ManagementMode, SharingScope: input.SharingScope,
		SystemManaged: resource.SystemManaged, ArchivedAt: resource.ArchivedAt,
	})
	if err != nil {
		return models.ResourceEntity{}, mapResourceConflict(err)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) ArchiveResource(ctx context.Context, resourceID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	bindings, err := tx.NewSelect().TableExpr("environment_resources").Where("resource_id = ?", resourceID).Where("archived_at IS NULL").Count(ctx)
	if err != nil {
		return err
	}
	if bindings > 0 {
		return domainError("resource", "dependency", "archive active Environment bindings before archiving this Resource")
	}
	now := time.Now().UTC()
	statements := []struct {
		table string
		where string
	}{
		{"resource_volume_mounts", "resource_installation_id IN (SELECT id FROM resource_installations WHERE resource_id = ?)"},
		{"resource_health_checks", "resource_installation_id IN (SELECT id FROM resource_installations WHERE resource_id = ?)"},
		{"resource_endpoints", "resource_id = ?"},
		{"resource_credentials", "resource_id = ?"},
		{"resource_installations", "resource_id = ?"},
		{"resource_volumes", "resource_id = ?"},
		{"resources", "id = ?"},
	}
	for _, statement := range statements {
		if _, err := tx.NewUpdate().Table(statement.table).Set("archived_at = ?", now).Set("updated_at = ?", now).
			Where(statement.where, resourceID).Where("archived_at IS NULL").Exec(ctx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (service *ResourceManagement) loadResource(ctx context.Context, db storage.Executor, resourceID uuid.UUID, lock bool) (models.ResourceEntity, error) {
	var resource models.ResourceEntity
	query := db.NewSelect().TableExpr("resources AS resource").ColumnExpr("resource.*").
		Where("resource.id = ?", resourceID).Where("resource.archived_at IS NULL")
	if lock {
		query = query.For("UPDATE OF resource")
	}
	if err := query.Scan(ctx, &resource); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.ResourceEntity{}, models.ErrNotFound
		}
		return models.ResourceEntity{}, err
	}
	if lock && resource.SystemManaged {
		return models.ResourceEntity{}, ErrSystemResourceImmutable
	}
	return resource, nil
}

func (service *ResourceManagement) validateResourceTransition(ctx context.Context, db storage.Executor, current models.ResourceEntity, input ResourceInput) error {
	if current.Kind != input.Kind {
		kindDependencies, err := db.NewSelect().TableExpr("resources AS resource").Where("resource.id = ?", current.ID).
			Where(`EXISTS (SELECT 1 FROM resource_endpoints WHERE resource_id = resource.id AND archived_at IS NULL)
				OR EXISTS (SELECT 1 FROM resource_credentials WHERE resource_id = resource.id AND archived_at IS NULL)
				OR EXISTS (SELECT 1 FROM resource_health_checks AS health_check JOIN resource_installations AS installation ON installation.id = health_check.resource_installation_id WHERE installation.resource_id = resource.id AND health_check.archived_at IS NULL)`).Count(ctx)
		if err != nil {
			return err
		}
		if kindDependencies > 0 {
			return domainError("kind", "topology", "archive active endpoints, credentials, and health checks before changing Resource kind")
		}
	}
	if current.ManagementMode != input.ManagementMode && input.ManagementMode == models.ResourceManagementExternal {
		managedTopology, err := db.NewSelect().TableExpr("resources AS resource").Where("resource.id = ?", current.ID).
			Where(`EXISTS (SELECT 1 FROM resource_installations WHERE resource_id = resource.id AND archived_at IS NULL)
				OR EXISTS (SELECT 1 FROM resource_volumes WHERE resource_id = resource.id AND archived_at IS NULL)
				OR EXISTS (SELECT 1 FROM resource_endpoints WHERE resource_id = resource.id AND resource_installation_id IS NOT NULL AND archived_at IS NULL)
				OR EXISTS (SELECT 1 FROM resource_credentials WHERE resource_id = resource.id AND resource_installation_id IS NOT NULL AND archived_at IS NULL)`).Count(ctx)
		if err != nil {
			return err
		}
		if managedTopology > 0 {
			return domainError("managementMode", "topology", "archive managed placement topology before changing this Resource to external")
		}
	}
	if input.ManagementMode == models.ResourceManagementExternal {
		endpoints, err := db.NewSelect().TableExpr("resource_endpoints").Where("resource_id = ?", current.ID).Where("archived_at IS NULL").Count(ctx)
		if err != nil {
			return err
		}
		if endpoints == 0 {
			return domainError("managementMode", "incomplete", "external Resources require an active endpoint")
		}
	}
	return nil
}

func domainError(field, code, message string) error {
	return errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: field, Code: code, Message: message}})
}

func mapResourceConflict(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	field := "resource"
	message := "an active record already uses this value"
	switch pgErr.ConstraintName {
	case "resources_active_owner_name":
		field, message = "name", "an active Resource with this name already exists"
	case "resource_endpoints_active_resource_name", "resource_credentials_active_resource_name", "resource_volumes_active_resource_name", "resource_health_checks_active_installation_name":
		field = "name"
	case "resource_installations_active_server_container_name":
		field = "containerName"
	case "resource_volume_mounts_active_installation_path":
		field = "mountPath"
	}
	return domainError(field, "unique", message)
}

func (service *ResourceManagement) ConnectEnvironment(ctx context.Context, resourceID uuid.UUID, input ResourceConnectionInput) (models.EnvironmentResourceEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.EnvironmentResourceEntity{}, err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return models.EnvironmentResourceEntity{}, err
	}
	if err := service.validateConnectionTopology(ctx, tx, resourceID, input); err != nil {
		return models.EnvironmentResourceEntity{}, err
	}
	connection, err := models.EnvironmentResource.Create(ctx, tx, models.CreateEnvironmentResourceData{
		Alias: input.Alias, Configuration: normalizedJSON(input.Configuration),
		EnvironmentID: input.EnvironmentID, ResourceID: resourceID,
		ResourceEndpointID: input.ResourceEndpointID, ResourceCredentialID: input.ResourceCredentialID,
	})
	if err != nil {
		return models.EnvironmentResourceEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.EnvironmentResourceEntity{}, err
	}
	return connection, nil
}

func (service *ResourceManagement) UpdateEnvironmentConnection(ctx context.Context, resourceID, connectionID uuid.UUID, input ResourceConnectionInput) (models.EnvironmentResourceEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.EnvironmentResourceEntity{}, err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return models.EnvironmentResourceEntity{}, err
	}
	var current models.EnvironmentResourceEntity
	err = tx.NewSelect().Model(&current).Where("id = ?", connectionID).Where("resource_id = ?", resourceID).
		Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.EnvironmentResourceEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.EnvironmentResourceEntity{}, err
	}
	if err := service.validateConnectionTopology(ctx, tx, resourceID, input); err != nil {
		return models.EnvironmentResourceEntity{}, err
	}
	connection, err := models.EnvironmentResource.Update(ctx, tx, models.UpdateEnvironmentResourceData{
		ID: current.ID, Alias: input.Alias, Configuration: normalizedJSON(input.Configuration),
		ArchivedAt: current.ArchivedAt, EnvironmentID: input.EnvironmentID, ResourceID: resourceID,
		ResourceEndpointID: input.ResourceEndpointID, ResourceCredentialID: input.ResourceCredentialID,
	})
	if err != nil {
		return models.EnvironmentResourceEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.EnvironmentResourceEntity{}, err
	}
	return connection, nil
}

func (service *ResourceManagement) DisconnectEnvironment(ctx context.Context, resourceID, connectionID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	var connection models.EnvironmentResourceEntity
	err = tx.NewSelect().Model(&connection).Where("id = ?", connectionID).Where("resource_id = ?", resourceID).
		Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	dependencies, err := tx.NewSelect().TableExpr("environment_resources AS connection").Where("connection.id = ?", connectionID).
		Where(`EXISTS (SELECT 1 FROM network_access_rules WHERE environment_resource_id = connection.id AND archived_at IS NULL)
			OR EXISTS (SELECT 1 FROM backup_policies WHERE environment_resource_id = connection.id AND archived_at IS NULL)
			OR EXISTS (SELECT 1 FROM backups WHERE environment_resource_id = connection.id)
			OR EXISTS (SELECT 1 FROM resource_restores WHERE source_environment_resource_id = connection.id OR target_environment_resource_id = connection.id)`).Count(ctx)
	if err != nil {
		return err
	}
	if dependencies > 0 {
		return domainError("connection", "dependency", "Connected Environment is required by an active network rule, backup, or restore")
	}
	if err := models.EnvironmentResource.Archive(ctx, tx, connection.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) validateConnectionTopology(ctx context.Context, db storage.Executor, resourceID uuid.UUID, input ResourceConnectionInput) error {
	environments, err := db.NewSelect().TableExpr("environments AS environment").
		Join("JOIN applications AS application ON application.id = environment.application_id AND application.archived_at IS NULL").
		Where("environment.id = ?", input.EnvironmentID).Where("environment.archived_at IS NULL").Count(ctx)
	if err != nil {
		return err
	}
	if err := requireChild(environments, "environmentId", "Environment is unavailable"); err != nil {
		return err
	}
	var endpoint struct {
		PrivateNetworkID *uuid.UUID `bun:"private_network_id"`
	}
	err = db.NewSelect().TableExpr("resource_endpoints").ColumnExpr("private_network_id").
		Where("id = ?", input.ResourceEndpointID).Where("resource_id = ?", resourceID).
		Where("archived_at IS NULL").Scan(ctx, &endpoint)
	if errors.Is(err, sql.ErrNoRows) {
		return domainError("resourceEndpointId", "unavailable", "endpoint must be active and belong to this Resource")
	}
	if err != nil {
		return err
	}
	if input.ResourceCredentialID != nil {
		credentials, err := db.NewSelect().TableExpr("resource_credentials").Where("id = ?", *input.ResourceCredentialID).
			Where("resource_id = ?", resourceID).Where("archived_at IS NULL").Count(ctx)
		if err != nil {
			return err
		}
		if err := requireChild(credentials, "resourceCredentialId", "credential must be active and belong to this Resource"); err != nil {
			return err
		}
	}
	if endpoint.PrivateNetworkID != nil {
		networks, err := db.NewSelect().TableExpr("private_networks").Where("id = ?", *endpoint.PrivateNetworkID).Where("archived_at IS NULL").Count(ctx)
		if err != nil {
			return err
		}
		if err := requireChild(networks, "resourceEndpointId", "endpoint private network is unavailable"); err != nil {
			return err
		}
		access, err := db.NewSelect().TableExpr("environment_networks").Where("environment_id = ?", input.EnvironmentID).
			Where("private_network_id = ?", *endpoint.PrivateNetworkID).Where("removed_at IS NULL").Count(ctx)
		if err != nil {
			return err
		}
		if err := requireChild(access, "environmentId", "Environment cannot reach the endpoint private network"); err != nil {
			return err
		}
	}
	return nil
}

func normalizedJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func (service *ResourceManagement) credentialPayload(input ResourceCredentialInput, definition models.ResourceKindDefinition) ([]byte, []byte, error) {
	values := make(map[string]string)
	for key, value := range input.SecretValues {
		key = strings.TrimSpace(key)
		if key != "" && value != "" {
			values[key] = value
		}
	}
	if len(values) == 0 {
		return nil, nil, domainError("secretValues", "required", "at least one credential value is required")
	}
	if definition.Kind != "custom" {
		allowed := make(map[string]models.ResourceCredentialField, len(definition.CredentialFields))
		for _, field := range definition.CredentialFields {
			allowed[field.Name] = field
			if field.Required && values[field.Name] == "" {
				return nil, nil, domainError("secretValues."+field.Name, "required", field.Label+" is required")
			}
		}
		for key := range values {
			if _, ok := allowed[key]; !ok {
				return nil, nil, domainError("secretValues."+key, "unsupported", "credential field is not supported by this resource kind")
			}
		}
	}
	payload, err := json.Marshal(struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}{SchemaVersion: 1, Values: values})
	if err != nil {
		return nil, nil, err
	}
	encrypted, err := secretcrypto.EncryptForPurpose(payload, service.config.App.SessionEncryptionKey, resourceCredentialPurpose)
	if err != nil {
		return nil, nil, err
	}
	key, err := hex.DecodeString(service.config.App.SessionEncryptionKey)
	if err != nil || len(key) != 32 {
		return nil, nil, errors.New("resource credential digest key is invalid")
	}
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write(payload)
	return encrypted, digest.Sum(nil), nil
}

func (service *ResourceManagement) CreateEndpoint(ctx context.Context, resourceID uuid.UUID, input ResourceEndpointInput) (models.ResourceEndpointEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	endpoint, err := service.createEndpoint(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceEndpointEntity{}, mapResourceConflict(err)
	}
	return endpoint, nil
}

func (service *ResourceManagement) createEndpoint(ctx context.Context, db storage.Executor, resource models.ResourceEntity, input ResourceEndpointInput) (models.ResourceEndpointEntity, error) {
	input.Settings = normalizedJSON(input.Settings)
	entity := models.ResourceEndpointEntity{
		Name: input.Name, Role: input.Role, Address: input.Address, Port: input.Port,
		Protocol: input.Protocol, TlsMode: input.TLSMode, Settings: input.Settings,
		ResourceID: resource.ID, ResourceInstallationID: input.ResourceInstallationID,
		PrivateNetworkID: input.PrivateNetworkID,
	}
	if err := entity.ValidateForKind(resource.Kind); err != nil {
		return models.ResourceEndpointEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	if resource.ManagementMode == models.ResourceManagementExternal && input.ResourceInstallationID != nil {
		return models.ResourceEndpointEntity{}, domainError("resourceInstallationId", "mode", "external Resources cannot use managed installations")
	}
	if err := service.validateEndpointTopology(ctx, db, resource, nil, input); err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	created, err := models.ResourceEndpoint.Create(ctx, db, models.CreateResourceEndpointData{
		Name: entity.Name, Role: entity.Role, Address: entity.Address, Port: entity.Port,
		Protocol: entity.Protocol, TlsMode: entity.TlsMode, Settings: entity.Settings,
		ResourceID: resource.ID, ResourceInstallationID: entity.ResourceInstallationID,
		PrivateNetworkID: entity.PrivateNetworkID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) UpdateEndpoint(ctx context.Context, resourceID, endpointID uuid.UUID, input ResourceEndpointInput) (models.ResourceEndpointEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	var current models.ResourceEndpointEntity
	err = tx.NewSelect().Model(&current).Where("id = ?", endpointID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceEndpointEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	input.Settings = normalizedJSON(input.Settings)
	entity := models.ResourceEndpointEntity{
		ID: current.ID, Name: input.Name, Role: input.Role, Address: input.Address, Port: input.Port,
		Protocol: input.Protocol, TlsMode: input.TLSMode, Settings: input.Settings,
		ResourceID: resource.ID, ResourceInstallationID: input.ResourceInstallationID,
		PrivateNetworkID: input.PrivateNetworkID, ArchivedAt: current.ArchivedAt,
	}
	if err := entity.ValidateForKind(resource.Kind); err != nil {
		return models.ResourceEndpointEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	if resource.ManagementMode == models.ResourceManagementExternal && input.ResourceInstallationID != nil {
		return models.ResourceEndpointEntity{}, domainError("resourceInstallationId", "mode", "external Resources cannot use managed installations")
	}
	if err := service.validateEndpointTopology(ctx, tx, resource, &current.ID, input); err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	updated, err := models.ResourceEndpoint.Update(ctx, tx, models.UpdateResourceEndpointData{
		ID: current.ID, Name: entity.Name, Role: entity.Role, Address: entity.Address, Port: entity.Port,
		Protocol: entity.Protocol, TlsMode: entity.TlsMode, Settings: entity.Settings,
		ArchivedAt: current.ArchivedAt, ResourceID: resource.ID,
		ResourceInstallationID: entity.ResourceInstallationID, PrivateNetworkID: entity.PrivateNetworkID,
	})
	if err != nil {
		return models.ResourceEndpointEntity{}, mapResourceConflict(err)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceEndpointEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) validateEndpointTopology(ctx context.Context, db storage.Executor, resource models.ResourceEntity, endpointID *uuid.UUID, input ResourceEndpointInput) error {
	var installationServerID uuid.UUID
	if input.ResourceInstallationID != nil {
		err := db.NewSelect().TableExpr("resource_installations AS installation").ColumnExpr("installation.server_id").
			Join("JOIN servers AS server ON server.id = installation.server_id AND server.archived_at IS NULL").
			Where("installation.id = ?", *input.ResourceInstallationID).Where("installation.resource_id = ?", resource.ID).
			Where("installation.archived_at IS NULL").Scan(ctx, &installationServerID)
		if errors.Is(err, sql.ErrNoRows) {
			return domainError("resourceInstallationId", "unavailable", "installation must be active and belong to this Resource")
		}
		if err != nil {
			return err
		}
	}
	if input.PrivateNetworkID != nil {
		count, err := db.NewSelect().TableExpr("private_networks").Where("id = ?", *input.PrivateNetworkID).Where("archived_at IS NULL").Count(ctx)
		if err != nil {
			return err
		}
		if err := requireChild(count, "privateNetworkId", "private network is unavailable"); err != nil {
			return err
		}
		if input.ResourceInstallationID != nil {
			count, err = db.NewSelect().TableExpr("server_networks").Where("server_id = ?", installationServerID).
				Where("private_network_id = ?", *input.PrivateNetworkID).Where("removed_at IS NULL").Count(ctx)
			if err != nil {
				return err
			}
			if err := requireChild(count, "privateNetworkId", "installation Server cannot reach this private network"); err != nil {
				return err
			}
		}
		if endpointID != nil {
			incompatible, err := db.NewSelect().TableExpr("environment_resources AS connection").
				Where("connection.resource_endpoint_id = ?", *endpointID).Where("connection.archived_at IS NULL").
				Where("NOT EXISTS (SELECT 1 FROM environment_networks AS access WHERE access.environment_id = connection.environment_id AND access.private_network_id = ? AND access.removed_at IS NULL)", *input.PrivateNetworkID).Count(ctx)
			if err != nil {
				return err
			}
			if incompatible > 0 {
				return domainError("privateNetworkId", "topology", "an existing Connected Environment cannot reach this private network")
			}
		}
	}
	return nil
}

func (service *ResourceManagement) ArchiveEndpoint(ctx context.Context, resourceID, endpointID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	var endpoint models.ResourceEndpointEntity
	err = tx.NewSelect().Model(&endpoint).Where("id = ?", endpointID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	dependencies, err := tx.NewSelect().TableExpr("resource_endpoints AS endpoint").Where("endpoint.id = ?", endpointID).
		Where("EXISTS (SELECT 1 FROM environment_resources WHERE resource_endpoint_id = endpoint.id AND archived_at IS NULL) OR EXISTS (SELECT 1 FROM resource_health_checks WHERE resource_endpoint_id = endpoint.id AND archived_at IS NULL)").Count(ctx)
	if err != nil {
		return err
	}
	if dependencies > 0 {
		return domainError("endpoint", "dependency", "endpoint is selected by an active binding or health check")
	}
	if resourceID != uuid.Nil {
		endpoints, countErr := tx.NewSelect().TableExpr("resource_endpoints").Where("resource_id = ?", resourceID).Where("archived_at IS NULL").Count(ctx)
		if countErr != nil {
			return countErr
		}
		var mode models.ResourceManagementModeEnum
		if modeErr := tx.NewSelect().TableExpr("resources").ColumnExpr("management_mode").Where("id = ?", resourceID).Scan(ctx, &mode); modeErr != nil {
			return modeErr
		}
		if mode == models.ResourceManagementExternal && endpoints == 1 {
			return domainError("endpoint", "required", "external Resources must retain at least one active endpoint")
		}
	}
	now := time.Now().UTC()
	if _, err := tx.NewUpdate().Table("resource_endpoints").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("id = ?", endpointID).Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) CreateCredential(ctx context.Context, resourceID uuid.UUID, input ResourceCredentialInput) (models.ResourceCredentialEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	credential, err := service.createCredential(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceCredentialEntity{}, mapResourceConflict(err)
	}
	return credential, nil
}

func (service *ResourceManagement) createCredential(ctx context.Context, db storage.Executor, resource models.ResourceEntity, input ResourceCredentialInput) (models.ResourceCredentialEntity, error) {
	definition, ok := models.FindResourceKind(resource.Kind)
	if !ok {
		return models.ResourceCredentialEntity{}, domainError("kind", "unsupported", "resource kind is not supported")
	}
	if input.ResourceInstallationID != nil {
		count, err := db.NewSelect().TableExpr("resource_installations").Where("id = ?", *input.ResourceInstallationID).
			Where("resource_id = ?", resource.ID).Where("archived_at IS NULL").Count(ctx)
		if err != nil {
			return models.ResourceCredentialEntity{}, err
		}
		if err := requireChild(count, "resourceInstallationId", "installation must be active and belong to this Resource"); err != nil {
			return models.ResourceCredentialEntity{}, err
		}
	}
	encrypted, digest, err := service.credentialPayload(input, definition)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	created, err := models.ResourceCredential.Create(ctx, db, models.CreateResourceCredentialData{
		Name: input.Name, Role: input.Role, Username: nullableString(input.Username), Metadata: normalizedJSON(input.Metadata),
		EncPayload: encrypted, Digest: digest, ResourceID: resource.ID, ResourceInstallationID: input.ResourceInstallationID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) UpdateCredentialMetadata(ctx context.Context, resourceID, credentialID uuid.UUID, input ResourceCredentialInput) (models.ResourceCredentialEntity, error) {
	return service.updateCredential(ctx, resourceID, credentialID, input, false)
}

func (service *ResourceManagement) RotateCredential(ctx context.Context, resourceID, credentialID uuid.UUID, input ResourceCredentialInput) (models.ResourceCredentialEntity, error) {
	return service.updateCredential(ctx, resourceID, credentialID, input, true)
}

func (service *ResourceManagement) updateCredential(ctx context.Context, resourceID, credentialID uuid.UUID, input ResourceCredentialInput, rotate bool) (models.ResourceCredentialEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	var current models.ResourceCredentialEntity
	err = tx.NewSelect().Model(&current).Where("id = ?", credentialID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceCredentialEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	if input.ResourceInstallationID != nil {
		count, countErr := tx.NewSelect().TableExpr("resource_installations").Where("id = ?", *input.ResourceInstallationID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").Count(ctx)
		if countErr != nil {
			return models.ResourceCredentialEntity{}, countErr
		}
		if err := requireChild(count, "resourceInstallationId", "installation must be active and belong to this Resource"); err != nil {
			return models.ResourceCredentialEntity{}, err
		}
	}
	encrypted, digest := current.EncPayload, current.Digest
	if rotate {
		definition, _ := models.FindResourceKind(resource.Kind)
		encrypted, digest, err = service.credentialPayload(input, definition)
		if err != nil {
			return models.ResourceCredentialEntity{}, err
		}
	}
	updated, err := models.ResourceCredential.Update(ctx, tx, models.UpdateResourceCredentialData{
		ID: current.ID, Name: input.Name, Role: input.Role, Username: nullableString(input.Username),
		Metadata: normalizedJSON(input.Metadata), EncPayload: encrypted, Digest: digest,
		ArchivedAt: current.ArchivedAt, ResourceID: resourceID, ResourceInstallationID: input.ResourceInstallationID,
	})
	if err != nil {
		return models.ResourceCredentialEntity{}, mapResourceConflict(err)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceCredentialEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) ArchiveCredential(ctx context.Context, resourceID, credentialID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	var credential models.ResourceCredentialEntity
	err = tx.NewSelect().Model(&credential).Where("id = ?", credentialID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	dependencies, err := tx.NewSelect().TableExpr("resource_credentials AS credential").Where("credential.id = ?", credentialID).
		Where("EXISTS (SELECT 1 FROM environment_resources WHERE resource_credential_id = credential.id AND archived_at IS NULL) OR EXISTS (SELECT 1 FROM resource_health_checks WHERE resource_credential_id = credential.id AND archived_at IS NULL)").Count(ctx)
	if err != nil {
		return err
	}
	if dependencies > 0 {
		return domainError("credential", "dependency", "credential is selected by an active binding or health check")
	}
	now := time.Now().UTC()
	if _, err := tx.NewUpdate().Table("resource_credentials").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("id = ?", credentialID).Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) CreateInstallation(ctx context.Context, resourceID uuid.UUID, input ResourceInstallationInput) (models.ResourceInstallationEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if resource.ManagementMode != models.ResourceManagementManaged {
		return models.ResourceInstallationEntity{}, domainError("managementMode", "mode", "installations are allowed only for managed Resources")
	}
	installation, err := service.createInstallation(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceInstallationEntity{}, mapResourceConflict(err)
	}
	return installation, nil
}

func (service *ResourceManagement) createInstallation(ctx context.Context, db storage.Executor, resource models.ResourceEntity, input ResourceInstallationInput) (models.ResourceInstallationEntity, error) {
	if resource.ManagementMode != models.ResourceManagementManaged {
		return models.ResourceInstallationEntity{}, domainError("managementMode", "mode", "installations are allowed only for managed Resources")
	}
	if err := service.validatePlacement(ctx, db, input.ServerID); err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if input.RegistryCredentialID != nil {
		count, err := db.NewSelect().TableExpr("credentials").Where("id = ?", *input.RegistryCredentialID).Where("archived_at IS NULL").Count(ctx)
		if err != nil {
			return models.ResourceInstallationEntity{}, err
		}
		if err := requireChild(count, "registryCredentialId", "registry credential is unavailable"); err != nil {
			return models.ResourceInstallationEntity{}, err
		}
	}
	created, err := models.ResourceInstallation.Create(ctx, db, models.CreateResourceInstallationData{
		ImageReference: input.ImageReference, ImageDigest: nullableString(input.ImageDigest),
		ContainerName: input.ContainerName, RestartPolicy: input.RestartPolicy,
		Configuration: normalizedJSON(input.Configuration), ResourceID: resource.ID,
		ServerID: input.ServerID, RegistryCredentialID: input.RegistryCredentialID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) UpdateInstallation(ctx context.Context, resourceID, installationID uuid.UUID, input ResourceInstallationInput) (models.ResourceInstallationEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if resource.ManagementMode != models.ResourceManagementManaged {
		return models.ResourceInstallationEntity{}, domainError("managementMode", "mode", "installations are allowed only for managed Resources")
	}
	var current models.ResourceInstallationEntity
	err = tx.NewSelect().Model(&current).Where("id = ?", installationID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceInstallationEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if err := service.validatePlacement(ctx, tx, input.ServerID); err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if input.RegistryCredentialID != nil {
		count, countErr := tx.NewSelect().TableExpr("credentials").Where("id = ?", *input.RegistryCredentialID).Where("archived_at IS NULL").Count(ctx)
		if countErr != nil {
			return models.ResourceInstallationEntity{}, countErr
		}
		if err := requireChild(count, "registryCredentialId", "registry credential is unavailable"); err != nil {
			return models.ResourceInstallationEntity{}, err
		}
	}
	if current.ServerID != input.ServerID {
		if err := service.validateInstallationMove(ctx, tx, installationID, input.ServerID); err != nil {
			return models.ResourceInstallationEntity{}, err
		}
	}
	updated, err := models.ResourceInstallation.Update(ctx, tx, models.UpdateResourceInstallationData{
		ID: current.ID, ImageReference: input.ImageReference, ImageDigest: nullableString(input.ImageDigest),
		ContainerName: input.ContainerName, RestartPolicy: input.RestartPolicy,
		Configuration: normalizedJSON(input.Configuration), ArchivedAt: current.ArchivedAt,
		ResourceID: resourceID, ServerID: input.ServerID, RegistryCredentialID: input.RegistryCredentialID,
	})
	if err != nil {
		return models.ResourceInstallationEntity{}, mapResourceConflict(err)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceInstallationEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) validatePlacement(ctx context.Context, db storage.Executor, serverID uuid.UUID) error {
	count, err := db.NewSelect().TableExpr("servers").Where("id = ?", serverID).Where("archived_at IS NULL").Count(ctx)
	if err != nil {
		return err
	}
	return requireChild(count, "serverId", "Server must be active")
}

func (service *ResourceManagement) validateInstallationMove(ctx context.Context, db storage.Executor, installationID, targetServerID uuid.UUID) error {
	unreachableNetworks, err := db.NewSelect().TableExpr("resource_endpoints AS endpoint").
		Where("endpoint.resource_installation_id = ?", installationID).Where("endpoint.archived_at IS NULL").
		Where("endpoint.private_network_id IS NOT NULL").
		Where("NOT EXISTS (SELECT 1 FROM server_networks AS access WHERE access.server_id = ? AND access.private_network_id = endpoint.private_network_id AND access.removed_at IS NULL)", targetServerID).Count(ctx)
	if err != nil {
		return err
	}
	if unreachableNetworks > 0 {
		return domainError("serverId", "network_topology", "target Server cannot reach every endpoint private network")
	}
	incompatibleVolumes, err := db.NewSelect().TableExpr("resource_volume_mounts AS mount").
		Join("JOIN resource_volumes AS volume ON volume.id = mount.resource_volume_id AND volume.archived_at IS NULL").
		Where("mount.resource_installation_id = ?", installationID).Where("mount.archived_at IS NULL").
		Where("volume.server_id <> ?", targetServerID).Count(ctx)
	if err != nil {
		return err
	}
	if incompatibleVolumes > 0 {
		return domainError("serverId", "storage_topology", "mounted server-local Volume requires explicit migration or reassignment")
	}
	return nil
}

func (service *ResourceManagement) ArchiveInstallation(ctx context.Context, resourceID, installationID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	var installation models.ResourceInstallationEntity
	err = tx.NewSelect().Model(&installation).Where("id = ?", installationID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	dependencies, err := tx.NewSelect().TableExpr("resource_installations AS installation").Where("installation.id = ?", installationID).
		Where(`EXISTS (SELECT 1 FROM resource_endpoints WHERE resource_installation_id = installation.id AND archived_at IS NULL)
			OR EXISTS (SELECT 1 FROM resource_credentials WHERE resource_installation_id = installation.id AND archived_at IS NULL)
			OR EXISTS (SELECT 1 FROM resource_volume_mounts WHERE resource_installation_id = installation.id AND archived_at IS NULL)
			OR EXISTS (SELECT 1 FROM resource_health_checks WHERE resource_installation_id = installation.id AND archived_at IS NULL)`).Count(ctx)
	if err != nil {
		return err
	}
	if dependencies > 0 {
		return domainError("installation", "dependency", "installation has active endpoints, credentials, mounts, or health checks")
	}
	now := time.Now().UTC()
	if _, err := tx.NewUpdate().Table("resource_installations").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("id = ?", installationID).Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) CreateVolume(ctx context.Context, resourceID uuid.UUID, input ResourceVolumeInput) (models.ResourceVolumeEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	if resource.ManagementMode != models.ResourceManagementManaged {
		return models.ResourceVolumeEntity{}, domainError("managementMode", "mode", "volumes are allowed only for managed Resources")
	}
	volume, err := service.createVolume(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceVolumeEntity{}, mapResourceConflict(err)
	}
	return volume, nil
}

func (service *ResourceManagement) createVolume(ctx context.Context, db storage.Executor, resource models.ResourceEntity, input ResourceVolumeInput) (models.ResourceVolumeEntity, error) {
	if resource.ManagementMode != models.ResourceManagementManaged {
		return models.ResourceVolumeEntity{}, domainError("managementMode", "mode", "volumes are allowed only for managed Resources")
	}
	if err := service.validatePlacement(ctx, db, input.ServerID); err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	created, err := models.ResourceVolume.Create(ctx, db, models.CreateResourceVolumeData{
		Name: input.Name, Driver: input.Driver, Configuration: normalizedJSON(input.Configuration),
		ResourceID: resource.ID, ServerID: input.ServerID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) UpdateVolume(ctx context.Context, resourceID, volumeID uuid.UUID, input ResourceVolumeInput) (models.ResourceVolumeEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	if resource.ManagementMode != models.ResourceManagementManaged {
		return models.ResourceVolumeEntity{}, domainError("managementMode", "mode", "volumes are allowed only for managed Resources")
	}
	var current models.ResourceVolumeEntity
	err = tx.NewSelect().Model(&current).Where("id = ?", volumeID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceVolumeEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	if err := service.validatePlacement(ctx, tx, input.ServerID); err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	if current.ServerID != input.ServerID {
		mounts, countErr := tx.NewSelect().TableExpr("resource_volume_mounts").Where("resource_volume_id = ?", volumeID).Where("archived_at IS NULL").Count(ctx)
		if countErr != nil {
			return models.ResourceVolumeEntity{}, countErr
		}
		if mounts > 0 {
			return models.ResourceVolumeEntity{}, domainError("serverId", "topology", "archive volume mounts before changing Servers")
		}
	}
	updated, err := models.ResourceVolume.Update(ctx, tx, models.UpdateResourceVolumeData{
		ID: current.ID, Name: input.Name, Driver: input.Driver, Configuration: normalizedJSON(input.Configuration),
		ArchivedAt: current.ArchivedAt, ResourceID: resourceID, ServerID: input.ServerID,
	})
	if err != nil {
		return models.ResourceVolumeEntity{}, mapResourceConflict(err)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceVolumeEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) ArchiveVolume(ctx context.Context, resourceID, volumeID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	var volume models.ResourceVolumeEntity
	err = tx.NewSelect().Model(&volume).Where("id = ?", volumeID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	mounts, err := tx.NewSelect().TableExpr("resource_volume_mounts").Where("resource_volume_id = ?", volumeID).Where("archived_at IS NULL").Count(ctx)
	if err != nil {
		return err
	}
	if mounts > 0 {
		return domainError("volume", "dependency", "volume has active mounts")
	}
	now := time.Now().UTC()
	if _, err := tx.NewUpdate().Table("resource_volumes").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("id = ?", volumeID).Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) CreateMount(ctx context.Context, resourceID uuid.UUID, input ResourceMountInput) (models.ResourceVolumeMountEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	mount, err := service.createMount(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceVolumeMountEntity{}, mapResourceConflict(err)
	}
	return mount, nil
}

func (service *ResourceManagement) createMount(ctx context.Context, db storage.Executor, resource models.ResourceEntity, input ResourceMountInput) (models.ResourceVolumeMountEntity, error) {
	if resource.ManagementMode != models.ResourceManagementManaged {
		return models.ResourceVolumeMountEntity{}, domainError("managementMode", "mode", "mounts are allowed only for managed Resources")
	}
	var topology struct {
		VolumeResourceID       uuid.UUID `bun:"volume_resource_id"`
		VolumeServerID         uuid.UUID `bun:"volume_server_id"`
		InstallationResourceID uuid.UUID `bun:"installation_resource_id"`
		InstallationServerID   uuid.UUID `bun:"installation_server_id"`
	}
	err := db.NewSelect().TableExpr("resource_volumes AS volume").
		ColumnExpr("volume.resource_id AS volume_resource_id, volume.server_id AS volume_server_id, installation.resource_id AS installation_resource_id, installation.server_id AS installation_server_id").
		Join("JOIN resource_installations AS installation ON installation.id = ? AND installation.archived_at IS NULL", input.ResourceInstallationID).
		Where("volume.id = ?", input.ResourceVolumeID).Where("volume.archived_at IS NULL").Scan(ctx, &topology)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceVolumeMountEntity{}, domainError("mount", "topology", "volume and installation must be active")
	}
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	if topology.VolumeResourceID != resource.ID || topology.InstallationResourceID != resource.ID || topology.VolumeServerID != topology.InstallationServerID {
		return models.ResourceVolumeMountEntity{}, domainError("mount", "topology", "volume and installation must belong to the same Resource and Server")
	}
	created, err := models.ResourceVolumeMount.Create(ctx, db, models.CreateResourceVolumeMountData{
		MountPath: input.MountPath, ReadOnly: input.ReadOnly, ResourceVolumeID: input.ResourceVolumeID,
		ResourceInstallationID: input.ResourceInstallationID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) UpdateMount(ctx context.Context, resourceID, mountID uuid.UUID, input ResourceMountInput) (models.ResourceVolumeMountEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	if resource.ManagementMode != models.ResourceManagementManaged {
		return models.ResourceVolumeMountEntity{}, domainError("managementMode", "mode", "mounts are allowed only for managed Resources")
	}
	var current models.ResourceVolumeMountEntity
	err = tx.NewSelect().Model(&current).Where("id = ?", mountID).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceVolumeMountEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	var owned int
	owned, err = tx.NewSelect().TableExpr("resource_volumes").Where("id = ?", current.ResourceVolumeID).Where("resource_id = ?", resourceID).Count(ctx)
	if err != nil || owned != 1 {
		if err != nil {
			return models.ResourceVolumeMountEntity{}, err
		}
		return models.ResourceVolumeMountEntity{}, models.ErrNotFound
	}
	if _, err := service.createMountValidation(ctx, tx, resource, input); err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	updated, err := models.ResourceVolumeMount.Update(ctx, tx, models.UpdateResourceVolumeMountData{
		ID: current.ID, MountPath: input.MountPath, ReadOnly: input.ReadOnly, ArchivedAt: current.ArchivedAt,
		ResourceVolumeID: input.ResourceVolumeID, ResourceInstallationID: input.ResourceInstallationID,
	})
	if err != nil {
		return models.ResourceVolumeMountEntity{}, mapResourceConflict(err)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceVolumeMountEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) createMountValidation(ctx context.Context, db storage.Executor, resource models.ResourceEntity, input ResourceMountInput) (models.ResourceVolumeMountEntity, error) {
	entity := models.ResourceVolumeMountEntity{MountPath: input.MountPath, ReadOnly: input.ReadOnly, ResourceVolumeID: input.ResourceVolumeID, ResourceInstallationID: input.ResourceInstallationID}
	if err := entity.Validate(); err != nil {
		return models.ResourceVolumeMountEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	var topology struct {
		VolumeResourceID       uuid.UUID `bun:"volume_resource_id"`
		VolumeServerID         uuid.UUID `bun:"volume_server_id"`
		InstallationResourceID uuid.UUID `bun:"installation_resource_id"`
		InstallationServerID   uuid.UUID `bun:"installation_server_id"`
	}
	err := db.NewSelect().TableExpr("resource_volumes AS volume").
		ColumnExpr("volume.resource_id AS volume_resource_id, volume.server_id AS volume_server_id, installation.resource_id AS installation_resource_id, installation.server_id AS installation_server_id").
		Join("JOIN resource_installations AS installation ON installation.id = ? AND installation.archived_at IS NULL", input.ResourceInstallationID).
		Where("volume.id = ?", input.ResourceVolumeID).Where("volume.archived_at IS NULL").Scan(ctx, &topology)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceVolumeMountEntity{}, domainError("mount", "topology", "volume and installation must be active")
	}
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	if topology.VolumeResourceID != resource.ID || topology.InstallationResourceID != resource.ID || topology.VolumeServerID != topology.InstallationServerID {
		return models.ResourceVolumeMountEntity{}, domainError("mount", "topology", "volume and installation must belong to the same Resource and Server")
	}
	return entity, nil
}

func (service *ResourceManagement) ArchiveMount(ctx context.Context, resourceID, mountID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	var mount models.ResourceVolumeMountEntity
	err = tx.NewSelect().Model(&mount).
		Join("JOIN resource_volumes AS volume ON volume.id = resource_volume_mounts.resource_volume_id").
		Where("resource_volume_mounts.id = ?", mountID).Where("volume.resource_id = ?", resourceID).
		Where("resource_volume_mounts.archived_at IS NULL").For("UPDATE OF resource_volume_mounts").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := tx.NewUpdate().Table("resource_volume_mounts").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("id = ?", mountID).Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) CreateHealthCheck(ctx context.Context, resourceID uuid.UUID, input ResourceHealthCheckInput) (models.ResourceHealthCheckEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	check, err := service.createHealthCheck(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceHealthCheckEntity{}, mapResourceConflict(err)
	}
	return check, nil
}

func (service *ResourceManagement) createHealthCheck(ctx context.Context, db storage.Executor, resource models.ResourceEntity, input ResourceHealthCheckInput) (models.ResourceHealthCheckEntity, error) {
	if resource.ManagementMode != models.ResourceManagementManaged {
		return models.ResourceHealthCheckEntity{}, domainError("managementMode", "mode", "health checks are allowed only for managed Resources")
	}
	entity := models.ResourceHealthCheckEntity{
		Name: input.Name, Kind: input.Kind, Configuration: normalizedJSON(input.Configuration),
		IntervalSeconds: input.IntervalSeconds, TimeoutSeconds: input.TimeoutSeconds,
		FailureThreshold: input.FailureThreshold, SuccessThreshold: input.SuccessThreshold, Enabled: input.Enabled,
		ResourceInstallationID: input.ResourceInstallationID, ResourceEndpointID: input.ResourceEndpointID,
		ResourceCredentialID: input.ResourceCredentialID,
	}
	if err := entity.ValidateForKind(resource.Kind); err != nil {
		return models.ResourceHealthCheckEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	if err := service.validateHealthTopology(ctx, db, resource.ID, input); err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	created, err := models.ResourceHealthCheck.Create(ctx, db, models.CreateResourceHealthCheckData{
		Name: entity.Name, Kind: entity.Kind, Configuration: entity.Configuration,
		IntervalSeconds: entity.IntervalSeconds, TimeoutSeconds: entity.TimeoutSeconds,
		FailureThreshold: entity.FailureThreshold, SuccessThreshold: entity.SuccessThreshold, Enabled: entity.Enabled,
		ResourceInstallationID: entity.ResourceInstallationID, ResourceEndpointID: entity.ResourceEndpointID,
		ResourceCredentialID: entity.ResourceCredentialID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) validateHealthTopology(ctx context.Context, db storage.Executor, resourceID uuid.UUID, input ResourceHealthCheckInput) error {
	installations, err := db.NewSelect().TableExpr("resource_installations").Where("id = ?", input.ResourceInstallationID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").Count(ctx)
	if err != nil {
		return err
	}
	if err := requireChild(installations, "resourceInstallationId", "installation must be active and belong to this Resource"); err != nil {
		return err
	}
	if input.ResourceEndpointID != nil {
		endpoints, countErr := db.NewSelect().TableExpr("resource_endpoints").Where("id = ?", *input.ResourceEndpointID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").
			Where("resource_installation_id IS NULL OR resource_installation_id = ?", input.ResourceInstallationID).Count(ctx)
		if countErr != nil {
			return countErr
		}
		if err := requireChild(endpoints, "resourceEndpointId", "endpoint does not belong to this installation topology"); err != nil {
			return err
		}
	}
	if input.ResourceCredentialID != nil {
		credentials, countErr := db.NewSelect().TableExpr("resource_credentials").Where("id = ?", *input.ResourceCredentialID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").
			Where("resource_installation_id IS NULL OR resource_installation_id = ?", input.ResourceInstallationID).Count(ctx)
		if countErr != nil {
			return countErr
		}
		if err := requireChild(credentials, "resourceCredentialId", "credential does not belong to this installation topology"); err != nil {
			return err
		}
	}
	return nil
}

func (service *ResourceManagement) UpdateHealthCheck(ctx context.Context, resourceID, healthCheckID uuid.UUID, input ResourceHealthCheckInput) (models.ResourceHealthCheckEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	if resource.ManagementMode != models.ResourceManagementManaged {
		return models.ResourceHealthCheckEntity{}, domainError("managementMode", "mode", "health checks are allowed only for managed Resources")
	}
	var current models.ResourceHealthCheckEntity
	err = tx.NewSelect().Model(&current).Join("JOIN resource_installations AS installation ON installation.id = resource_health_checks.resource_installation_id").
		Where("resource_health_checks.id = ?", healthCheckID).Where("installation.resource_id = ?", resourceID).
		Where("resource_health_checks.archived_at IS NULL").For("UPDATE OF resource_health_checks").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceHealthCheckEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	entity := models.ResourceHealthCheckEntity{
		ID: current.ID, Name: input.Name, Kind: input.Kind, Configuration: normalizedJSON(input.Configuration),
		IntervalSeconds: input.IntervalSeconds, TimeoutSeconds: input.TimeoutSeconds,
		FailureThreshold: input.FailureThreshold, SuccessThreshold: input.SuccessThreshold, Enabled: input.Enabled,
		ArchivedAt: current.ArchivedAt, ResourceInstallationID: input.ResourceInstallationID,
		ResourceEndpointID: input.ResourceEndpointID, ResourceCredentialID: input.ResourceCredentialID,
	}
	if err := entity.ValidateForKind(resource.Kind); err != nil {
		return models.ResourceHealthCheckEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	if err := service.validateHealthTopology(ctx, tx, resourceID, input); err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	updated, err := models.ResourceHealthCheck.Update(ctx, tx, models.UpdateResourceHealthCheckData{
		ID: current.ID, Name: entity.Name, Kind: entity.Kind, Configuration: entity.Configuration,
		IntervalSeconds: entity.IntervalSeconds, TimeoutSeconds: entity.TimeoutSeconds,
		FailureThreshold: entity.FailureThreshold, SuccessThreshold: entity.SuccessThreshold, Enabled: entity.Enabled,
		ArchivedAt: current.ArchivedAt, ResourceInstallationID: entity.ResourceInstallationID,
		ResourceEndpointID: entity.ResourceEndpointID, ResourceCredentialID: entity.ResourceCredentialID,
	})
	if err != nil {
		return models.ResourceHealthCheckEntity{}, mapResourceConflict(err)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceHealthCheckEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) ArchiveHealthCheck(ctx context.Context, resourceID, healthCheckID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	var check models.ResourceHealthCheckEntity
	err = tx.NewSelect().Model(&check).Join("JOIN resource_installations AS installation ON installation.id = resource_health_checks.resource_installation_id").
		Where("resource_health_checks.id = ?", healthCheckID).Where("installation.resource_id = ?", resourceID).
		Where("resource_health_checks.archived_at IS NULL").For("UPDATE OF resource_health_checks").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := tx.NewUpdate().Table("resource_health_checks").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("id = ?", healthCheckID).Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func requireChild(count int, field, message string) error {
	if count != 1 {
		return domainError(field, "topology", message)
	}
	return nil
}
