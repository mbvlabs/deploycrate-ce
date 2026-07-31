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

type DatabaseEntity struct {
	bun.BaseModel     `bun:"table:databases,alias:database"`
	ID                uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt         time.Time       `bun:"created_at"`
	UpdatedAt         time.Time       `bun:"updated_at"`
	Name              string          `bun:"name"`
	Encoding          sql.NullString  `bun:"encoding"`
	Collation         sql.NullString  `bun:"database_collation"`
	Settings          json.RawMessage `bun:"settings,type:jsonb"`
	DesiredState      string          `bun:"desired_state"`
	ObservedState     string          `bun:"observed_state"`
	ArchivedAt        sql.NullTime    `bun:"archived_at"`
	DatabaseClusterID uuid.UUID       `bun:"database_cluster_id,type:uuid"`
}

func (entity *DatabaseEntity) Validate() error {
	entity.Name = strings.TrimSpace(entity.Name)
	entity.DesiredState = strings.ToLower(strings.TrimSpace(entity.DesiredState))
	entity.ObservedState = strings.ToLower(strings.TrimSpace(entity.ObservedState))
	builder := validation.NewBuilder()
	builder.Required("name", entity.Name)
	if entity.DatabaseClusterID == uuid.Nil {
		builder.Add("databaseClusterId", "required", "Database Cluster is required")
	}
	if len([]byte(entity.Name)) > 63 || strings.ContainsRune(entity.Name, '\x00') {
		builder.Add("name", "format", "database name must be at most 63 bytes and cannot contain null bytes")
	}
	if entity.DesiredState != "provisioned" && entity.DesiredState != "deprovisioned" {
		builder.Add("desiredState", "unsupported", "Database desired state is not supported")
	}
	if strings.TrimSpace(entity.ObservedState) == "" {
		builder.Add("observedState", "required", "Database observed state is required")
	}
	if !validJSONObject(entity.Settings) {
		builder.Add("settings", "invalid", "Database settings must be a JSON object")
	}
	return builder.Err()
}

type CreateDatabaseData struct {
	Name                        string
	Encoding, Collation         sql.NullString
	Settings                    json.RawMessage
	DesiredState, ObservedState string
	ArchivedAt                  sql.NullTime
	DatabaseClusterID           uuid.UUID
}

func (database) Create(ctx context.Context, db storage.Executor, data CreateDatabaseData) (DatabaseEntity, error) {
	now := time.Now().UTC()
	entity := DatabaseEntity{ID: uuid.New(), CreatedAt: now, UpdatedAt: now, Name: data.Name, Encoding: data.Encoding, Collation: data.Collation, Settings: data.Settings, DesiredState: data.DesiredState, ObservedState: data.ObservedState, ArchivedAt: data.ArchivedAt, DatabaseClusterID: data.DatabaseClusterID}
	if err := validation.Validate(&entity); err != nil {
		return DatabaseEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "database-name:"+entity.DatabaseClusterID.String()+":"+strings.ToLower(entity.Name), entity.ID, db.NewSelect().Model((*DatabaseEntity)(nil)).Where("database_cluster_id = ?", entity.DatabaseClusterID).Where("lower(name) = ?", strings.ToLower(entity.Name)), "name", "an active Database already uses this name in the Cluster"); err != nil {
		return DatabaseEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return DatabaseEntity{}, err
	}
	return entity, nil
}

func (database) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (DatabaseEntity, error) {
	var entity DatabaseEntity
	if err := db.NewSelect().Model(&entity).Where("database.id = ?", id).Scan(ctx); err != nil {
		return DatabaseEntity{}, err
	}
	return entity, nil
}

func (database) FindForUpdate(ctx context.Context, db storage.Executor, id uuid.UUID) (DatabaseEntity, error) {
	var entity DatabaseEntity
	if err := db.NewSelect().Model(&entity).Where("database.id = ?", id).For("UPDATE").Scan(ctx); err != nil {
		return DatabaseEntity{}, err
	}
	return entity, nil
}

func (database) Update(ctx context.Context, db storage.Executor, entity DatabaseEntity) (DatabaseEntity, error) {
	entity.UpdatedAt = time.Now().UTC()
	if err := validation.Validate(&entity); err != nil {
		return DatabaseEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := db.NewUpdate().Model(&entity).Column("updated_at", "name", "encoding", "database_collation", "settings", "desired_state", "observed_state", "archived_at").WherePK().Returning("*").Scan(ctx); err != nil {
		return DatabaseEntity{}, err
	}
	return entity, nil
}

type DatabaseResourceEntity struct {
	bun.BaseModel `bun:"table:database_resources,alias:database_resource"`
	ResourceID    uuid.UUID `bun:"resource_id,pk,type:uuid"`
	DatabaseID    uuid.UUID `bun:"database_id,type:uuid"`
	CreatedAt     time.Time `bun:"created_at"`
	UpdatedAt     time.Time `bun:"updated_at"`
}

func (entity *DatabaseResourceEntity) Validate() error {
	builder := validation.NewBuilder()
	if entity.ResourceID == uuid.Nil || entity.DatabaseID == uuid.Nil {
		builder.Add("backing", "required", "Resource and Database are required")
	}
	return builder.Err()
}

func (databaseResource) Create(ctx context.Context, db storage.Executor, resourceID, databaseID uuid.UUID) (DatabaseResourceEntity, error) {
	now := time.Now().UTC()
	entity := DatabaseResourceEntity{ResourceID: resourceID, DatabaseID: databaseID, CreatedAt: now, UpdatedAt: now}
	if err := validation.Validate(&entity); err != nil {
		return DatabaseResourceEntity{}, errors.Join(ErrDomainValidation, err)
	}
	var backing struct{ ResourceKind, Engine string }
	if err := db.NewSelect().TableExpr("resources AS resource").ColumnExpr("resource.kind AS resource_kind, cluster.engine").Join("JOIN databases AS database ON database.id = ? AND database.archived_at IS NULL", entity.DatabaseID).Join("JOIN database_clusters AS cluster ON cluster.id = database.database_cluster_id AND cluster.archived_at IS NULL").Where("resource.id = ?", entity.ResourceID).Where("resource.archived_at IS NULL").Scan(ctx, &backing); err != nil {
		return DatabaseResourceEntity{}, err
	}
	if backing.ResourceKind != backing.Engine {
		return DatabaseResourceEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "resourceId", Code: "kind", Message: "Resource kind must match the Database Cluster engine"}})
	}
	if err := ensureUnique(ctx, db, "database-resource:"+entity.DatabaseID.String(), db.NewSelect().Model((*DatabaseResourceEntity)(nil)).Where("database_id = ?", entity.DatabaseID), "databaseId", "the Database is already attached to a Resource"); err != nil {
		return DatabaseResourceEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return DatabaseResourceEntity{}, err
	}
	return entity, nil
}

type DatabaseResourceDetail struct {
	ResourceID          uuid.UUID `bun:"resource_id"`
	DatabaseID          uuid.UUID `bun:"database_id"`
	DatabaseName        string    `bun:"database_name"`
	DatabaseClusterID   uuid.UUID `bun:"database_cluster_id"`
	DatabaseClusterName string    `bun:"database_cluster_name"`
	Engine              string    `bun:"engine"`
}

func (databaseResource) FindByResource(ctx context.Context, db storage.Executor, resourceID uuid.UUID) (DatabaseResourceDetail, error) {
	var detail DatabaseResourceDetail
	if err := db.NewSelect().TableExpr("database_resources AS backing").ColumnExpr("backing.resource_id, backing.database_id, database.name AS database_name").ColumnExpr("cluster.id AS database_cluster_id, cluster.name AS database_cluster_name, cluster.engine").Join("JOIN databases AS database ON database.id = backing.database_id").Join("JOIN database_clusters AS cluster ON cluster.id = database.database_cluster_id").Where("backing.resource_id = ?", resourceID).Scan(ctx, &detail); err != nil {
		return DatabaseResourceDetail{}, err
	}
	return detail, nil
}

type DatabaseResourceEndpointEntity struct {
	bun.BaseModel             `bun:"table:database_resource_endpoints,alias:database_resource_endpoint"`
	ResourceEndpointID        uuid.UUID `bun:"resource_endpoint_id,pk,type:uuid"`
	DatabaseClusterEndpointID uuid.UUID `bun:"database_cluster_endpoint_id,type:uuid"`
	CreatedAt                 time.Time `bun:"created_at"`
	UpdatedAt                 time.Time `bun:"updated_at"`
}

func (databaseResourceEndpoint) Create(ctx context.Context, db storage.Executor, resourceEndpointID, clusterEndpointID uuid.UUID) (DatabaseResourceEndpointEntity, error) {
	if resourceEndpointID == uuid.Nil || clusterEndpointID == uuid.Nil {
		return DatabaseResourceEndpointEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "endpoint", Code: "required", Message: "published and Cluster endpoints are required"}})
	}
	var coherent bool
	if err := db.NewSelect().TableExpr("resource_endpoints AS endpoint").ColumnExpr("database.database_cluster_id = cluster_endpoint.database_cluster_id").Join("JOIN database_resources AS backing ON backing.resource_id = endpoint.resource_id").Join("JOIN databases AS database ON database.id = backing.database_id").Join("JOIN database_cluster_endpoints AS cluster_endpoint ON cluster_endpoint.id = ?", clusterEndpointID).Where("endpoint.id = ?", resourceEndpointID).Scan(ctx, &coherent); err != nil {
		return DatabaseResourceEndpointEntity{}, err
	}
	if !coherent {
		return DatabaseResourceEndpointEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "endpoint", Code: "cluster", Message: "published endpoint and Database must belong to the same Cluster"}})
	}
	now := time.Now().UTC()
	entity := DatabaseResourceEndpointEntity{ResourceEndpointID: resourceEndpointID, DatabaseClusterEndpointID: clusterEndpointID, CreatedAt: now, UpdatedAt: now}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return DatabaseResourceEndpointEntity{}, err
	}
	return entity, nil
}
