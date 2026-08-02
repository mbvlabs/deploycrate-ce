package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	containerclient "deploycrate-ce/clients/container"
	nativeclient "deploycrate-ce/clients/nativeinstall"
	postgresqlclient "deploycrate-ce/clients/postgresql"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type DatabaseClusters struct {
	db        storage.Pool
	config    config.Config
	container *ContainerExecution
	servers   *ServerExecution
	native    nativeclient.Client
	postgres  postgresqlclient.Client
	resources *ResourceManagement
}

func NewDatabaseClusters(db storage.Pool, cfg config.Config, resources *ResourceManagement, container *ContainerExecution, servers *ServerExecution) *DatabaseClusters {
	return &DatabaseClusters{db: db, config: cfg, container: container, servers: servers, native: nativeclient.New(), postgres: postgresqlclient.New(), resources: resources}
}

type DatabaseClusterEndpointInput struct {
	Name             string
	Role             string
	Address          string
	Port             int32
	Protocol         string
	TLSMode          string
	PrivateNetworkID *uuid.UUID
}

type DatabaseClusterPlacementInput struct {
	ServerID       uuid.UUID
	NodeName       string
	StorageName    string
	StorageDriver  string
	StorageID      string
	DataPath       string
	ImageReference string
	ImageDigest    string
	ContainerName  string
	RestartPolicy  string
	PackageName    string
	PackageVersion string
	ServiceName    string
	ConfigPath     string
}

type CreateDatabaseClusterInput struct {
	Name                      string
	Slug                      string
	Engine                    string
	EngineVersion             string
	DesiredInstallationMethod string
	Topology                  json.RawMessage
	MaintenancePolicy         json.RawMessage
	AdministratorUsername     string
	AdministratorPassword     string
	Endpoint                  DatabaseClusterEndpointInput
	Placement                 *DatabaseClusterPlacementInput
}

type PublishDatabaseInput struct {
	Name                string
	Encoding            string
	Collation           string
	Settings            json.RawMessage
	ApplicationUsername string
	ApplicationPassword string
	ResourceName        string
	ResourceSlug        string
	SharingScope        models.ResourceSharingScopeEnum
	ClusterEndpointID   uuid.UUID
	EnvironmentGrantIDs []uuid.UUID
	ApplicationGrantIDs []uuid.UUID
}

type DatabaseClusterListItem struct {
	ID                 uuid.UUID `bun:"id" json:"id"`
	Name               string    `bun:"name" json:"name"`
	Slug               string    `bun:"slug" json:"slug"`
	Engine             string    `bun:"engine" json:"engine"`
	EngineVersion      string    `bun:"engine_version" json:"engineVersion"`
	InstallationMethod string    `bun:"installation_method" json:"installationMethod"`
	NodeCount          int       `bun:"node_count" json:"nodeCount"`
	DatabaseCount      int       `bun:"database_count" json:"databaseCount"`
	Health             string    `bun:"health" json:"health"`
}

type DatabaseClusterServerOption struct {
	ID          uuid.UUID `bun:"id" json:"id"`
	Name        string    `bun:"name" json:"name"`
	Address     string    `bun:"address" json:"address"`
	Kind        string    `bun:"kind" json:"kind"`
	IPv4Address string    `bun:"ipv4_address" json:"-"`
}

type DatabaseClusterNetworkOption struct {
	ID   uuid.UUID `bun:"id" json:"id"`
	Name string    `bun:"name" json:"name"`
}

type DatabaseClusterOptions struct {
	Servers  []DatabaseClusterServerOption  `json:"servers"`
	Networks []DatabaseClusterNetworkOption `json:"networks"`
}

func (service *DatabaseClusters) Options(ctx context.Context) (DatabaseClusterOptions, error) {
	options := DatabaseClusterOptions{Servers: make([]DatabaseClusterServerOption, 0), Networks: make([]DatabaseClusterNetworkOption, 0)}
	if err := service.db.Executor().NewSelect().TableExpr("servers").ColumnExpr("id, name, kind, address, ipv4_address").
		Where("archived_at IS NULL").Where("is_configured = TRUE").Where("kind IN ('self_hosted', 'worker')").
		Where("capabilities @> '{\"database\":true}'::jsonb").OrderExpr("name").Scan(ctx, &options.Servers); err != nil {
		return DatabaseClusterOptions{}, err
	}
	if err := service.db.Executor().NewSelect().TableExpr("private_networks").ColumnExpr("id, name").Where("archived_at IS NULL").OrderExpr("name").Scan(ctx, &options.Networks); err != nil {
		return DatabaseClusterOptions{}, err
	}
	return options, nil
}

type DatabaseClusterNodeDetail struct {
	ID                 uuid.UUID `bun:"id" json:"id"`
	Name               string    `bun:"name" json:"name"`
	Role               string    `bun:"role" json:"role"`
	DesiredState       string    `bun:"desired_state" json:"desiredState"`
	ServerID           uuid.UUID `bun:"server_id" json:"serverId"`
	ServerName         string    `bun:"server_name" json:"serverName"`
	StorageID          uuid.UUID `bun:"storage_id" json:"storageId"`
	StorageName        string    `bun:"storage_name" json:"storageName"`
	StorageDriver      string    `bun:"storage_driver" json:"storageDriver"`
	DataPath           string    `bun:"data_path" json:"dataPath"`
	InstallationID     uuid.UUID `bun:"installation_id" json:"installationId"`
	InstallationMethod string    `bun:"installation_method" json:"installationMethod"`
	ObservedState      string    `bun:"observed_state" json:"observedState"`
	ServiceState       string    `bun:"service_state" json:"serviceState"`
	Health             string    `bun:"health" json:"health"`
}

type DatabaseClusterDatabaseDetail struct {
	ID            uuid.UUID  `bun:"id" json:"id"`
	Name          string     `bun:"name" json:"name"`
	DesiredState  string     `bun:"desired_state" json:"desiredState"`
	ObservedState string     `bun:"observed_state" json:"observedState"`
	ResourceID    *uuid.UUID `bun:"resource_id" json:"resourceId"`
	ResourceName  string     `bun:"resource_name" json:"resourceName"`
}

type DatabaseClusterDetail struct {
	Cluster   models.DatabaseClusterEntity
	Endpoints []models.DatabaseClusterEndpointEntity
	Nodes     []DatabaseClusterNodeDetail
	Databases []DatabaseClusterDatabaseDetail
}

func (service *DatabaseClusters) List(ctx context.Context) ([]DatabaseClusterListItem, error) {
	items := make([]DatabaseClusterListItem, 0)
	err := service.db.Executor().NewSelect().TableExpr("database_clusters AS cluster").
		ColumnExpr("cluster.id, cluster.name, cluster.slug, cluster.engine, cluster.engine_version").
		ColumnExpr("COALESCE(cluster.desired_installation_method, '') AS installation_method").
		ColumnExpr("(SELECT count(*) FROM database_cluster_nodes node WHERE node.database_cluster_id = cluster.id AND node.archived_at IS NULL) AS node_count").
		ColumnExpr("(SELECT count(*) FROM databases database WHERE database.database_cluster_id = cluster.id AND database.archived_at IS NULL) AS database_count").
		ColumnExpr("CASE WHEN EXISTS (SELECT 1 FROM database_node_installations installation JOIN database_cluster_nodes node ON node.id = installation.database_cluster_node_id WHERE node.database_cluster_id = cluster.id AND installation.archived_at IS NULL AND installation.health = 'unhealthy') THEN 'unhealthy' WHEN EXISTS (SELECT 1 FROM database_node_installations installation JOIN database_cluster_nodes node ON node.id = installation.database_cluster_node_id WHERE node.database_cluster_id = cluster.id AND installation.archived_at IS NULL AND installation.health = 'healthy') THEN 'healthy' ELSE 'unknown' END AS health").
		Where("cluster.management_mode = ?", models.ResourceManagementManaged.String()).
		Where("cluster.archived_at IS NULL").OrderExpr("cluster.name").Scan(ctx, &items)
	return items, err
}

func (service *DatabaseClusters) Detail(ctx context.Context, clusterID uuid.UUID) (DatabaseClusterDetail, error) {
	cluster, err := models.DatabaseCluster.Find(ctx, service.db.Executor(), clusterID)
	if err != nil {
		return DatabaseClusterDetail{}, err
	}
	if cluster.ManagementMode != models.ResourceManagementManaged.String() {
		return DatabaseClusterDetail{}, models.ErrNotFound
	}
	detail := DatabaseClusterDetail{Cluster: cluster, Endpoints: make([]models.DatabaseClusterEndpointEntity, 0), Nodes: make([]DatabaseClusterNodeDetail, 0), Databases: make([]DatabaseClusterDatabaseDetail, 0)}
	queries := []func() error{
		func() error {
			return service.db.Executor().NewSelect().Model(&detail.Endpoints).Where("database_cluster_id = ?", clusterID).Where("archived_at IS NULL").OrderExpr("role, name").Scan(ctx)
		},
		func() error {
			return service.db.Executor().NewSelect().TableExpr("database_cluster_nodes AS node").
				ColumnExpr("node.id, node.name, node.role, node.desired_state, node.server_id, server.name AS server_name").
				ColumnExpr("storage.id AS storage_id, storage.name AS storage_name, storage.driver AS storage_driver, storage.data_path").
				ColumnExpr("installation.id AS installation_id, installation.installation_method, installation.observed_state, installation.service_state, installation.health").
				Join("JOIN servers AS server ON server.id = node.server_id").
				Join("LEFT JOIN database_node_storage AS storage ON storage.database_cluster_node_id = node.id AND storage.archived_at IS NULL").
				Join("LEFT JOIN database_node_installations AS installation ON installation.database_cluster_node_id = node.id AND installation.archived_at IS NULL").
				Where("node.database_cluster_id = ?", clusterID).Where("node.archived_at IS NULL").OrderExpr("node.created_at").Scan(ctx, &detail.Nodes)
		},
		func() error {
			return service.db.Executor().NewSelect().TableExpr("databases AS database").
				ColumnExpr("database.id, database.name, database.desired_state, database.observed_state, backing.resource_id, COALESCE(resource.name, '') AS resource_name").
				Join("LEFT JOIN database_resources AS backing ON backing.database_cluster_id = database.database_cluster_id").
				Join("LEFT JOIN resources AS resource ON resource.id = backing.resource_id").
				Where("database.database_cluster_id = ?", clusterID).Where("database.archived_at IS NULL").OrderExpr("database.name").Scan(ctx, &detail.Databases)
		},
	}
	for _, query := range queries {
		if err := query(); err != nil {
			return DatabaseClusterDetail{}, err
		}
	}
	return detail, nil
}

func (service *DatabaseClusters) Create(ctx context.Context, input CreateDatabaseClusterInput) (models.DatabaseClusterEntity, error) {
	input.Engine = strings.ToLower(strings.TrimSpace(input.Engine))
	input.DesiredInstallationMethod = strings.ToLower(strings.TrimSpace(input.DesiredInstallationMethod))
	if input.Engine != models.DatabaseEnginePostgreSQL {
		return models.DatabaseClusterEntity{}, domainError("engine", "unsupported", "Database Clusters currently support PostgreSQL only")
	}
	if input.Placement == nil {
		return models.DatabaseClusterEntity{}, domainError("placement", "required", "Database runtimes require a Node placement")
	}
	if err := service.requireLocalDatabaseServer(ctx, service.db.Executor(), input.Placement.ServerID); err != nil {
		return models.DatabaseClusterEntity{}, err
	}
	originAddress, err := models.ServerOriginAddress(ctx, service.db.Executor(), input.Placement.ServerID)
	if err != nil {
		return models.DatabaseClusterEntity{}, err
	}
	input.Endpoint.Address = originAddress
	desiredMethod := sql.NullString{String: input.DesiredInstallationMethod, Valid: input.DesiredInstallationMethod != ""}
	encrypted, digest, err := service.clusterCredential(input.AdministratorPassword)
	if err != nil {
		return models.DatabaseClusterEntity{}, err
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.DatabaseClusterEntity{}, err
	}
	defer tx.Rollback()
	cluster, err := models.DatabaseCluster.Create(ctx, tx, models.CreateDatabaseClusterData{
		Name: input.Name, Slug: input.Slug, Engine: input.Engine, EngineVersion: input.EngineVersion,
		ManagementMode: models.ResourceManagementManaged.String(), DesiredInstallationMethod: desiredMethod,
		Topology: normalizedJSON(input.Topology), MaintenancePolicy: normalizedJSON(input.MaintenancePolicy),
	})
	if err != nil {
		return models.DatabaseClusterEntity{}, err
	}
	if _, err := models.DatabaseClusterCredential.Create(ctx, tx, models.CreateDatabaseClusterCredentialData{
		Name: "Cluster administrator", Role: "administrator", Username: input.AdministratorUsername,
		Metadata: json.RawMessage(`{"schema_version":1}`), EncPayload: encrypted, Digest: digest, DatabaseClusterID: cluster.ID,
	}); err != nil {
		return models.DatabaseClusterEntity{}, err
	}
	if _, err := models.DatabaseClusterEndpoint.Create(ctx, tx, models.CreateDatabaseClusterEndpointData{
		Name: input.Endpoint.Name, Role: input.Endpoint.Role, Address: input.Endpoint.Address,
		Port: input.Endpoint.Port, Protocol: input.Endpoint.Protocol, TLSMode: input.Endpoint.TLSMode,
		DesiredState: "available", ObservedState: "available", Settings: json.RawMessage(`{}`),
		DatabaseClusterID: cluster.ID,
	}); err != nil {
		return models.DatabaseClusterEntity{}, err
	}
	if input.Endpoint.PrivateNetworkID != nil {
		attachments, err := tx.NewSelect().TableExpr("server_networks").
			Where("server_id = ?", input.Placement.ServerID).Where("private_network_id = ?", *input.Endpoint.PrivateNetworkID).
			Where("driver = 'wireguard'").Where("removed_at IS NULL").Where("configuration ->> 'address' = ?", originAddress).Count(ctx)
		if err != nil {
			return models.DatabaseClusterEntity{}, err
		}
		if attachments != 1 {
			return models.DatabaseClusterEntity{}, domainError("privateNetworkId", "topology", "Database Cluster Server must have the DeployCrate WireGuard attachment")
		}
		if _, err := models.DatabaseClusterEndpoint.Create(ctx, tx, models.CreateDatabaseClusterEndpointData{
			Name: "WireGuard " + input.Endpoint.Name, Role: "wireguard", Address: originAddress,
			Port: input.Endpoint.Port, Protocol: input.Endpoint.Protocol, TLSMode: input.Endpoint.TLSMode,
			DesiredState: "available", ObservedState: "available", Settings: json.RawMessage(`{}`),
			DatabaseClusterID: cluster.ID, PrivateNetworkID: input.Endpoint.PrivateNetworkID,
		}); err != nil {
			return models.DatabaseClusterEntity{}, err
		}
	}
	installationID, err := service.createNodeTopology(ctx, tx, cluster, input.Endpoint.Port, *input.Placement)
	if err != nil {
		return models.DatabaseClusterEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.DatabaseClusterEntity{}, err
	}
	if err := service.ProvisionInstallation(ctx, installationID); err != nil {
		_ = service.recordInstallationFailure(context.WithoutCancel(ctx), installationID, err)
		cleanupErr := service.ArchiveCluster(context.WithoutCancel(ctx), cluster.ID)
		return cluster, errors.Join(fmt.Errorf("provision Database Node Installation: %w", err), cleanupErr)
	}
	return cluster, nil
}

func (service *DatabaseClusters) CreateResource(ctx context.Context, clusterInput CreateDatabaseClusterInput, databaseInput PublishDatabaseInput) (models.DatabaseClusterEntity, models.ResourceEntity, error) {
	cluster, err := service.Create(ctx, clusterInput)
	if err != nil {
		return cluster, models.ResourceEntity{}, err
	}
	endpoint, err := service.primaryEndpoint(ctx, cluster.ID)
	if err == nil {
		databaseInput.ClusterEndpointID = endpoint.ID
		var resource models.ResourceEntity
		resource, err = service.PublishDatabase(ctx, cluster.ID, databaseInput)
		if err == nil {
			return cluster, resource, nil
		}
		if fieldErrors, ok := validation.As(err); ok {
			err = errors.Join(models.ErrDomainValidation, validation.WithFieldPrefix(fieldErrors, "database"))
		}
	}
	cleanupErr := service.ArchiveCluster(context.WithoutCancel(ctx), cluster.ID)
	return cluster, models.ResourceEntity{}, errors.Join(err, cleanupErr)
}

func (service *DatabaseClusters) createNodeTopology(ctx context.Context, db storage.Executor, cluster models.DatabaseClusterEntity, hostPort int32, placement DatabaseClusterPlacementInput) (uuid.UUID, error) {
	if err := service.requireLocalDatabaseServer(ctx, db, placement.ServerID); err != nil {
		return uuid.Nil, err
	}
	node, err := models.DatabaseClusterNode.Create(ctx, db, models.CreateDatabaseClusterNodeData{
		Name: placement.NodeName, Role: "primary", DesiredState: "running", DatabaseClusterID: cluster.ID, ServerID: placement.ServerID,
	})
	if err != nil {
		return uuid.Nil, err
	}
	storageRecord, err := models.DatabaseNodeStorage.Create(ctx, db, models.CreateDatabaseNodeStorageData{
		Name: placement.StorageName, Driver: placement.StorageDriver,
		ExternalID: nullableString(placement.StorageID), DataPath: placement.DataPath, Configuration: json.RawMessage(`{}`),
		DatabaseClusterNodeID: node.ID, ServerID: placement.ServerID,
	})
	if err != nil {
		return uuid.Nil, err
	}
	installation, err := models.DatabaseNodeInstallation.Create(ctx, db, models.CreateDatabaseNodeInstallationData{
		InstallationMethod: cluster.DesiredInstallationMethod.String, DesiredState: "running", ObservedState: "pending",
		ServiceState: "pending", Health: "unknown", DatabaseClusterNodeID: node.ID,
		ServerID: placement.ServerID, DatabaseNodeStorageID: storageRecord.ID,
	})
	if err != nil {
		return uuid.Nil, err
	}
	if cluster.DesiredInstallationMethod.String == models.DatabaseInstallDocker {
		_, err = models.DockerDatabaseInstallation.Create(ctx, db, models.CreateDockerDatabaseNodeInstallationData{
			DatabaseNodeInstallationID: installation.ID, ImageReference: placement.ImageReference,
			ImageDigest: nullableString(placement.ImageDigest), ContainerName: placement.ContainerName,
			RestartPolicy: placement.RestartPolicy,
			PortMappings:  json.RawMessage(fmt.Sprintf(`[{"hostPort":%d,"containerPort":%d,"protocol":"tcp"}]`, hostPort, defaultDatabasePort(cluster.Engine))),
			Configuration: json.RawMessage(`{"mount_path":"/var/lib/database"}`),
		})
	} else {
		_, err = models.NativeDatabaseInstallation.Create(ctx, db, models.CreateNativeDatabaseNodeInstallationData{
			DatabaseNodeInstallationID: installation.ID, PackageName: placement.PackageName,
			RequestedPackageVersion: nullableString(placement.PackageVersion), ServiceName: placement.ServiceName,
			ConfigurationPath: placement.ConfigPath, Settings: json.RawMessage(`{}`),
		})
	}
	return installation.ID, err
}

func (service *DatabaseClusters) ProvisionInstallation(ctx context.Context, installationID uuid.UUID) error {
	topology, err := service.loadInstallationTopology(ctx, installationID)
	if err != nil {
		return err
	}
	if err := service.runInstallationRuntime(ctx, databaseRuntime{InstallationID: installationID, Topology: topology}); err != nil {
		return err
	}
	connection, err := service.postgreSQLAdministratorConnection(ctx, topology.ClusterID)
	if err != nil {
		return err
	}
	if err := service.postgres.WaitForReady(ctx, connection, time.Minute); err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = service.db.Executor().NewUpdate().TableExpr("database_node_installations").
		Set("observed_state = 'running'").Set("service_state = 'running'").Set("health = 'healthy'").
		Set("installed_version = ?", topology.EngineVersion).Set("observed_at = ?", now).Set("updated_at = ?", now).
		Where("id = ?", installationID).Exec(ctx)
	return err
}

type databaseInstallationTopology struct {
	ClusterID         uuid.UUID `bun:"cluster_id"`
	ServerID          uuid.UUID `bun:"server_id"`
	Engine            string    `bun:"engine"`
	EngineVersion     string    `bun:"engine_version"`
	Method            string    `bun:"method"`
	StorageExternalID string    `bun:"storage_external_id"`
	DataPath          string    `bun:"data_path"`
	ImageReference    string    `bun:"image_reference"`
	ContainerName     string    `bun:"container_name"`
	RestartPolicy     string    `bun:"restart_policy"`
	PackageName       string    `bun:"package_name"`
	PackageVersion    string    `bun:"package_version"`
	ServiceName       string    `bun:"service_name"`
	ConfigPath        string    `bun:"config_path"`
	Port              int32     `bun:"port"`
}

func (service *DatabaseClusters) loadInstallationTopology(ctx context.Context, installationID uuid.UUID) (databaseInstallationTopology, error) {
	var topology databaseInstallationTopology
	err := service.db.Executor().NewSelect().TableExpr("database_node_installations AS installation").
		ColumnExpr("cluster.id AS cluster_id, installation.server_id, cluster.engine, cluster.engine_version, installation.installation_method AS method").
		ColumnExpr("COALESCE(storage.external_id, '') AS storage_external_id, storage.data_path").
		ColumnExpr("COALESCE(docker.image_reference, '') AS image_reference, COALESCE(docker.container_name, '') AS container_name, COALESCE(docker.restart_policy, '') AS restart_policy").
		ColumnExpr("COALESCE(native.package_name, '') AS package_name, COALESCE(native.requested_package_version, '') AS package_version, COALESCE(native.service_name, '') AS service_name, COALESCE(native.configuration_path, '') AS config_path").
		ColumnExpr("endpoint.port").
		Join("JOIN database_cluster_nodes AS node ON node.id = installation.database_cluster_node_id AND node.archived_at IS NULL").
		Join("JOIN database_clusters AS cluster ON cluster.id = node.database_cluster_id AND cluster.archived_at IS NULL").
		Join("JOIN database_node_storage AS storage ON storage.id = installation.database_node_storage_id AND storage.archived_at IS NULL").
		Join("JOIN database_cluster_endpoints AS endpoint ON endpoint.database_cluster_id = cluster.id AND endpoint.role = 'primary' AND endpoint.archived_at IS NULL").
		Join("LEFT JOIN docker_database_node_installations AS docker ON docker.database_node_installation_id = installation.id").
		Join("LEFT JOIN native_database_node_installations AS native ON native.database_node_installation_id = installation.id").
		Where("installation.id = ?", installationID).Where("installation.archived_at IS NULL").Scan(ctx, &topology)
	return topology, err
}

type databaseRuntime struct {
	InstallationID uuid.UUID
	Topology       databaseInstallationTopology
}

func (service *DatabaseClusters) requireLocalDatabaseServer(ctx context.Context, db storage.Executor, serverID uuid.UUID) error {
	_, err := models.RequireServerCapability(ctx, db, serverID, models.ServerCapabilityDatabase)
	return err
}

func (service *DatabaseClusters) activeClusterRuntimes(ctx context.Context, clusterID uuid.UUID) ([]databaseRuntime, error) {
	installationIDs := make([]uuid.UUID, 0)
	err := service.db.Executor().NewSelect().TableExpr("database_node_installations AS installation").ColumnExpr("installation.id").
		Join("JOIN database_cluster_nodes AS node ON node.id = installation.database_cluster_node_id AND node.archived_at IS NULL").
		Where("node.database_cluster_id = ?", clusterID).Where("installation.archived_at IS NULL").OrderExpr("installation.created_at").Scan(ctx, &installationIDs)
	if err != nil {
		return nil, err
	}
	runtimes := make([]databaseRuntime, 0, len(installationIDs))
	for _, installationID := range installationIDs {
		topology, err := service.loadInstallationTopology(ctx, installationID)
		if err != nil {
			return nil, err
		}
		runtimes = append(runtimes, databaseRuntime{InstallationID: installationID, Topology: topology})
	}
	return runtimes, nil
}

func (service *DatabaseClusters) runInstallationRuntime(ctx context.Context, runtime databaseRuntime) error {
	topology := runtime.Topology
	if topology.Engine != models.DatabaseEnginePostgreSQL {
		return domainError("engine", "unsupported", "Database Cluster runtime provisioning currently supports PostgreSQL only")
	}
	if err := service.requireLocalDatabaseServer(ctx, service.db.Executor(), topology.ServerID); err != nil {
		return err
	}
	administrator, password, err := service.administrator(ctx, topology.ClusterID)
	if err != nil {
		return err
	}
	if topology.Method == models.DatabaseInstallDocker {
		return service.container.Run(ctx, topology.ServerID, models.ServerCapabilityDatabase, containerclient.RunSpec{
			InstallationID: runtime.InstallationID.String(), ContainerName: topology.ContainerName,
			ImageReference: topology.ImageReference, RestartPolicy: topology.RestartPolicy,
			PortMappings: []containerclient.PortMapping{{HostPort: topology.Port, ContainerPort: defaultDatabasePort(topology.Engine), Protocol: "tcp"}},
			VolumeMounts: []containerclient.VolumeMount{{Name: topology.StorageExternalID, MountPath: topology.DataPath}},
			Environment:  map[string]string{"POSTGRES_USER": administrator.Username, "POSTGRES_PASSWORD": password, "POSTGRES_DB": "postgres"},
		})
	}
	if topology.Method != models.DatabaseInstallNative {
		return domainError("installationMethod", "unsupported", "Database Node Installation method is unsupported")
	}
	target, err := service.servers.Target(ctx, topology.ServerID, models.ServerCapabilityDatabase)
	if err != nil {
		return err
	}
	if target.Remote {
		return domainError("installationMethod", "worker_native", "native Database installs are available only on the control plane; choose Docker for a database-capable worker")
	}
	return service.native.Install(ctx, nativeclient.InstallSpec{
		InstallationID: runtime.InstallationID.String(), Engine: topology.Engine, EngineVersion: topology.EngineVersion,
		PackageName: topology.PackageName, PackageVersion: topology.PackageVersion, ServiceName: topology.ServiceName,
		ConfigPath: topology.ConfigPath, DataPath: topology.DataPath, Port: topology.Port,
		AdministratorUsername: administrator.Username, AdministratorPassword: password, Settings: json.RawMessage(`{}`),
	})
}

func (service *DatabaseClusters) stopInstallationRuntime(ctx context.Context, runtime databaseRuntime) error {
	if err := service.requireLocalDatabaseServer(ctx, service.db.Executor(), runtime.Topology.ServerID); err != nil {
		return err
	}
	if runtime.Topology.Method == models.DatabaseInstallDocker {
		state, err := service.container.Inspect(ctx, runtime.Topology.ServerID, models.ServerCapabilityDatabase, runtime.InstallationID.String(), runtime.Topology.ContainerName)
		if err != nil || !state.Exists {
			return err
		}
		return service.container.Remove(ctx, runtime.Topology.ServerID, models.ServerCapabilityDatabase, runtime.InstallationID.String(), runtime.Topology.ContainerName)
	}
	if runtime.Topology.Method == models.DatabaseInstallNative {
		target, targetErr := service.servers.Target(ctx, runtime.Topology.ServerID, models.ServerCapabilityDatabase)
		if targetErr != nil {
			return targetErr
		}
		if target.Remote {
			return domainError("installationMethod", "worker_native", "native Database installs are available only on the control plane")
		}
		state, err := service.native.Inspect(ctx, runtime.InstallationID.String(), runtime.Topology.PackageName, runtime.Topology.ServiceName)
		if err != nil || !state.Running {
			return err
		}
		return service.native.Stop(ctx, runtime.InstallationID.String(), runtime.Topology.ServiceName)
	}
	return domainError("installationMethod", "unsupported", "Database Node Installation method is unsupported")
}

func (service *DatabaseClusters) restoreInstallationRuntimes(ctx context.Context, runtimes []databaseRuntime) error {
	errorsToJoin := make([]error, 0)
	for _, runtime := range runtimes {
		var err error
		if runtime.Topology.Method == models.DatabaseInstallNative {
			err = service.native.Start(ctx, runtime.InstallationID.String(), runtime.Topology.ServiceName)
		} else {
			err = service.runInstallationRuntime(ctx, runtime)
		}
		if err != nil {
			errorsToJoin = append(errorsToJoin, fmt.Errorf("restore Database Node Installation %s: %w", runtime.InstallationID, err))
			continue
		}
		now := time.Now().UTC()
		if _, err := service.db.Executor().NewUpdate().TableExpr("database_node_installations").
			Set("observed_state = 'running'").Set("service_state = 'running'").Set("health = 'healthy'").Set("reason = NULL").
			Set("observed_at = ?", now).Set("updated_at = ?", now).Where("id = ?", runtime.InstallationID).Exec(ctx); err != nil {
			errorsToJoin = append(errorsToJoin, fmt.Errorf("record restored Database Node Installation %s: %w", runtime.InstallationID, err))
		}
	}
	return errors.Join(errorsToJoin...)
}

func (service *DatabaseClusters) PublishDatabase(ctx context.Context, clusterID uuid.UUID, input PublishDatabaseInput) (models.ResourceEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	defer tx.Rollback()
	cluster, err := models.DatabaseCluster.FindForUpdate(ctx, tx, clusterID)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	if cluster.Engine != models.DatabaseEnginePostgreSQL {
		return models.ResourceEntity{}, domainError("engine", "unsupported", "publishing Databases currently supports PostgreSQL only")
	}
	if cluster.ManagementMode != models.ResourceManagementManaged.String() {
		return models.ResourceEntity{}, models.ErrNotFound
	}
	connection, err := service.postgreSQLAdministratorConnection(ctx, cluster.ID)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	if strings.EqualFold(strings.TrimSpace(input.ApplicationUsername), strings.TrimSpace(connection.Username)) {
		return models.ResourceEntity{}, domainError("applicationUsername", "administrator", "Application username must be different from the Database Cluster administrator")
	}
	var endpoint models.DatabaseClusterEndpointEntity
	if err := tx.NewSelect().Model(&endpoint).Where("id = ?", input.ClusterEndpointID).Where("database_cluster_id = ?", clusterID).Where("archived_at IS NULL").Scan(ctx); err != nil {
		return models.ResourceEntity{}, err
	}
	resource, err := models.Resource.Create(ctx, tx, models.CreateResourceData{
		Name: input.ResourceName, Slug: input.ResourceSlug, Kind: cluster.Engine,
		ManagementMode: models.ResourceManagementManaged, SharingScope: input.SharingScope,
	})
	if err != nil {
		return models.ResourceEntity{}, databaseResourceValidation(err)
	}
	if _, err := models.DatabaseResource.Create(ctx, tx, resource.ID, cluster.ID); err != nil {
		return models.ResourceEntity{}, err
	}
	database, err := models.Database.Create(ctx, tx, models.CreateDatabaseData{
		Name: input.Name, Encoding: nullableString(input.Encoding), Collation: nullableString(input.Collation),
		Settings: normalizedJSON(input.Settings), DesiredState: "provisioned", ObservedState: "provisioned", DatabaseClusterID: cluster.ID,
	})
	if err != nil {
		return models.ResourceEntity{}, err
	}
	credential, err := service.resources.createCredential(ctx, tx, resource, ResourceCredentialInput{
		Name: "Application user", Username: input.ApplicationUsername,
		Metadata:     json.RawMessage(`{"schema_version":1,"credential_kind":"database_user"}`),
		SecretValues: map[string]string{"password": input.ApplicationPassword},
	})
	if err != nil {
		return models.ResourceEntity{}, err
	}
	published, err := models.ResourceEndpoint.Create(ctx, tx, models.CreateResourceEndpointData{
		Name: database.Name + " " + endpoint.Name, Role: "primary", Address: endpoint.Address, Port: endpoint.Port,
		Protocol: endpoint.Protocol, TlsMode: endpoint.TLSMode,
		Settings: json.RawMessage(fmt.Sprintf(`{"database":%q}`, database.Name)), ResourceID: resource.ID,
		PrivateNetworkID: endpoint.PrivateNetworkID,
	})
	if err != nil {
		return models.ResourceEntity{}, err
	}
	if _, err := models.DatabaseResourceEndpoint.Create(ctx, tx, published.ID, endpoint.ID); err != nil {
		return models.ResourceEntity{}, err
	}
	if _, err := models.ResourceHealthCheck.Create(ctx, tx, models.CreateResourceHealthCheckData{
		Name: "PostgreSQL readiness", Kind: "postgresql", Configuration: json.RawMessage(`{}`),
		IntervalSeconds: 15, TimeoutSeconds: 3, FailureThreshold: 3, SuccessThreshold: 1, Enabled: true,
		ResourceID: resource.ID, ResourceEndpointID: &published.ID, ResourceCredentialID: &credential.ID,
	}); err != nil {
		return models.ResourceEntity{}, err
	}
	for _, environmentID := range input.EnvironmentGrantIDs {
		if _, err := models.ResourceEnvironmentGrant.Create(ctx, tx, resource.ID, environmentID); err != nil {
			return models.ResourceEntity{}, err
		}
	}
	for _, applicationID := range input.ApplicationGrantIDs {
		if _, err := models.ResourceApplicationGrant.Create(ctx, tx, resource.ID, applicationID); err != nil {
			return models.ResourceEntity{}, err
		}
	}
	createdInEngine, err := service.postgres.CreateDatabase(ctx, connection, input.Name)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	if err := service.resources.reconcilePostgreSQLCredential(ctx, tx, resource, credential, nil); err != nil {
		if createdInEngine {
			return models.ResourceEntity{}, errors.Join(err, service.postgres.DropDatabase(context.WithoutCancel(ctx), connection, database.Name))
		}
		return models.ResourceEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		if createdInEngine {
			cleanupErr := service.postgres.DropDatabase(context.WithoutCancel(ctx), connection, input.Name)
			return models.ResourceEntity{}, errors.Join(err, cleanupErr)
		}
		return models.ResourceEntity{}, err
	}
	return resource, nil
}

func (service *DatabaseClusters) CreateDatabaseForResource(ctx context.Context, resourceID uuid.UUID, input PublishDatabaseInput) (models.DatabaseEntity, error) {
	backing, err := models.DatabaseResource.FindByResource(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return models.DatabaseEntity{}, err
	}
	connection, err := service.postgreSQLAdministratorConnection(ctx, backing.DatabaseClusterID)
	if err != nil {
		return models.DatabaseEntity{}, err
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.DatabaseEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.resources.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.DatabaseEntity{}, err
	}
	if resource.ArchivedAt.Valid || resource.Kind != models.DatabaseEnginePostgreSQL || resource.ManagementMode != models.ResourceManagementManaged {
		return models.DatabaseEntity{}, models.ErrNotFound
	}
	cluster, err := models.DatabaseCluster.FindForUpdate(ctx, tx, backing.DatabaseClusterID)
	if err != nil {
		return models.DatabaseEntity{}, err
	}
	if cluster.ArchivedAt.Valid || cluster.Engine != resource.Kind {
		return models.DatabaseEntity{}, models.ErrNotFound
	}
	var clusterEndpoint models.DatabaseClusterEndpointEntity
	if err := tx.NewSelect().Model(&clusterEndpoint).Where("database_cluster_id = ?", cluster.ID).
		Where("role = 'primary'").Where("archived_at IS NULL").OrderExpr("created_at").Limit(1).Scan(ctx); err != nil {
		return models.DatabaseEntity{}, err
	}
	database, err := models.Database.Create(ctx, tx, models.CreateDatabaseData{
		Name: input.Name, Encoding: nullableString(input.Encoding), Collation: nullableString(input.Collation),
		Settings: normalizedJSON(input.Settings), DesiredState: "provisioned", ObservedState: "provisioned", DatabaseClusterID: cluster.ID,
	})
	if err != nil {
		return models.DatabaseEntity{}, err
	}
	published, err := models.ResourceEndpoint.Create(ctx, tx, models.CreateResourceEndpointData{
		Name: database.Name + " " + clusterEndpoint.Name, Role: "primary",
		Address: clusterEndpoint.Address, Port: clusterEndpoint.Port, Protocol: clusterEndpoint.Protocol, TlsMode: clusterEndpoint.TLSMode,
		Settings: json.RawMessage(fmt.Sprintf(`{"database":%q}`, database.Name)), ResourceID: resource.ID,
		PrivateNetworkID: clusterEndpoint.PrivateNetworkID,
	})
	if err != nil {
		return models.DatabaseEntity{}, err
	}
	if _, err := models.DatabaseResourceEndpoint.Create(ctx, tx, published.ID, clusterEndpoint.ID); err != nil {
		return models.DatabaseEntity{}, err
	}
	privateClusterEndpoints := make([]models.DatabaseClusterEndpointEntity, 0)
	if err := tx.NewSelect().Model(&privateClusterEndpoints).Distinct().
		Join("JOIN database_resource_endpoints AS link ON link.database_cluster_endpoint_id = database_cluster_endpoint.id").
		Join("JOIN resource_endpoints AS endpoint ON endpoint.id = link.resource_endpoint_id AND endpoint.resource_id = ? AND endpoint.private_network_id IS NOT NULL AND endpoint.archived_at IS NULL", resource.ID).
		Where("database_cluster_endpoint.database_cluster_id = ?", cluster.ID).
		Where("database_cluster_endpoint.private_network_id IS NOT NULL").Where("database_cluster_endpoint.archived_at IS NULL").Scan(ctx); err != nil {
		return models.DatabaseEntity{}, err
	}
	for _, privateClusterEndpoint := range privateClusterEndpoints {
		privatePublished, err := models.ResourceEndpoint.Create(ctx, tx, models.CreateResourceEndpointData{
			Name: database.Name + " " + privateClusterEndpoint.Name, Role: "wireguard",
			Address: privateClusterEndpoint.Address, Port: privateClusterEndpoint.Port,
			Protocol: privateClusterEndpoint.Protocol, TlsMode: privateClusterEndpoint.TLSMode,
			Settings: json.RawMessage(fmt.Sprintf(`{"database":%q}`, database.Name)), ResourceID: resource.ID,
			PrivateNetworkID: privateClusterEndpoint.PrivateNetworkID,
		})
		if err != nil {
			return models.DatabaseEntity{}, err
		}
		if _, err := models.DatabaseResourceEndpoint.Create(ctx, tx, privatePublished.ID, privateClusterEndpoint.ID); err != nil {
			return models.DatabaseEntity{}, err
		}
	}
	createdInEngine, err := service.postgres.CreateDatabase(ctx, connection, database.Name)
	if err != nil {
		return models.DatabaseEntity{}, err
	}
	credentials := make([]models.ResourceCredentialEntity, 0)
	if err := tx.NewSelect().Model(&credentials).Where("resource_id = ?", resource.ID).Where("archived_at IS NULL").OrderExpr("created_at").Scan(ctx); err != nil {
		if createdInEngine {
			return models.DatabaseEntity{}, errors.Join(err, service.postgres.DropDatabase(context.WithoutCancel(ctx), connection, database.Name))
		}
		return models.DatabaseEntity{}, err
	}
	for _, credential := range credentials {
		if err := service.resources.reconcilePostgreSQLCredentialInDatabase(ctx, tx, resource, credential, database.Name); err != nil {
			if createdInEngine {
				return models.DatabaseEntity{}, errors.Join(err, service.postgres.DropDatabase(context.WithoutCancel(ctx), connection, database.Name))
			}
			return models.DatabaseEntity{}, err
		}
	}
	if len(credentials) > 0 {
		if _, err := models.ResourceHealthCheck.Create(ctx, tx, models.CreateResourceHealthCheckData{
			Name: database.Name + " readiness", Kind: "postgresql", Configuration: json.RawMessage(`{}`),
			IntervalSeconds: 15, TimeoutSeconds: 3, FailureThreshold: 3, SuccessThreshold: 1, Enabled: true,
			ResourceID: resource.ID, ResourceEndpointID: &published.ID, ResourceCredentialID: &credentials[0].ID,
		}); err != nil {
			if createdInEngine {
				return models.DatabaseEntity{}, errors.Join(err, service.postgres.DropDatabase(context.WithoutCancel(ctx), connection, database.Name))
			}
			return models.DatabaseEntity{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		if createdInEngine {
			return models.DatabaseEntity{}, errors.Join(err, service.postgres.DropDatabase(context.WithoutCancel(ctx), connection, database.Name))
		}
		return models.DatabaseEntity{}, err
	}
	return database, nil
}

func databaseResourceValidation(err error) error {
	fieldErrors, ok := validation.As(err)
	if !ok {
		return err
	}
	mapped := make(validation.ValidationErrors, 0, len(fieldErrors))
	for _, fieldErr := range fieldErrors {
		switch fieldErr.Field {
		case "name":
			fieldErr.Field = "resourceName"
		case "slug":
			fieldErr.Field = "resourceSlug"
		}
		mapped = append(mapped, fieldErr)
	}
	return errors.Join(models.ErrDomainValidation, mapped)
}

func (service *DatabaseClusters) DeprovisionDatabase(ctx context.Context, clusterID, databaseID uuid.UUID) error {
	database, err := models.Database.Find(ctx, service.db.Executor(), databaseID)
	if err != nil {
		return err
	}
	if database.DatabaseClusterID != clusterID || database.ArchivedAt.Valid {
		return models.ErrNotFound
	}
	var blockers int
	err = service.db.Executor().NewSelect().TableExpr("databases AS database").ColumnExpr("count(*)").
		Join("LEFT JOIN database_resources AS backing ON backing.database_cluster_id = database.database_cluster_id").
		Where("database.id = ?", databaseID).
		Where(`EXISTS (SELECT 1 FROM resources resource WHERE resource.id = backing.resource_id AND resource.archived_at IS NULL)
			OR EXISTS (SELECT 1 FROM environment_resources connection WHERE connection.resource_id = backing.resource_id AND connection.archived_at IS NULL)
			OR EXISTS (SELECT 1 FROM database_restores restore WHERE restore.database_id = database.id AND restore.status IN ('pending','safety_backup','restoring'))`).Scan(ctx, &blockers)
	if err != nil {
		return err
	}
	if blockers != 0 {
		return domainError("database", "dependency", "archive the Resource, disconnect Environments, and finish active restores before deprovisioning the Database")
	}
	cluster, err := models.DatabaseCluster.Find(ctx, service.db.Executor(), clusterID)
	if err != nil {
		return err
	}
	if cluster.Engine != models.DatabaseEnginePostgreSQL {
		return domainError("engine", "unsupported", "deprovisioning Databases currently supports PostgreSQL only")
	}
	if cluster.ManagementMode != models.ResourceManagementManaged.String() {
		return models.ErrNotFound
	}
	connection, err := service.postgreSQLAdministratorConnection(ctx, cluster.ID)
	if err != nil {
		return err
	}
	if err := service.postgres.DropDatabase(ctx, connection, database.Name); err != nil {
		return err
	}
	now := time.Now().UTC()
	database.DesiredState, database.ObservedState, database.ArchivedAt = "deprovisioned", "deprovisioned", sql.NullTime{Time: now, Valid: true}
	_, err = models.Database.Update(ctx, service.db.Executor(), database)
	return err
}

func (service *DatabaseClusters) ArchiveDatabaseResource(ctx context.Context, resourceID uuid.UUID) (bool, error) {
	var target struct {
		ClusterID        uuid.UUID    `bun:"cluster_id"`
		ManagementMode   string       `bun:"management_mode"`
		ResourceArchived sql.NullTime `bun:"resource_archived"`
		ClusterArchived  sql.NullTime `bun:"cluster_archived"`
	}
	err := service.db.Executor().NewSelect().TableExpr("database_resources AS backing").
		ColumnExpr("cluster.id AS cluster_id, cluster.management_mode").
		ColumnExpr("resource.archived_at AS resource_archived, cluster.archived_at AS cluster_archived").
		Join("JOIN resources AS resource ON resource.id = backing.resource_id").
		Join("JOIN database_clusters AS cluster ON cluster.id = backing.database_cluster_id").
		Where("backing.resource_id = ?", resourceID).Scan(ctx, &target)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if target.ManagementMode != models.ResourceManagementManaged.String() {
		return false, nil
	}
	if !target.ResourceArchived.Valid {
		if err := service.resources.ArchiveResource(ctx, resourceID); err != nil {
			return true, err
		}
	}
	databases := make([]models.DatabaseEntity, 0)
	if err := service.db.Executor().NewSelect().Model(&databases).Where("database_cluster_id = ?", target.ClusterID).Where("archived_at IS NULL").OrderExpr("created_at").Scan(ctx); err != nil {
		return true, err
	}
	for _, database := range databases {
		now := time.Now().UTC()
		if _, err := service.db.Executor().NewUpdate().TableExpr("backup_policies").Set("activated_at = NULL").Set("archived_at = ?", now).Set("updated_at = ?", now).
			Where("database_id = ?", database.ID).Where("archived_at IS NULL").Exec(ctx); err != nil {
			return true, err
		}
		if err := service.DeprovisionDatabase(ctx, target.ClusterID, database.ID); err != nil {
			return true, err
		}
	}
	if !target.ClusterArchived.Valid {
		if err := service.ArchiveCluster(ctx, target.ClusterID); err != nil {
			return true, err
		}
	}
	return true, nil
}

func (service *DatabaseClusters) ArchiveCluster(ctx context.Context, clusterID uuid.UUID) (err error) {
	cluster, err := models.DatabaseCluster.Find(ctx, service.db.Executor(), clusterID)
	if err != nil {
		return err
	}
	if cluster.ManagementMode != models.ResourceManagementManaged.String() {
		return models.ErrNotFound
	}
	var activeDatabases int
	if activeDatabases, err = service.db.Executor().NewSelect().TableExpr("databases").Where("database_cluster_id = ?", clusterID).Where("archived_at IS NULL").Count(ctx); err != nil {
		return err
	}
	if activeDatabases != 0 {
		return domainError("databaseCluster", "dependency", "deprovision every Database before archiving the Cluster")
	}
	runtimes, err := service.activeClusterRuntimes(ctx, clusterID)
	if err != nil {
		return err
	}
	stopped := make([]databaseRuntime, 0, len(runtimes))
	for _, runtime := range runtimes {
		if stopErr := service.stopInstallationRuntime(ctx, runtime); stopErr != nil {
			restoreErr := service.restoreInstallationRuntimes(context.WithoutCancel(ctx), stopped)
			return errors.Join(stopErr, restoreErr)
		}
		stopped = append(stopped, runtime)
	}
	restoreRuntimes := len(stopped) > 0
	defer func() {
		if restoreRuntimes {
			err = errors.Join(err, service.restoreInstallationRuntimes(context.WithoutCancel(ctx), stopped))
		}
	}()

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cluster, err = models.DatabaseCluster.FindForUpdate(ctx, tx, cluster.ID)
	if err != nil {
		return err
	}
	if activeDatabases, err = tx.NewSelect().TableExpr("databases").Where("database_cluster_id = ?", clusterID).Where("archived_at IS NULL").Count(ctx); err != nil {
		return err
	}
	if activeDatabases != 0 {
		return domainError("databaseCluster", "dependency", "deprovision every Database before archiving the Cluster")
	}
	now := time.Now().UTC()
	if _, err = tx.NewUpdate().TableExpr("database_node_installations AS installation").
		Set("archived_at = ?", now).Set("desired_state = 'stopped'").Set("observed_state = 'stopped'").Set("service_state = 'stopped'").Set("updated_at = ?", now).
		Where("EXISTS (SELECT 1 FROM database_cluster_nodes node WHERE node.id = installation.database_cluster_node_id AND node.database_cluster_id = ?)", clusterID).
		Where("installation.archived_at IS NULL").Exec(ctx); err != nil {
		return err
	}
	if _, err = tx.NewUpdate().TableExpr("database_node_storage AS storage").Set("archived_at = ?", now).Set("updated_at = ?", now).
		Where("EXISTS (SELECT 1 FROM database_cluster_nodes node WHERE node.id = storage.database_cluster_node_id AND node.database_cluster_id = ?)", clusterID).
		Where("storage.archived_at IS NULL").Exec(ctx); err != nil {
		return err
	}
	if _, err = tx.NewUpdate().TableExpr("database_cluster_nodes").Set("desired_state = 'retired'").Set("archived_at = ?", now).Set("updated_at = ?", now).
		Where("database_cluster_id = ?", clusterID).Where("archived_at IS NULL").Exec(ctx); err != nil {
		return err
	}
	for _, table := range []string{"database_cluster_endpoints", "database_cluster_credentials"} {
		if _, err = tx.NewUpdate().TableExpr(table).Set("archived_at = ?", now).Set("updated_at = ?", now).
			Where("database_cluster_id = ?", clusterID).Where("archived_at IS NULL").Exec(ctx); err != nil {
			return err
		}
	}
	cluster.ArchivedAt = sql.NullTime{Time: now, Valid: true}
	if _, err = models.DatabaseCluster.Update(ctx, tx, cluster); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	restoreRuntimes = false
	return nil
}

func (service *DatabaseClusters) clusterCredential(password string) ([]byte, []byte, error) {
	if password == "" {
		return nil, nil, domainError("administratorPassword", "required", "Cluster administrator password is required")
	}
	payload, err := json.Marshal(map[string]any{"schema_version": 1, "values": map[string]string{"password": password}})
	if err != nil {
		return nil, nil, err
	}
	encrypted, err := secretcrypto.EncryptForPurpose(payload, service.config.App.SessionEncryptionKey, databaseClusterCredentialPurpose)
	if err != nil {
		return nil, nil, err
	}
	key, err := hex.DecodeString(service.config.App.SessionEncryptionKey)
	if err != nil || len(key) != 32 {
		return nil, nil, errors.New("Database Cluster credential digest key is invalid")
	}
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write(payload)
	return encrypted, digest.Sum(nil), nil
}

func (service *DatabaseClusters) administrator(ctx context.Context, clusterID uuid.UUID) (models.DatabaseClusterCredentialEntity, string, error) {
	credential, err := models.DatabaseClusterCredential.ActiveAdministrator(ctx, service.db.Executor(), clusterID)
	if err != nil {
		return models.DatabaseClusterCredentialEntity{}, "", err
	}
	plaintext, err := secretcrypto.DecryptForPurpose(credential.EncPayload, service.config.App.SessionEncryptionKey, databaseClusterCredentialPurpose)
	if err != nil {
		return models.DatabaseClusterCredentialEntity{}, "", err
	}
	var payload struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}
	if json.Unmarshal(plaintext, &payload) != nil || payload.SchemaVersion != 1 || payload.Values["password"] == "" {
		return models.DatabaseClusterCredentialEntity{}, "", errors.New("Database Cluster administrator credential is invalid")
	}
	return credential, payload.Values["password"], nil
}

func (service *DatabaseClusters) postgreSQLAdministratorConnection(ctx context.Context, clusterID uuid.UUID) (postgresqlclient.Connection, error) {
	credential, password, err := service.administrator(ctx, clusterID)
	if err != nil {
		return postgresqlclient.Connection{}, err
	}
	endpoint, err := service.primaryEndpoint(ctx, clusterID)
	if err != nil {
		return postgresqlclient.Connection{}, err
	}
	return postgresqlclient.Connection{Host: endpoint.Address, Port: endpoint.Port, Username: credential.Username, Password: password}, nil
}

func (service *DatabaseClusters) primaryEndpoint(ctx context.Context, clusterID uuid.UUID) (models.DatabaseClusterEndpointEntity, error) {
	var endpoint models.DatabaseClusterEndpointEntity
	err := service.db.Executor().NewSelect().Model(&endpoint).Where("database_cluster_id = ?", clusterID).Where("role = 'primary'").Where("archived_at IS NULL").Scan(ctx)
	return endpoint, err
}

func (service *DatabaseClusters) loadClusterEndpoint(ctx context.Context, clusterID, endpointID uuid.UUID) (models.DatabaseClusterEndpointEntity, error) {
	var endpoint models.DatabaseClusterEndpointEntity
	err := service.db.Executor().NewSelect().Model(&endpoint).Where("id = ?", endpointID).Where("database_cluster_id = ?", clusterID).Where("archived_at IS NULL").Scan(ctx)
	return endpoint, err
}

func (service *DatabaseClusters) recordInstallationFailure(ctx context.Context, installationID uuid.UUID, cause error) error {
	now := time.Now().UTC()
	_, err := service.db.Executor().NewUpdate().TableExpr("database_node_installations").Set("observed_state = 'failed'").Set("service_state = 'failed'").Set("health = 'unhealthy'").Set("reason = ?", cause.Error()).Set("observed_at = ?", now).Set("updated_at = ?", now).Where("id = ?", installationID).Exec(ctx)
	return err
}

func defaultDatabasePort(engine string) int32 {
	if engine == models.DatabaseEngineMySQL {
		return 3306
	}
	return 5432
}
