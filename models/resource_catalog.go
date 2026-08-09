package models

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"

	"github.com/google/uuid"
)

func (resource) ListCatalog(ctx context.Context, db storage.Executor, filters ResourceListFilters) ([]ResourceListItem, error) {
	items := make([]ResourceListItem, 0)
	query := db.NewSelect().TableExpr("resources AS resource").ColumnExpr("resource.id, resource.name, resource.slug, resource.resource_type, resource.configuration ->> 'engine' AS engine, resource.system_managed, resource.environment_attachable").ColumnExpr("CASE WHEN resource.resource_type = 'database' THEN jsonb_array_length(COALESCE(resource.configuration -> 'databases', '[]'::jsonb)) ELSE 0 END AS database_count").ColumnExpr("(SELECT count(*) FROM environment_resources AS connection WHERE connection.resource_id = resource.id AND connection.archived_at IS NULL) AS connection_count").ColumnExpr("(SELECT count(*) FROM resource_installations AS installation WHERE installation.resource_id = resource.id AND installation.archived_at IS NULL) AS installation_count").ColumnExpr("(SELECT count(*) FROM resource_endpoints AS endpoint WHERE endpoint.resource_id = resource.id AND endpoint.archived_at IS NULL) AS endpoint_count").
		ColumnExpr(`CASE WHEN EXISTS (SELECT 1 FROM resource_health_checks AS health_check JOIN resource_health_check_statuses AS status ON status.health_check_id = health_check.id WHERE health_check.resource_id = resource.id AND health_check.archived_at IS NULL AND health_check.enabled = TRUE AND status.expires_at > CURRENT_TIMESTAMP AND status.state = 'unhealthy') THEN 'unhealthy' WHEN EXISTS (SELECT 1 FROM resource_health_checks AS health_check JOIN resource_health_check_statuses AS status ON status.health_check_id = health_check.id WHERE health_check.resource_id = resource.id AND health_check.archived_at IS NULL AND health_check.enabled = TRUE AND status.expires_at > CURRENT_TIMESTAMP AND status.state = 'degraded') THEN 'degraded' WHEN EXISTS (SELECT 1 FROM resource_health_checks AS health_check WHERE health_check.resource_id = resource.id AND health_check.archived_at IS NULL AND health_check.enabled = TRUE) AND NOT EXISTS (SELECT 1 FROM resource_health_checks AS health_check LEFT JOIN resource_health_check_statuses AS status ON status.health_check_id = health_check.id WHERE health_check.resource_id = resource.id AND health_check.archived_at IS NULL AND health_check.enabled = TRUE AND (status.health_check_id IS NULL OR status.expires_at <= CURRENT_TIMESTAMP OR status.state <> 'healthy')) THEN 'healthy' ELSE 'unknown' END AS health`).Where("resource.archived_at IS NULL").OrderExpr("resource.system_managed ASC, resource.name ASC")
	if search := strings.TrimSpace(filters.Search); search != "" {
		query = query.Where("resource.name ILIKE ?", "%"+search+"%")
	}
	if filters.Engine != "" {
		query = query.Where("resource.configuration ->> 'engine' = ?", filters.Engine)
	}
	if filters.ResourceType != "" {
		query = query.Where("resource.resource_type = ?", filters.ResourceType)
	}
	return items, query.Scan(ctx, &items)
}

type ResourceNetworkServer struct {
	NetworkID uuid.UUID `bun:"network_id"`
	ServerID  uuid.UUID `bun:"server_id"`
	Address   string    `bun:"address"`
}

func (resource) FormOptions(ctx context.Context, db storage.Executor) (ResourceFormOptions, error) {
	options := ResourceFormOptions{Servers: make([]ResourceServerOption, 0), PrivateNetworks: make([]ResourceNetworkOption, 0), RegistryCredentials: make([]ResourceRegistryCredentialOption, 0)}
	if err := db.NewSelect().TableExpr("servers AS server").ColumnExpr("server.id, server.name, server.address").Where("server.archived_at IS NULL").Where("server.is_configured = TRUE").Where("server.kind IN ('self_hosted', 'worker')").Where("server.capabilities @> '{\"resource\":true}'::jsonb").OrderExpr("server.name").Scan(ctx, &options.Servers); err != nil {
		return ResourceFormOptions{}, err
	}
	if err := db.NewSelect().TableExpr("private_networks AS network").ColumnExpr("network.id, network.name").Where("network.archived_at IS NULL").OrderExpr("network.name").Scan(ctx, &options.PrivateNetworks); err != nil {
		return ResourceFormOptions{}, err
	}
	if err := db.NewSelect().TableExpr("credentials AS credential").ColumnExpr("credential.id, credential.name").Where("credential.archived_at IS NULL").OrderExpr("credential.name").Scan(ctx, &options.RegistryCredentials); err != nil {
		return ResourceFormOptions{}, err
	}
	access := make([]ResourceNetworkServer, 0)
	if err := db.NewSelect().TableExpr("server_networks").ColumnExpr("private_network_id AS network_id, server_id, COALESCE(configuration ->> 'address', '') AS address").Where("driver = 'wireguard'").Where("removed_at IS NULL").Scan(ctx, &access); err != nil {
		return ResourceFormOptions{}, err
	}
	indexes := make(map[uuid.UUID]int, len(options.PrivateNetworks))
	for index := range options.PrivateNetworks {
		indexes[options.PrivateNetworks[index].ID] = index
		options.PrivateNetworks[index].ServerIDs = make([]uuid.UUID, 0)
		options.PrivateNetworks[index].ServerAddresses = make(map[uuid.UUID]string)
	}
	for _, item := range access {
		if index, ok := indexes[item.NetworkID]; ok {
			options.PrivateNetworks[index].ServerIDs = append(options.PrivateNetworks[index].ServerIDs, item.ServerID)
			if item.Address != "" {
				options.PrivateNetworks[index].ServerAddresses[item.ServerID] = item.Address
			}
		}
	}
	return options, nil
}

func (resource) DetailCatalog(ctx context.Context, db storage.Executor, entity ResourceEntity) (ResourceDetails, error) {
	detail := ResourceDetails{Resource: entity, Connections: make([]ResourceConnectionDetail, 0), Endpoints: make([]ResourceEndpointEntity, 0), Credentials: make([]ResourceCredentialEntity, 0), Installations: make([]ResourceInstallationDetail, 0), Volumes: make([]ResourceVolumeDetail, 0), Mounts: make([]ResourceMountDetail, 0), HealthChecks: make([]ResourceHealthCheckDetail, 0)}
	id := entity.ID
	if err := db.NewSelect().TableExpr("environment_resources AS connection").ColumnExpr("connection.*").ColumnExpr("environment.name AS environment_name, environment.kind AS environment_kind, environment.archived_at IS NOT NULL AS environment_archived").ColumnExpr("application.name AS application_name, application.slug AS application_slug, application.archived_at IS NOT NULL AS application_archived").ColumnExpr("endpoint.name AS endpoint_name, COALESCE(credential.metadata, '{}'::jsonb) AS credential_metadata, COALESCE(credential.name, '') AS credential_name").Join("JOIN environments AS environment ON environment.id = connection.environment_id AND environment.archived_at IS NULL").Join("JOIN applications AS application ON application.id = environment.application_id AND application.archived_at IS NULL").Join("JOIN resource_endpoints AS endpoint ON endpoint.id = connection.resource_endpoint_id AND endpoint.archived_at IS NULL").Join("LEFT JOIN resource_credentials AS credential ON credential.id = connection.resource_credential_id AND credential.archived_at IS NULL").Where("connection.resource_id = ?", id).Where("connection.archived_at IS NULL").OrderExpr("application.name, environment.name").Scan(ctx, &detail.Connections); err != nil {
		return ResourceDetails{}, err
	}
	if err := db.NewSelect().Model(&detail.Endpoints).Where("resource_id = ?", id).Where("archived_at IS NULL").OrderExpr("name").Scan(ctx); err != nil {
		return ResourceDetails{}, err
	}
	if err := db.NewSelect().Model(&detail.Credentials).Where("resource_id = ?", id).Where("archived_at IS NULL").OrderExpr("name").Scan(ctx); err != nil {
		return ResourceDetails{}, err
	}
	if err := db.NewSelect().TableExpr("resource_installations AS installation").ColumnExpr("installation.*").ColumnExpr("server.name AS server_name, server.address AS server_address").ColumnExpr("COALESCE(status.state, '') AS state, COALESCE(status.service_state, '') AS service_state, COALESCE(status.health, '') AS health, COALESCE(status.health_reason, '') AS health_reason, COALESCE(status.details, '{}'::jsonb) AS container_details, status.observed_at").Join("JOIN servers AS server ON server.id = installation.server_id").Join("LEFT JOIN resource_installation_statuses AS status ON status.resource_installation_id = installation.id").Where("installation.resource_id = ?", id).Where("installation.archived_at IS NULL").OrderExpr("installation.created_at").Scan(ctx, &detail.Installations); err != nil {
		return ResourceDetails{}, err
	}
	if err := db.NewSelect().TableExpr("resource_volumes AS volume").ColumnExpr("volume.*, server.name AS server_name").Join("JOIN servers AS server ON server.id = volume.server_id").Where("volume.resource_id = ?", id).Where("volume.archived_at IS NULL").OrderExpr("volume.name").Scan(ctx, &detail.Volumes); err != nil {
		return ResourceDetails{}, err
	}
	if err := db.NewSelect().TableExpr("resource_volume_mounts AS mount").ColumnExpr("mount.*, volume.name AS volume_name, installation.container_name").Join("JOIN resource_volumes AS volume ON volume.id = mount.resource_volume_id").Join("JOIN resource_installations AS installation ON installation.id = mount.resource_installation_id").Where("volume.resource_id = ?", id).Where("mount.archived_at IS NULL").OrderExpr("mount.mount_path").Scan(ctx, &detail.Mounts); err != nil {
		return ResourceDetails{}, err
	}
	if err := db.NewSelect().TableExpr("resource_health_checks AS health_check").ColumnExpr("health_check.*").ColumnExpr("CASE WHEN status.expires_at > CURRENT_TIMESTAMP THEN status.state ELSE 'unknown' END AS state").ColumnExpr("CASE WHEN status.health_check_id IS NOT NULL AND status.expires_at <= CURRENT_TIMESTAMP THEN 'Health status is stale.' ELSE COALESCE(status.message, '') END AS message").ColumnExpr("status.latency_ms, COALESCE(status.consecutive_successes, 0) AS consecutive_successes, COALESCE(status.consecutive_failures, 0) AS consecutive_failures, status.observed_at, status.expires_at").Join("LEFT JOIN resource_health_check_statuses AS status ON status.health_check_id = health_check.id").Where("health_check.resource_id = ?", id).Where("health_check.archived_at IS NULL").OrderExpr("health_check.name").Scan(ctx, &detail.HealthChecks); err != nil {
		return ResourceDetails{}, err
	}
	return detail, nil
}

type ResourceArchiveDependencies struct {
	BindingCount       int
	PrivateAccessCount int
	Installations      []ResourceInstallationEntity
	Volumes            []ResourceVolumeEntity
}

func (resource) ArchiveDependencies(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (ResourceArchiveDependencies, error) {
	var result ResourceArchiveDependencies
	var err error
	result.BindingCount, err = db.NewSelect().TableExpr("environment_resources").Where("resource_id = ?", resourceID).Where("archived_at IS NULL").Count(ctx)
	if err != nil {
		return result, err
	}
	result.PrivateAccessCount, err = db.NewSelect().TableExpr("wireguard_device_resource_grants").Where("resource_id = ?", resourceID).Where("revoked_at IS NULL").Count(ctx)
	if err != nil {
		return result, err
	}
	err = db.NewSelect().Model(&result.Installations).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").OrderExpr("created_at").Scan(ctx)
	if err != nil {
		return result, err
	}
	err = db.NewSelect().Model(&result.Volumes).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").OrderExpr("created_at").Scan(ctx)
	return result, err
}
func (resource) ArchiveCascade(ctx context.Context, db storage.Executor, resourceID uuid.UUID, at time.Time) error {
	if _, err := db.NewUpdate().Table("backup_policies").Set("activated_at = NULL").Set("archived_at = ?", at).Set("updated_at = ?", at).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").Exec(ctx); err != nil {
		return err
	}
	statements := []struct{ table, where string }{{"resource_volume_mounts", "resource_installation_id IN (SELECT id FROM resource_installations WHERE resource_id = ?)"}, {"resource_health_checks", "resource_id = ?"}, {"resource_endpoints", "resource_id = ?"}, {"resource_credentials", "resource_id = ?"}, {"resource_installations", "resource_id = ?"}, {"resource_volumes", "resource_id = ?"}, {"resources", "id = ?"}}
	for _, statement := range statements {
		if _, err := db.NewUpdate().Table(statement.table).Set("archived_at = ?", at).Set("updated_at = ?", at).Where(statement.where, resourceID).Where("archived_at IS NULL").Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}
func (resource) FindActive(ctx context.Context, db storage.Executor, id uuid.UUID, lock bool) (ResourceEntity, error) {
	var row ResourceEntity
	query := db.NewSelect().TableExpr("resources AS resource").ColumnExpr("resource.*").Where("resource.id = ?", id).Where("resource.archived_at IS NULL")
	if lock {
		query = query.For("UPDATE OF resource")
	}
	err := query.Scan(ctx, &row)
	if errors.Is(err, sql.ErrNoRows) {
		return ResourceEntity{}, ErrNotFound
	}
	return row, err
}
func (resource) HasKindDependencies(ctx context.Context, db storage.Executor, id uuid.UUID) (bool, error) {
	count, err := db.NewSelect().TableExpr("resources AS resource").Where("resource.id = ?", id).Where(`EXISTS (SELECT 1 FROM resource_endpoints WHERE resource_id = resource.id AND archived_at IS NULL) OR EXISTS (SELECT 1 FROM resource_credentials WHERE resource_id = resource.id AND archived_at IS NULL) OR EXISTS (SELECT 1 FROM resource_health_checks WHERE resource_id = resource.id AND archived_at IS NULL)`).Count(ctx)
	return count > 0, err
}
func (resourceEndpoint) ActivePrimaryPublicCount(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (int, error) {
	return db.NewSelect().TableExpr("resource_endpoints").Where("resource_id = ?", resourceID).Where("role = 'primary'").Where("private_network_id IS NULL").Where("archived_at IS NULL").Count(ctx)
}
func (resourceEndpoint) ActiveForResource(ctx context.Context, db storage.Executor, resourceID uuid.UUID) ([]ResourceEndpointEntity, error) {
	rows := make([]ResourceEndpointEntity, 0)
	err := db.NewSelect().Model(&rows).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").OrderExpr("created_at").Scan(ctx)
	return rows, err
}
func (serverNetwork) WireGuardAddress(ctx context.Context, db storage.Executor, serverID, networkID uuid.UUID) (string, error) {
	var address string
	err := db.NewSelect().TableExpr("server_networks").ColumnExpr("COALESCE(configuration ->> 'address', '')").Where("server_id = ?", serverID).Where("private_network_id = ?", networkID).Where("driver = 'wireguard'").Where("removed_at IS NULL").Limit(1).Scan(ctx, &address)
	return address, err
}
func (resourceCredential) FindAdministrator(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (ResourceCredentialEntity, error) {
	var row ResourceCredentialEntity
	err := db.NewSelect().Model(&row).Where("resource_id = ?", resourceID).Where("metadata ->> 'purpose' = 'administrator'").Where("archived_at IS NULL").OrderExpr("created_at").Limit(1).Scan(ctx)
	return row, err
}
func (resourceEndpoint) FindPrimary(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (ResourceEndpointEntity, error) {
	var row ResourceEndpointEntity
	err := db.NewSelect().Model(&row).Where("resource_id = ?", resourceID).Where("role = 'primary'").Where("archived_at IS NULL").OrderExpr("created_at").Limit(1).Scan(ctx)
	return row, err
}
func (resourceCredential) ActiveForResourceAll(ctx context.Context, db storage.Executor, resourceID uuid.UUID) ([]ResourceCredentialEntity, error) {
	rows := make([]ResourceCredentialEntity, 0)
	err := db.NewSelect().Model(&rows).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").OrderExpr("created_at").Scan(ctx)
	return rows, err
}

type PostgreSQLCredentialTopology struct {
	AdministratorUsername string `bun:"administrator_username"`
	AdministratorPayload  []byte `bun:"administrator_payload"`
	Address               string `bun:"address"`
	Port                  int32  `bun:"port"`
}

func (resource) PostgreSQLCredentialTopology(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (PostgreSQLCredentialTopology, error) {
	var row PostgreSQLCredentialTopology
	err := db.NewSelect().TableExpr("resources AS resource").ColumnExpr("administrator.username AS administrator_username, administrator.enc_payload AS administrator_payload").ColumnExpr("endpoint.address, endpoint.port").Join("JOIN resource_credentials AS administrator ON administrator.resource_id = resource.id AND administrator.metadata ->> 'purpose' = 'administrator' AND administrator.archived_at IS NULL").Join("JOIN resource_endpoints AS endpoint ON endpoint.resource_id = resource.id AND endpoint.role = 'primary' AND endpoint.archived_at IS NULL").Where("resource.id = ?", resourceID).Where("resource.archived_at IS NULL").Scan(ctx, &row)
	return row, err
}
func (privateNetwork) ActiveExists(ctx context.Context, db storage.Executor, id uuid.UUID) (bool, error) {
	count, err := db.NewSelect().TableExpr("private_networks").Where("id = ?", id).Where("archived_at IS NULL").Count(ctx)
	return count == 1, err
}
func (resourceEndpoint) WireGuardGatewayExists(ctx context.Context, db storage.Executor, resourceID, networkID uuid.UUID) (bool, error) {
	count, err := db.NewSelect().TableExpr("resource_endpoints").Where("resource_id = ?", resourceID).Where("private_network_id = ?", networkID).Where("role = 'wireguard'").Where("name = 'Private access'").Where("archived_at IS NULL").Count(ctx)
	return count == 1, err
}
func (environmentResource) IncompatibleEndpointNetworkCount(ctx context.Context, db storage.Executor, endpointID, networkID uuid.UUID) (int, error) {
	return db.NewSelect().TableExpr("environment_resources AS connection").Where("connection.resource_endpoint_id = ?", endpointID).Where("connection.archived_at IS NULL").Where("NOT EXISTS (SELECT 1 FROM environment_networks AS access WHERE access.environment_id = connection.environment_id AND access.private_network_id = ? AND access.removed_at IS NULL)", networkID).Count(ctx)
}
func (resourceEndpoint) LockActiveForResource(ctx context.Context, db storage.Executor, resourceID, endpointID uuid.UUID) (ResourceEndpointEntity, error) {
	var row ResourceEndpointEntity
	err := db.NewSelect().Model(&row).Where("id = ?", endpointID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	return row, err
}

type PostgreSQLCredentialCounts struct{ Administrators, Usernames int }

func (resourceCredential) PostgreSQLCounts(ctx context.Context, db storage.Executor, resourceID uuid.UUID, username string, excludeID *uuid.UUID) (PostgreSQLCredentialCounts, error) {
	var result PostgreSQLCredentialCounts
	administratorQuery := db.NewSelect().TableExpr("resource_credentials").Where("resource_id = ?", resourceID).Where("metadata ->> 'purpose' = 'administrator'").Where("archived_at IS NULL")
	usernameQuery := db.NewSelect().TableExpr("resource_credentials").Where("resource_id = ?", resourceID).Where("username = ?", username).Where("archived_at IS NULL")
	if excludeID != nil {
		administratorQuery = administratorQuery.Where("id <> ?", *excludeID)
		usernameQuery = usernameQuery.Where("id <> ?", *excludeID)
	}
	var err error
	result.Administrators, err = administratorQuery.Count(ctx)
	if err != nil {
		return result, err
	}
	result.Usernames, err = usernameQuery.Count(ctx)
	return result, err
}
func (resourceCredential) LockActiveForResource(ctx context.Context, db storage.Executor, resourceID, credentialID uuid.UUID) (ResourceCredentialEntity, error) {
	var row ResourceCredentialEntity
	err := db.NewSelect().Model(&row).Where("id = ?", credentialID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	return row, err
}
func activeEnvironmentResourceConnections(ctx context.Context, db storage.Executor, column string, value uuid.UUID) ([]EnvironmentResourceEntity, error) {
	rows := make([]EnvironmentResourceEntity, 0)
	err := db.NewSelect().Model(&rows).Join("JOIN environments AS environment ON environment.id = environment_resources.environment_id AND environment.archived_at IS NULL").Join("JOIN applications AS application ON application.id = environment.application_id AND application.archived_at IS NULL").Where(column+" = ?", value).Where("environment_resources.archived_at IS NULL").OrderExpr("environment_resources.environment_id, environment_resources.id").Scan(ctx)
	return rows, err
}
func (environmentResource) ActiveForResourceID(ctx context.Context, db storage.Executor, id uuid.UUID) ([]EnvironmentResourceEntity, error) {
	return activeEnvironmentResourceConnections(ctx, db, "resource_id", id)
}
func (environmentResource) ActiveForEndpointID(ctx context.Context, db storage.Executor, id uuid.UUID) ([]EnvironmentResourceEntity, error) {
	return activeEnvironmentResourceConnections(ctx, db, "resource_endpoint_id", id)
}
func (environmentResource) ActiveForCredentialID(ctx context.Context, db storage.Executor, id uuid.UUID) ([]EnvironmentResourceEntity, error) {
	return activeEnvironmentResourceConnections(ctx, db, "resource_credential_id", id)
}
func (environmentResource) LockActiveConnection(ctx context.Context, db storage.Executor, resourceID, connectionID uuid.UUID) (EnvironmentResourceEntity, error) {
	var row EnvironmentResourceEntity
	err := db.NewSelect().Model(&row).Join("JOIN environments AS environment ON environment.id = environment_resources.environment_id AND environment.archived_at IS NULL").Join("JOIN applications AS application ON application.id = environment.application_id AND application.archived_at IS NULL").Where("environment_resources.id = ?", connectionID).Where("environment_resources.resource_id = ?", resourceID).Where("environment_resources.archived_at IS NULL").For("UPDATE").Scan(ctx)
	return row, err
}
func (resourceCredential) ActiveDependencyCount(ctx context.Context, db storage.Executor, id uuid.UUID) (int, error) {
	return db.NewSelect().TableExpr("resource_credentials AS credential").Where("credential.id = ?", id).Where("EXISTS (SELECT 1 FROM environment_resources WHERE resource_credential_id = credential.id AND archived_at IS NULL) OR EXISTS (SELECT 1 FROM resource_health_checks WHERE resource_credential_id = credential.id AND archived_at IS NULL)").Count(ctx)
}
func (resourceHealthCheck) ActiveKindCount(ctx context.Context, db storage.Executor, resourceID uuid.UUID, kind string) (int, error) {
	return db.NewSelect().TableExpr("resource_health_checks").Where("resource_id = ?", resourceID).Where("kind = ?", kind).Where("archived_at IS NULL").Count(ctx)
}
func (resourceInstallation) ActiveCountForResource(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (int, error) {
	return db.NewSelect().TableExpr("resource_installations").Where("resource_id = ?", resourceID).Where("archived_at IS NULL").Count(ctx)
}
func (credential) ActiveExists(ctx context.Context, db storage.Executor, id uuid.UUID) (bool, error) {
	count, err := db.NewSelect().TableExpr("credentials").Where("id = ?", id).Where("archived_at IS NULL").Count(ctx)
	return count == 1, err
}
func (resourceInstallation) LockActiveForResource(ctx context.Context, db storage.Executor, resourceID, installationID uuid.UUID) (ResourceInstallationEntity, error) {
	var row ResourceInstallationEntity
	err := db.NewSelect().Model(&row).Where("id = ?", installationID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	return row, err
}
func (resourceInstallation) FindActiveForResourceID(ctx context.Context, db storage.Executor, resourceID, installationID uuid.UUID) (ResourceInstallationEntity, error) {
	var row ResourceInstallationEntity
	err := db.NewSelect().Model(&row).Where("id = ?", installationID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").Scan(ctx)
	return row, err
}
func (resourceInstallation) ActiveForResource(ctx context.Context, db storage.Executor, resourceID uuid.UUID) ([]ResourceInstallationEntity, error) {
	rows := make([]ResourceInstallationEntity, 0)
	err := db.NewSelect().Model(&rows).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").OrderExpr("created_at").Scan(ctx)
	return rows, err
}
func (resource) EngineForID(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (string, error) {
	var engine string
	err := db.NewSelect().TableExpr("resources").ColumnExpr("configuration ->> 'engine'").Where("id = ?", resourceID).Scan(ctx, &engine)
	return engine, err
}

type InstallationMoveConflicts struct{ UnreachableNetworks, IncompatibleVolumes int }

func (resourceInstallation) MoveConflicts(ctx context.Context, db storage.Executor, installationID, targetServerID uuid.UUID) (InstallationMoveConflicts, error) {
	var result InstallationMoveConflicts
	var err error
	result.UnreachableNetworks, err = db.NewSelect().TableExpr("resource_endpoints AS endpoint").Where("endpoint.resource_id = (SELECT resource_id FROM resource_installations WHERE id = ?)", installationID).Where("endpoint.archived_at IS NULL").Where("endpoint.private_network_id IS NOT NULL").Where("NOT EXISTS (SELECT 1 FROM server_networks AS access WHERE access.server_id = ? AND access.private_network_id = endpoint.private_network_id AND access.removed_at IS NULL)", targetServerID).Count(ctx)
	if err != nil {
		return result, err
	}
	result.IncompatibleVolumes, err = db.NewSelect().TableExpr("resource_volume_mounts AS mount").Join("JOIN resource_volumes AS volume ON volume.id = mount.resource_volume_id AND volume.archived_at IS NULL").Where("mount.resource_installation_id = ?", installationID).Where("mount.archived_at IS NULL").Where("volume.server_id <> ?", targetServerID).Count(ctx)
	return result, err
}
func (resourceInstallation) ActiveMountDependencyCount(ctx context.Context, db storage.Executor, installationID uuid.UUID) (int, error) {
	return db.NewSelect().TableExpr("resource_installations AS installation").Where("installation.id = ?", installationID).Where("EXISTS (SELECT 1 FROM resource_volume_mounts WHERE resource_installation_id = installation.id AND archived_at IS NULL)").Count(ctx)
}
func (resourceInstallation) ArchiveID(ctx context.Context, db storage.Executor, id uuid.UUID, at time.Time) error {
	_, err := db.NewUpdate().Table("resource_installations").Set("archived_at = ?", at).Set("updated_at = ?", at).Where("id = ?", id).Exec(ctx)
	return err
}
func (resourceRestore) ActiveCountForResource(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (int, error) {
	return db.NewSelect().TableExpr("resource_restores AS restore").Where("restore.resource_id = ?", resourceID).Where("restore.status IN (?, ?, ?)", ResourceRestoreStatusPending, ResourceRestoreStatusSafetyBackup, ResourceRestoreStatusRestoring).Count(ctx)
}
func (resourceVolume) ActiveCountForResource(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (int, error) {
	return db.NewSelect().TableExpr("resource_volumes").Where("resource_id = ?", resourceID).Where("archived_at IS NULL").Count(ctx)
}
func (resourceVolume) LockActiveForResource(ctx context.Context, db storage.Executor, resourceID, volumeID uuid.UUID) (ResourceVolumeEntity, error) {
	var row ResourceVolumeEntity
	err := db.NewSelect().Model(&row).Where("id = ?", volumeID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	return row, err
}
func (resourceVolumeMount) ActiveCountForVolume(ctx context.Context, db storage.Executor, volumeID uuid.UUID) (int, error) {
	return db.NewSelect().TableExpr("resource_volume_mounts").Where("resource_volume_id = ?", volumeID).Where("archived_at IS NULL").Count(ctx)
}
func (resourceVolume) ArchiveID(ctx context.Context, db storage.Executor, id uuid.UUID, at time.Time) error {
	_, err := db.NewUpdate().Table("resource_volumes").Set("archived_at = ?", at).Set("updated_at = ?", at).Where("id = ?", id).Exec(ctx)
	return err
}
func (resourceVolumeMount) ActiveCountForResource(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (int, error) {
	return db.NewSelect().TableExpr("resource_volume_mounts AS mount").Join("JOIN resource_installations AS installation ON installation.id = mount.resource_installation_id AND installation.archived_at IS NULL").Where("installation.resource_id = ?", resourceID).Where("mount.archived_at IS NULL").Count(ctx)
}

type ResourceMountTopology struct {
	VolumeResourceID       uuid.UUID `bun:"volume_resource_id"`
	VolumeServerID         uuid.UUID `bun:"volume_server_id"`
	InstallationResourceID uuid.UUID `bun:"installation_resource_id"`
	InstallationServerID   uuid.UUID `bun:"installation_server_id"`
}

func (resourceVolumeMount) Topology(ctx context.Context, db storage.Executor, volumeID, installationID uuid.UUID) (ResourceMountTopology, error) {
	var row ResourceMountTopology
	err := db.NewSelect().TableExpr("resource_volumes AS volume").ColumnExpr("volume.resource_id AS volume_resource_id, volume.server_id AS volume_server_id, installation.resource_id AS installation_resource_id, installation.server_id AS installation_server_id").Join("JOIN resource_installations AS installation ON installation.id = ? AND installation.archived_at IS NULL", installationID).Where("volume.id = ?", volumeID).Where("volume.archived_at IS NULL").Scan(ctx, &row)
	return row, err
}
func (resourceVolumeMount) LockActive(ctx context.Context, db storage.Executor, id uuid.UUID) (ResourceVolumeMountEntity, error) {
	var row ResourceVolumeMountEntity
	err := db.NewSelect().Model(&row).Where("id = ?", id).Where("archived_at IS NULL").For("UPDATE").Scan(ctx)
	return row, err
}
func (resourceVolume) OwnedByResource(ctx context.Context, db storage.Executor, volumeID, resourceID uuid.UUID) (bool, error) {
	count, err := db.NewSelect().TableExpr("resource_volumes").Where("id = ?", volumeID).Where("resource_id = ?", resourceID).Count(ctx)
	return count == 1, err
}
func (resourceVolumeMount) LockActiveForResource(ctx context.Context, db storage.Executor, resourceID, mountID uuid.UUID) (ResourceVolumeMountEntity, error) {
	var row ResourceVolumeMountEntity
	err := db.NewSelect().Model(&row).Join("JOIN resource_volumes AS volume ON volume.id = resource_volume_mounts.resource_volume_id").Where("resource_volume_mounts.id = ?", mountID).Where("volume.resource_id = ?", resourceID).Where("resource_volume_mounts.archived_at IS NULL").For("UPDATE OF resource_volume_mounts").Scan(ctx)
	return row, err
}
func (resourceVolumeMount) ArchiveID(ctx context.Context, db storage.Executor, id uuid.UUID, at time.Time) error {
	_, err := db.NewUpdate().Table("resource_volume_mounts").Set("archived_at = ?", at).Set("updated_at = ?", at).Where("id = ?", id).Exec(ctx)
	return err
}
func (resource) TypeForID(ctx context.Context, db storage.Executor, id uuid.UUID) (ResourceTypeEnum, error) {
	var value ResourceTypeEnum
	err := db.NewSelect().TableExpr("resources").ColumnExpr("resource_type").Where("id = ?", id).Scan(ctx, &value)
	return value, err
}
func (resourceEndpoint) ActiveBelongsToResource(ctx context.Context, db storage.Executor, endpointID, resourceID uuid.UUID) (bool, error) {
	count, err := db.NewSelect().TableExpr("resource_endpoints").Where("id = ?", endpointID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").Count(ctx)
	return count == 1, err
}
func (resourceCredential) ActiveBelongsToResource(ctx context.Context, db storage.Executor, credentialID, resourceID uuid.UUID) (bool, error) {
	count, err := db.NewSelect().TableExpr("resource_credentials").Where("id = ?", credentialID).Where("resource_id = ?", resourceID).Where("archived_at IS NULL").Count(ctx)
	return count == 1, err
}
func (resourceHealthCheck) LockActiveForResource(ctx context.Context, db storage.Executor, resourceID, checkID uuid.UUID) (ResourceHealthCheckEntity, error) {
	var row ResourceHealthCheckEntity
	err := db.NewSelect().Model(&row).Where("resource_health_checks.id = ?", checkID).Where("resource_health_checks.resource_id = ?", resourceID).Where("resource_health_checks.archived_at IS NULL").For("UPDATE OF resource_health_checks").Scan(ctx)
	return row, err
}
func (resourceHealthCheck) ArchiveID(ctx context.Context, db storage.Executor, id uuid.UUID, at time.Time) error {
	_, err := db.NewUpdate().Table("resource_health_checks").Set("archived_at = ?", at).Set("updated_at = ?", at).Where("id = ?", id).Exec(ctx)
	return err
}
