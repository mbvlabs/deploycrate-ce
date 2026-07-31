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

const (
	DatabaseEnginePostgreSQL = "postgresql"
	DatabaseEngineMySQL      = "mysql"
	DatabaseInstallDocker    = "docker"
	DatabaseInstallNative    = "native"
	DatabaseSharingDedicated = "dedicated"
	DatabaseSharingShared    = "shared"
)

type DatabaseClusterEntity struct {
	bun.BaseModel             `bun:"table:database_clusters,alias:database_cluster"`
	ID                        uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt                 time.Time       `bun:"created_at"`
	UpdatedAt                 time.Time       `bun:"updated_at"`
	Name                      string          `bun:"name"`
	Slug                      string          `bun:"slug"`
	Engine                    string          `bun:"engine"`
	EngineVersion             string          `bun:"engine_version"`
	SharingMode               string          `bun:"sharing_mode"`
	ManagementMode            string          `bun:"management_mode"`
	DesiredInstallationMethod sql.NullString  `bun:"desired_installation_method"`
	Topology                  json.RawMessage `bun:"topology,type:jsonb"`
	MaintenancePolicy         json.RawMessage `bun:"maintenance_policy,type:jsonb"`
	ArchivedAt                sql.NullTime    `bun:"archived_at"`
}

func (entity *DatabaseClusterEntity) Validate() error {
	entity.Name = strings.TrimSpace(entity.Name)
	entity.Slug = strings.ToLower(strings.TrimSpace(entity.Slug))
	entity.Engine = strings.ToLower(strings.TrimSpace(entity.Engine))
	entity.EngineVersion = strings.TrimSpace(entity.EngineVersion)
	entity.SharingMode = strings.ToLower(strings.TrimSpace(entity.SharingMode))
	entity.ManagementMode = strings.ToLower(strings.TrimSpace(entity.ManagementMode))
	if entity.DesiredInstallationMethod.Valid {
		entity.DesiredInstallationMethod.String = strings.ToLower(strings.TrimSpace(entity.DesiredInstallationMethod.String))
		if entity.DesiredInstallationMethod.String == "" {
			entity.DesiredInstallationMethod = sql.NullString{}
		}
	}
	builder := validation.NewBuilder()
	builder.Required("name", entity.Name)
	builder.Required("slug", entity.Slug)
	builder.Required("engineVersion", entity.EngineVersion)
	if !validSlug(entity.Slug) {
		builder.Add("slug", "format", "slug must contain lowercase letters, numbers, and single hyphens")
	}
	if entity.Engine != DatabaseEnginePostgreSQL && entity.Engine != DatabaseEngineMySQL {
		builder.Add("engine", "unsupported", "database engine must be postgresql or mysql")
	}
	if entity.SharingMode != DatabaseSharingDedicated && entity.SharingMode != DatabaseSharingShared {
		builder.Add("sharingMode", "unsupported", "database sharing mode must be dedicated or shared")
	}
	if entity.ManagementMode == ResourceManagementManaged.String() {
		if !entity.DesiredInstallationMethod.Valid ||
			(entity.DesiredInstallationMethod.String != DatabaseInstallDocker && entity.DesiredInstallationMethod.String != DatabaseInstallNative) {
			builder.Add("desiredInstallationMethod", "required", "managed Clusters require docker or native installation")
		}
	} else if entity.ManagementMode == ResourceManagementExternal.String() {
		if entity.DesiredInstallationMethod.Valid {
			builder.Add("desiredInstallationMethod", "forbidden", "external Clusters cannot select an installation method")
		}
	} else {
		builder.Add("managementMode", "unsupported", "management mode must be managed or external")
	}
	if !validJSONObject(entity.Topology) {
		builder.Add("topology", "invalid", "topology must be a JSON object")
	}
	if !validJSONObject(entity.MaintenancePolicy) {
		builder.Add("maintenancePolicy", "invalid", "maintenance policy must be a JSON object")
	}
	return builder.Err()
}

type CreateDatabaseClusterData struct {
	Name                      string
	Slug                      string
	Engine                    string
	EngineVersion             string
	SharingMode               string
	ManagementMode            string
	DesiredInstallationMethod sql.NullString
	Topology                  json.RawMessage
	MaintenancePolicy         json.RawMessage
	ArchivedAt                sql.NullTime
}

func (databaseCluster) Create(ctx context.Context, db storage.Executor, data CreateDatabaseClusterData) (DatabaseClusterEntity, error) {
	now := time.Now().UTC()
	entity := DatabaseClusterEntity{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, Name: data.Name, Slug: data.Slug,
		Engine: data.Engine, EngineVersion: data.EngineVersion, SharingMode: data.SharingMode, ManagementMode: data.ManagementMode,
		DesiredInstallationMethod: data.DesiredInstallationMethod, Topology: data.Topology,
		MaintenancePolicy: data.MaintenancePolicy, ArchivedAt: data.ArchivedAt,
	}
	if err := validation.Validate(&entity); err != nil {
		return DatabaseClusterEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "database-cluster-name:"+strings.ToLower(entity.Name), entity.ID, db.NewSelect().Model((*DatabaseClusterEntity)(nil)).Where("lower(name) = ?", strings.ToLower(entity.Name)), "name", "an active Database Cluster already uses this name"); err != nil {
		return DatabaseClusterEntity{}, err
	}
	if err := ensureActiveUnique(ctx, db, "database-cluster-slug:"+entity.Slug, entity.ID, db.NewSelect().Model((*DatabaseClusterEntity)(nil)).Where("lower(slug) = ?", entity.Slug), "slug", "an active Database Cluster already uses this slug"); err != nil {
		return DatabaseClusterEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return DatabaseClusterEntity{}, err
	}
	return entity, nil
}

func (databaseCluster) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (DatabaseClusterEntity, error) {
	var entity DatabaseClusterEntity
	if err := db.NewSelect().Model(&entity).Where("database_cluster.id = ?", id).Scan(ctx); err != nil {
		return DatabaseClusterEntity{}, err
	}
	return entity, nil
}

func (databaseCluster) FindForUpdate(ctx context.Context, db storage.Executor, id uuid.UUID) (DatabaseClusterEntity, error) {
	var entity DatabaseClusterEntity
	if err := db.NewSelect().Model(&entity).Where("database_cluster.id = ?", id).For("UPDATE").Scan(ctx); err != nil {
		return DatabaseClusterEntity{}, err
	}
	return entity, nil
}

func (databaseCluster) Update(ctx context.Context, db storage.Executor, entity DatabaseClusterEntity) (DatabaseClusterEntity, error) {
	entity.UpdatedAt = time.Now().UTC()
	if err := validation.Validate(&entity); err != nil {
		return DatabaseClusterEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "database-cluster-name:"+strings.ToLower(entity.Name), entity.ID, db.NewSelect().Model((*DatabaseClusterEntity)(nil)).Where("lower(name) = ?", strings.ToLower(entity.Name)), "name", "an active Database Cluster already uses this name"); err != nil {
		return DatabaseClusterEntity{}, err
	}
	if err := ensureActiveUnique(ctx, db, "database-cluster-slug:"+entity.Slug, entity.ID, db.NewSelect().Model((*DatabaseClusterEntity)(nil)).Where("lower(slug) = ?", entity.Slug), "slug", "an active Database Cluster already uses this slug"); err != nil {
		return DatabaseClusterEntity{}, err
	}
	if err := db.NewUpdate().Model(&entity).Column("updated_at", "name", "slug", "engine", "engine_version", "sharing_mode", "management_mode", "desired_installation_method", "topology", "maintenance_policy", "archived_at").WherePK().Returning("*").Scan(ctx); err != nil {
		return DatabaseClusterEntity{}, err
	}
	return entity, nil
}

type DatabaseClusterCredentialEntity struct {
	bun.BaseModel     `bun:"table:database_cluster_credentials,alias:database_cluster_credential"`
	ID                uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt         time.Time       `bun:"created_at"`
	UpdatedAt         time.Time       `bun:"updated_at"`
	Name              string          `bun:"name"`
	Role              string          `bun:"role"`
	Username          string          `bun:"username"`
	Metadata          json.RawMessage `bun:"metadata,type:jsonb"`
	EncPayload        []byte          `bun:"enc_payload"`
	Digest            []byte          `bun:"digest"`
	ArchivedAt        sql.NullTime    `bun:"archived_at"`
	DatabaseClusterID uuid.UUID       `bun:"database_cluster_id,type:uuid"`
}

func (entity *DatabaseClusterCredentialEntity) Validate() error {
	entity.Name = strings.TrimSpace(entity.Name)
	entity.Role = strings.ToLower(strings.TrimSpace(entity.Role))
	entity.Username = strings.TrimSpace(entity.Username)
	builder := validation.NewBuilder()
	builder.Required("name", entity.Name)
	builder.Required("role", entity.Role)
	builder.Required("username", entity.Username)
	if entity.DatabaseClusterID == uuid.Nil {
		builder.Add("databaseClusterId", "required", "Database Cluster is required")
	}
	if len(entity.EncPayload) == 0 || len(entity.Digest) == 0 {
		builder.Add("payload", "required", "encrypted credential payload and digest are required")
	}
	if !validJSONObject(entity.Metadata) {
		builder.Add("metadata", "invalid", "credential metadata must be a JSON object")
	}
	return builder.Err()
}

type CreateDatabaseClusterCredentialData struct {
	Name              string
	Role              string
	Username          string
	Metadata          json.RawMessage
	EncPayload        []byte
	Digest            []byte
	ArchivedAt        sql.NullTime
	DatabaseClusterID uuid.UUID
}

func (databaseClusterCredential) Create(ctx context.Context, db storage.Executor, data CreateDatabaseClusterCredentialData) (DatabaseClusterCredentialEntity, error) {
	now := time.Now().UTC()
	entity := DatabaseClusterCredentialEntity{ID: uuid.New(), CreatedAt: now, UpdatedAt: now, Name: data.Name, Role: data.Role, Username: data.Username, Metadata: data.Metadata, EncPayload: data.EncPayload, Digest: data.Digest, ArchivedAt: data.ArchivedAt, DatabaseClusterID: data.DatabaseClusterID}
	if err := validation.Validate(&entity); err != nil {
		return DatabaseClusterCredentialEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "database-cluster-credential:"+entity.DatabaseClusterID.String()+":"+entity.Role, entity.ID, db.NewSelect().Model((*DatabaseClusterCredentialEntity)(nil)).Where("database_cluster_id = ?", entity.DatabaseClusterID).Where("role = ?", entity.Role), "role", "an active credential already uses this role on the Database Resource"); err != nil {
		return DatabaseClusterCredentialEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return DatabaseClusterCredentialEntity{}, err
	}
	return entity, nil
}

func (databaseClusterCredential) ActiveAdministrator(ctx context.Context, db storage.Executor, clusterID uuid.UUID) (DatabaseClusterCredentialEntity, error) {
	var entity DatabaseClusterCredentialEntity
	if err := db.NewSelect().Model(&entity).Where("database_cluster_credential.database_cluster_id = ?", clusterID).Where("database_cluster_credential.role = 'administrator'").Where("database_cluster_credential.archived_at IS NULL").Scan(ctx); err != nil {
		return DatabaseClusterCredentialEntity{}, err
	}
	return entity, nil
}

type DatabaseClusterNodeEntity struct {
	bun.BaseModel     `bun:"table:database_cluster_nodes,alias:database_cluster_node"`
	ID                uuid.UUID    `bun:"id,pk,type:uuid"`
	CreatedAt         time.Time    `bun:"created_at"`
	UpdatedAt         time.Time    `bun:"updated_at"`
	Name              string       `bun:"name"`
	Role              string       `bun:"role"`
	DesiredState      string       `bun:"desired_state"`
	ArchivedAt        sql.NullTime `bun:"archived_at"`
	DatabaseClusterID uuid.UUID    `bun:"database_cluster_id,type:uuid"`
	ServerID          uuid.UUID    `bun:"server_id,type:uuid"`
}

func (entity *DatabaseClusterNodeEntity) Validate() error {
	entity.Name = strings.TrimSpace(entity.Name)
	entity.Role = strings.ToLower(strings.TrimSpace(entity.Role))
	entity.DesiredState = strings.ToLower(strings.TrimSpace(entity.DesiredState))
	builder := validation.NewBuilder()
	builder.Required("name", entity.Name)
	if entity.Role != "primary" && entity.Role != "replica" && entity.Role != "candidate" {
		builder.Add("role", "unsupported", "Node role must be primary, replica, or candidate")
	}
	if entity.DesiredState != "running" && entity.DesiredState != "stopped" && entity.DesiredState != "retired" {
		builder.Add("desiredState", "unsupported", "Node desired state is not supported")
	}
	if entity.DatabaseClusterID == uuid.Nil || entity.ServerID == uuid.Nil {
		builder.Add("placement", "required", "Cluster and Server are required")
	}
	return builder.Err()
}

type CreateDatabaseClusterNodeData struct {
	Name, Role, DesiredState    string
	ArchivedAt                  sql.NullTime
	DatabaseClusterID, ServerID uuid.UUID
}

func (databaseClusterNode) Create(ctx context.Context, db storage.Executor, data CreateDatabaseClusterNodeData) (DatabaseClusterNodeEntity, error) {
	now := time.Now().UTC()
	entity := DatabaseClusterNodeEntity{ID: uuid.New(), CreatedAt: now, UpdatedAt: now, Name: data.Name, Role: data.Role, DesiredState: data.DesiredState, ArchivedAt: data.ArchivedAt, DatabaseClusterID: data.DatabaseClusterID, ServerID: data.ServerID}
	if err := validation.Validate(&entity); err != nil {
		return DatabaseClusterNodeEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "database-cluster-node:"+entity.DatabaseClusterID.String()+":"+strings.ToLower(entity.Name), entity.ID, db.NewSelect().Model((*DatabaseClusterNodeEntity)(nil)).Where("database_cluster_id = ?", entity.DatabaseClusterID).Where("lower(name) = ?", strings.ToLower(entity.Name)), "name", "an active Node already uses this name on the Database Resource"); err != nil {
		return DatabaseClusterNodeEntity{}, err
	}
	if entity.Role == "primary" {
		if err := ensureActiveUnique(ctx, db, "database-cluster-primary:"+entity.DatabaseClusterID.String(), entity.ID, db.NewSelect().Model((*DatabaseClusterNodeEntity)(nil)).Where("database_cluster_id = ?", entity.DatabaseClusterID).Where("role = 'primary'"), "role", "the Database Resource already has an active primary Node"); err != nil {
			return DatabaseClusterNodeEntity{}, err
		}
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return DatabaseClusterNodeEntity{}, err
	}
	return entity, nil
}

func (databaseClusterNode) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (DatabaseClusterNodeEntity, error) {
	var entity DatabaseClusterNodeEntity
	if err := db.NewSelect().Model(&entity).Where("database_cluster_node.id = ?", id).Scan(ctx); err != nil {
		return DatabaseClusterNodeEntity{}, err
	}
	return entity, nil
}

type DatabaseNodeStorageEntity struct {
	bun.BaseModel         `bun:"table:database_node_storage,alias:database_node_storage"`
	ID                    uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt             time.Time       `bun:"created_at"`
	UpdatedAt             time.Time       `bun:"updated_at"`
	Name                  string          `bun:"name"`
	Driver                string          `bun:"driver"`
	ExternalID            sql.NullString  `bun:"external_id"`
	DataPath              string          `bun:"data_path"`
	Configuration         json.RawMessage `bun:"configuration,type:jsonb"`
	ArchivedAt            sql.NullTime    `bun:"archived_at"`
	DatabaseClusterNodeID uuid.UUID       `bun:"database_cluster_node_id,type:uuid"`
	ServerID              uuid.UUID       `bun:"server_id,type:uuid"`
}

func (entity *DatabaseNodeStorageEntity) Validate() error {
	entity.Name = strings.TrimSpace(entity.Name)
	entity.Driver = strings.ToLower(strings.TrimSpace(entity.Driver))
	entity.DataPath = strings.TrimSpace(entity.DataPath)
	builder := validation.NewBuilder()
	builder.Required("name", entity.Name)
	builder.Required("driver", entity.Driver)
	builder.Required("dataPath", entity.DataPath)
	if entity.DatabaseClusterNodeID == uuid.Nil || entity.ServerID == uuid.Nil {
		builder.Add("placement", "required", "Node and Server are required")
	}
	if !strings.HasPrefix(entity.DataPath, "/") {
		builder.Add("dataPath", "format", "database data path must be absolute")
	}
	if !validJSONObject(entity.Configuration) {
		builder.Add("configuration", "invalid", "storage configuration must be a JSON object")
	}
	return builder.Err()
}

type CreateDatabaseNodeStorageData struct {
	Name, Driver, DataPath          string
	ExternalID                      sql.NullString
	Configuration                   json.RawMessage
	ArchivedAt                      sql.NullTime
	DatabaseClusterNodeID, ServerID uuid.UUID
}

func (databaseNodeStorage) Create(ctx context.Context, db storage.Executor, data CreateDatabaseNodeStorageData) (DatabaseNodeStorageEntity, error) {
	now := time.Now().UTC()
	entity := DatabaseNodeStorageEntity{ID: uuid.New(), CreatedAt: now, UpdatedAt: now, Name: data.Name, Driver: data.Driver, ExternalID: data.ExternalID, DataPath: data.DataPath, Configuration: data.Configuration, ArchivedAt: data.ArchivedAt, DatabaseClusterNodeID: data.DatabaseClusterNodeID, ServerID: data.ServerID}
	if err := validation.Validate(&entity); err != nil {
		return DatabaseNodeStorageEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "database-node-storage:"+entity.DatabaseClusterNodeID.String()+":"+strings.ToLower(entity.Name), entity.ID, db.NewSelect().Model((*DatabaseNodeStorageEntity)(nil)).Where("database_cluster_node_id = ?", entity.DatabaseClusterNodeID).Where("lower(name) = ?", strings.ToLower(entity.Name)), "name", "active storage already uses this name on the Database Node"); err != nil {
		return DatabaseNodeStorageEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return DatabaseNodeStorageEntity{}, err
	}
	return entity, nil
}

type DatabaseNodeInstallationEntity struct {
	bun.BaseModel         `bun:"table:database_node_installations,alias:database_node_installation"`
	ID                    uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt             time.Time      `bun:"created_at"`
	UpdatedAt             time.Time      `bun:"updated_at"`
	InstallationMethod    string         `bun:"installation_method"`
	DesiredState          string         `bun:"desired_state"`
	ObservedState         string         `bun:"observed_state"`
	InstalledVersion      sql.NullString `bun:"installed_version"`
	ServiceState          string         `bun:"service_state"`
	Health                string         `bun:"health"`
	Reason                sql.NullString `bun:"reason"`
	ObservedAt            sql.NullTime   `bun:"observed_at"`
	ExternalRuntimeID     sql.NullString `bun:"external_runtime_id"`
	ArchivedAt            sql.NullTime   `bun:"archived_at"`
	DatabaseClusterNodeID uuid.UUID      `bun:"database_cluster_node_id,type:uuid"`
	ServerID              uuid.UUID      `bun:"server_id,type:uuid"`
	DatabaseNodeStorageID uuid.UUID      `bun:"database_node_storage_id,type:uuid"`
}

func (entity *DatabaseNodeInstallationEntity) Validate() error {
	entity.InstallationMethod = strings.ToLower(strings.TrimSpace(entity.InstallationMethod))
	builder := validation.NewBuilder()
	if entity.InstallationMethod != DatabaseInstallDocker && entity.InstallationMethod != DatabaseInstallNative {
		builder.Add("installationMethod", "unsupported", "installation method must be docker or native")
	}
	if strings.TrimSpace(entity.DesiredState) == "" || strings.TrimSpace(entity.ObservedState) == "" || strings.TrimSpace(entity.ServiceState) == "" || strings.TrimSpace(entity.Health) == "" {
		builder.Add("state", "required", "installation lifecycle states are required")
	}
	if entity.DatabaseClusterNodeID == uuid.Nil || entity.ServerID == uuid.Nil || entity.DatabaseNodeStorageID == uuid.Nil {
		builder.Add("placement", "required", "Node, Server, and stable storage are required")
	}
	return builder.Err()
}

type CreateDatabaseNodeInstallationData struct {
	ID                                                                    uuid.UUID
	InstallationMethod, DesiredState, ObservedState, ServiceState, Health string
	InstalledVersion, Reason, ExternalRuntimeID                           sql.NullString
	ObservedAt, ArchivedAt                                                sql.NullTime
	DatabaseClusterNodeID, ServerID, DatabaseNodeStorageID                uuid.UUID
}

func (databaseNodeInstallation) Create(ctx context.Context, db storage.Executor, data CreateDatabaseNodeInstallationData) (DatabaseNodeInstallationEntity, error) {
	now := time.Now().UTC()
	id := data.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	entity := DatabaseNodeInstallationEntity{ID: id, CreatedAt: now, UpdatedAt: now, InstallationMethod: data.InstallationMethod, DesiredState: data.DesiredState, ObservedState: data.ObservedState, InstalledVersion: data.InstalledVersion, ServiceState: data.ServiceState, Health: data.Health, Reason: data.Reason, ObservedAt: data.ObservedAt, ExternalRuntimeID: data.ExternalRuntimeID, ArchivedAt: data.ArchivedAt, DatabaseClusterNodeID: data.DatabaseClusterNodeID, ServerID: data.ServerID, DatabaseNodeStorageID: data.DatabaseNodeStorageID}
	if err := validation.Validate(&entity); err != nil {
		return DatabaseNodeInstallationEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "database-node-installation:"+entity.DatabaseClusterNodeID.String(), entity.ID, db.NewSelect().Model((*DatabaseNodeInstallationEntity)(nil)).Where("database_cluster_node_id = ?", entity.DatabaseClusterNodeID), "databaseClusterNodeId", "the Database Node already has an active installation"); err != nil {
		return DatabaseNodeInstallationEntity{}, err
	}
	var owner struct {
		NodeServerID, StorageNodeID, StorageServerID uuid.UUID
		ClusterMethod                                sql.NullString
		ManagementMode                               string
	}
	if err := db.NewSelect().TableExpr("database_cluster_nodes AS node").ColumnExpr("node.server_id AS node_server_id, storage.database_cluster_node_id AS storage_node_id, storage.server_id AS storage_server_id").ColumnExpr("cluster.desired_installation_method AS cluster_method, cluster.management_mode").Join("JOIN database_clusters AS cluster ON cluster.id = node.database_cluster_id").Join("JOIN database_node_storage AS storage ON storage.id = ?", entity.DatabaseNodeStorageID).Where("node.id = ?", entity.DatabaseClusterNodeID).Scan(ctx, &owner); err != nil {
		return DatabaseNodeInstallationEntity{}, err
	}
	if owner.ManagementMode != ResourceManagementManaged.String() || !owner.ClusterMethod.Valid || owner.ClusterMethod.String != entity.InstallationMethod || owner.NodeServerID != entity.ServerID || owner.StorageNodeID != entity.DatabaseClusterNodeID || owner.StorageServerID != entity.ServerID {
		return DatabaseNodeInstallationEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "installationMethod", Code: "topology", Message: "installation must match the managed Cluster method, Node, Server, and storage placement"}})
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return DatabaseNodeInstallationEntity{}, err
	}
	return entity, nil
}

type DockerDatabaseNodeInstallationEntity struct {
	bun.BaseModel              `bun:"table:docker_database_node_installations,alias:docker_database_node_installation"`
	DatabaseNodeInstallationID uuid.UUID       `bun:"database_node_installation_id,pk,type:uuid"`
	CreatedAt                  time.Time       `bun:"created_at"`
	UpdatedAt                  time.Time       `bun:"updated_at"`
	ImageReference             string          `bun:"image_reference"`
	ImageDigest                sql.NullString  `bun:"image_digest"`
	ContainerName              string          `bun:"container_name"`
	RestartPolicy              string          `bun:"restart_policy"`
	PortMappings               json.RawMessage `bun:"port_mappings,type:jsonb"`
	Configuration              json.RawMessage `bun:"configuration,type:jsonb"`
	RegistryResourceID         *uuid.UUID      `bun:"registry_resource_id,type:uuid"`
	RegistryCredentialID       *uuid.UUID      `bun:"registry_credential_id,type:uuid"`
}

func (entity *DockerDatabaseNodeInstallationEntity) Validate() error {
	builder := validation.NewBuilder()
	builder.Required("imageReference", strings.TrimSpace(entity.ImageReference))
	builder.Required("containerName", strings.TrimSpace(entity.ContainerName))
	builder.Required("restartPolicy", strings.TrimSpace(entity.RestartPolicy))
	if entity.DatabaseNodeInstallationID == uuid.Nil {
		builder.Add("databaseNodeInstallationId", "required", "Node Installation is required")
	}
	if !json.Valid(entity.PortMappings) || !validJSONObject(entity.Configuration) {
		builder.Add("configuration", "invalid", "Docker port mappings and configuration must be valid JSON")
	}
	if (entity.RegistryResourceID == nil) != (entity.RegistryCredentialID == nil) {
		builder.Add("registry", "incoherent", "Registry Resource and credential must be selected together")
	}
	return builder.Err()
}

type CreateDockerDatabaseNodeInstallationData struct {
	DatabaseNodeInstallationID               uuid.UUID
	ImageReference                           string
	ImageDigest                              sql.NullString
	ContainerName, RestartPolicy             string
	PortMappings, Configuration              json.RawMessage
	RegistryResourceID, RegistryCredentialID *uuid.UUID
}

func (dockerDatabaseInstallation) Create(ctx context.Context, db storage.Executor, data CreateDockerDatabaseNodeInstallationData) (DockerDatabaseNodeInstallationEntity, error) {
	now := time.Now().UTC()
	entity := DockerDatabaseNodeInstallationEntity{DatabaseNodeInstallationID: data.DatabaseNodeInstallationID, CreatedAt: now, UpdatedAt: now, ImageReference: data.ImageReference, ImageDigest: data.ImageDigest, ContainerName: data.ContainerName, RestartPolicy: data.RestartPolicy, PortMappings: data.PortMappings, Configuration: data.Configuration, RegistryResourceID: data.RegistryResourceID, RegistryCredentialID: data.RegistryCredentialID}
	if err := validation.Validate(&entity); err != nil {
		return DockerDatabaseNodeInstallationEntity{}, errors.Join(ErrDomainValidation, err)
	}
	var method string
	if err := db.NewSelect().TableExpr("database_node_installations").Column("installation_method").Where("id = ?", entity.DatabaseNodeInstallationID).Scan(ctx, &method); err != nil {
		return DockerDatabaseNodeInstallationEntity{}, err
	}
	if method != DatabaseInstallDocker {
		return DockerDatabaseNodeInstallationEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "databaseNodeInstallationId", Code: "method", Message: "Docker details require a Docker Node Installation"}})
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return DockerDatabaseNodeInstallationEntity{}, err
	}
	return entity, nil
}

type NativeDatabaseNodeInstallationEntity struct {
	bun.BaseModel              `bun:"table:native_database_node_installations,alias:native_database_node_installation"`
	DatabaseNodeInstallationID uuid.UUID       `bun:"database_node_installation_id,pk,type:uuid"`
	CreatedAt                  time.Time       `bun:"created_at"`
	UpdatedAt                  time.Time       `bun:"updated_at"`
	PackageName                string          `bun:"package_name"`
	RequestedPackageVersion    sql.NullString  `bun:"requested_package_version"`
	ServiceName                string          `bun:"service_name"`
	ConfigurationPath          string          `bun:"configuration_path"`
	Settings                   json.RawMessage `bun:"settings,type:jsonb"`
}

func (entity *NativeDatabaseNodeInstallationEntity) Validate() error {
	builder := validation.NewBuilder()
	if entity.DatabaseNodeInstallationID == uuid.Nil {
		builder.Add("databaseNodeInstallationId", "required", "Node Installation is required")
	}
	builder.Required("packageName", strings.TrimSpace(entity.PackageName))
	builder.Required("serviceName", strings.TrimSpace(entity.ServiceName))
	if !strings.HasPrefix(strings.TrimSpace(entity.ConfigurationPath), "/") {
		builder.Add("configurationPath", "format", "configuration path must be absolute")
	}
	if !validJSONObject(entity.Settings) {
		builder.Add("settings", "invalid", "native settings must be a JSON object")
	}
	return builder.Err()
}

type CreateNativeDatabaseNodeInstallationData struct {
	DatabaseNodeInstallationID     uuid.UUID
	PackageName                    string
	RequestedPackageVersion        sql.NullString
	ServiceName, ConfigurationPath string
	Settings                       json.RawMessage
}

func (nativeDatabaseInstallation) Create(ctx context.Context, db storage.Executor, data CreateNativeDatabaseNodeInstallationData) (NativeDatabaseNodeInstallationEntity, error) {
	now := time.Now().UTC()
	entity := NativeDatabaseNodeInstallationEntity{DatabaseNodeInstallationID: data.DatabaseNodeInstallationID, CreatedAt: now, UpdatedAt: now, PackageName: data.PackageName, RequestedPackageVersion: data.RequestedPackageVersion, ServiceName: data.ServiceName, ConfigurationPath: data.ConfigurationPath, Settings: data.Settings}
	if err := validation.Validate(&entity); err != nil {
		return NativeDatabaseNodeInstallationEntity{}, errors.Join(ErrDomainValidation, err)
	}
	var method string
	if err := db.NewSelect().TableExpr("database_node_installations").Column("installation_method").Where("id = ?", entity.DatabaseNodeInstallationID).Scan(ctx, &method); err != nil {
		return NativeDatabaseNodeInstallationEntity{}, err
	}
	if method != DatabaseInstallNative {
		return NativeDatabaseNodeInstallationEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "databaseNodeInstallationId", Code: "method", Message: "native details require a native Node Installation"}})
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return NativeDatabaseNodeInstallationEntity{}, err
	}
	return entity, nil
}

type DatabaseClusterEndpointEntity struct {
	bun.BaseModel                                  `bun:"table:database_cluster_endpoints,alias:database_cluster_endpoint"`
	ID                                             uuid.UUID `bun:"id,pk,type:uuid"`
	CreatedAt                                      time.Time `bun:"created_at"`
	UpdatedAt                                      time.Time `bun:"updated_at"`
	Name, Role, Address                            string
	Port                                           int32
	Protocol, TLSMode, DesiredState, ObservedState string
	Settings                                       json.RawMessage `bun:"settings,type:jsonb"`
	ArchivedAt                                     sql.NullTime
	DatabaseClusterID                              uuid.UUID  `bun:"database_cluster_id,type:uuid"`
	PrivateNetworkID                               *uuid.UUID `bun:"private_network_id,type:uuid"`
}

func (entity *DatabaseClusterEndpointEntity) Validate() error {
	entity.Name = strings.TrimSpace(entity.Name)
	entity.Role = strings.ToLower(strings.TrimSpace(entity.Role))
	entity.Address = strings.TrimSpace(entity.Address)
	entity.Protocol = strings.ToLower(strings.TrimSpace(entity.Protocol))
	entity.TLSMode = strings.ToLower(strings.TrimSpace(entity.TLSMode))
	builder := validation.NewBuilder()
	builder.Required("name", entity.Name)
	builder.Required("address", entity.Address)
	if entity.DatabaseClusterID == uuid.Nil {
		builder.Add("databaseClusterId", "required", "Database Cluster is required")
	}
	if entity.Port < 1 || entity.Port > 65535 {
		builder.Add("port", "range", "endpoint port must be between 1 and 65535")
	}
	if entity.Role != "primary" && entity.Role != "read_only" && entity.Role != "administrative" && entity.Role != "wireguard" {
		builder.Add("role", "unsupported", "endpoint role is not supported")
	}
	if !validJSONObject(entity.Settings) {
		builder.Add("settings", "invalid", "endpoint settings must be a JSON object")
	}
	return builder.Err()
}

type CreateDatabaseClusterEndpointData struct {
	Name, Role, Address                            string
	Port                                           int32
	Protocol, TLSMode, DesiredState, ObservedState string
	Settings                                       json.RawMessage
	ArchivedAt                                     sql.NullTime
	DatabaseClusterID                              uuid.UUID
	PrivateNetworkID                               *uuid.UUID
}

func (databaseClusterEndpoint) Create(ctx context.Context, db storage.Executor, data CreateDatabaseClusterEndpointData) (DatabaseClusterEndpointEntity, error) {
	now := time.Now().UTC()
	entity := DatabaseClusterEndpointEntity{ID: uuid.New(), CreatedAt: now, UpdatedAt: now, Name: data.Name, Role: data.Role, Address: data.Address, Port: data.Port, Protocol: data.Protocol, TLSMode: data.TLSMode, DesiredState: data.DesiredState, ObservedState: data.ObservedState, Settings: data.Settings, ArchivedAt: data.ArchivedAt, DatabaseClusterID: data.DatabaseClusterID, PrivateNetworkID: data.PrivateNetworkID}
	if err := validation.Validate(&entity); err != nil {
		return DatabaseClusterEndpointEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "database-cluster-endpoint:"+entity.DatabaseClusterID.String()+":"+strings.ToLower(entity.Name), entity.ID, db.NewSelect().Model((*DatabaseClusterEndpointEntity)(nil)).Where("database_cluster_id = ?", entity.DatabaseClusterID).Where("lower(name) = ?", strings.ToLower(entity.Name)), "name", "an active endpoint already uses this name on the Database Resource"); err != nil {
		return DatabaseClusterEndpointEntity{}, err
	}
	var engine string
	if err := db.NewSelect().TableExpr("database_clusters").Column("engine").Where("id = ?", entity.DatabaseClusterID).Scan(ctx, &engine); err != nil {
		return DatabaseClusterEndpointEntity{}, err
	}
	if (engine == DatabaseEnginePostgreSQL && entity.Protocol != "postgresql" && entity.Protocol != "tcp") || (engine == DatabaseEngineMySQL && entity.Protocol != "mysql" && entity.Protocol != "tcp") {
		return DatabaseClusterEndpointEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "protocol", Code: "engine", Message: "endpoint protocol must match the Cluster engine"}})
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return DatabaseClusterEndpointEntity{}, err
	}
	return entity, nil
}

func validSlug(value string) bool {
	if value == "" || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	previousHyphen := false
	for _, character := range value {
		isLetter := character >= 'a' && character <= 'z'
		isNumber := character >= '0' && character <= '9'
		if !isLetter && !isNumber && character != '-' {
			return false
		}
		if character == '-' && previousHyphen {
			return false
		}
		previousHyphen = character == '-'
	}
	return true
}
