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

type ResourceHealthCheckEntity struct {
	bun.BaseModel          `bun:"table:resource_health_checks,alias:resource_health_checks"`
	ID                     uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt              time.Time       `bun:"created_at"`
	UpdatedAt              time.Time       `bun:"updated_at"`
	Name                   string          `bun:"name"`
	Kind                   string          `bun:"kind"`
	Configuration          json.RawMessage `bun:"configuration,type:jsonb"`
	IntervalSeconds        int32           `bun:"interval_seconds"`
	TimeoutSeconds         int32           `bun:"timeout_seconds"`
	FailureThreshold       int32           `bun:"failure_threshold"`
	SuccessThreshold       int32           `bun:"success_threshold"`
	Enabled                bool            `bun:"enabled"`
	ArchivedAt             sql.NullTime    `bun:"archived_at"`
	ResourceID             uuid.UUID       `bun:"resource_id,type:uuid"`
	ResourceInstallationID *uuid.UUID      `bun:"resource_installation_id,type:uuid"`
	ResourceEndpointID     *uuid.UUID      `bun:"resource_endpoint_id,type:uuid"`
	ResourceCredentialID   *uuid.UUID      `bun:"resource_credential_id,type:uuid"`
}

type DueResourceHealthCheck struct {
	ResourceHealthCheckEntity
	ResourceID                 uuid.UUID       `bun:"resource_id"`
	ResourceName               string          `bun:"resource_name"`
	ResourceKind               string          `bun:"resource_kind"`
	ResourceDatabaseName       string          `bun:"resource_database_name"`
	EndpointAddress            string          `bun:"endpoint_address"`
	EndpointPort               int32           `bun:"endpoint_port"`
	EndpointProtocol           string          `bun:"endpoint_protocol"`
	EndpointTLSMode            string          `bun:"endpoint_tls_mode"`
	EndpointSettings           json.RawMessage `bun:"endpoint_settings"`
	CredentialUsername         sql.NullString  `bun:"credential_username"`
	CredentialEncryptedPayload []byte          `bun:"credential_encrypted_payload"`
	StatusPresent              bool            `bun:"status_present"`
	StatusState                string          `bun:"status_state"`
	StatusConsecutiveSuccesses int32           `bun:"status_consecutive_successes"`
	StatusConsecutiveFailures  int32           `bun:"status_consecutive_failures"`
	StatusExpiresAt            sql.NullTime    `bun:"status_expires_at"`
}

func (e *ResourceHealthCheckEntity) Validate() error {
	e.Name = strings.TrimSpace(e.Name)
	e.Kind = strings.ToLower(strings.TrimSpace(e.Kind))
	builder := validation.NewBuilder()
	builder.Required("name", e.Name)
	builder.Required("kind", e.Kind)
	if len(e.Configuration) == 0 || !json.Valid(e.Configuration) {
		builder.Add("configuration", "invalid", "configuration must be valid JSON")
	} else if settingsContainSecret(e.Configuration) {
		builder.Add("configuration", "secret", "configuration must not contain raw credentials")
	}
	if e.IntervalSeconds < 1 {
		builder.Add("intervalSeconds", "positive", "interval must be positive")
	}
	if e.TimeoutSeconds < 1 {
		builder.Add("timeoutSeconds", "positive", "timeout must be positive")
	} else if e.IntervalSeconds > 0 && e.TimeoutSeconds > e.IntervalSeconds {
		builder.Add("timeoutSeconds", "range", "timeout cannot exceed the interval")
	}
	if e.FailureThreshold < 1 {
		builder.Add("failureThreshold", "positive", "failure threshold must be positive")
	}
	if e.SuccessThreshold < 1 {
		builder.Add("successThreshold", "positive", "success threshold must be positive")
	}
	if e.ResourceID == uuid.Nil {
		builder.Add("resourceId", "required", "Resource is required")
	}
	return builder.Err()
}

func (e *ResourceHealthCheckEntity) ValidateForKind(resourceKind string) error {
	if err := e.Validate(); err != nil {
		return err
	}
	definition, ok := FindResourceKind(resourceKind)
	if !ok || !definition.SupportsHealthCheck(e.Kind) {
		return validation.ValidationErrors{{Field: "kind", Code: "unsupported", Message: "health check kind is not supported by this resource kind"}}
	}
	return nil
}

func (rhc resourceHealthCheck) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ResourceHealthCheckEntity, error) {
	var entity ResourceHealthCheckEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceHealthCheckEntity{}, err
	}

	return entity, nil
}

func (rhc resourceHealthCheck) DueApplicationChecks(
	ctx context.Context,
	db storage.Executor,
	now time.Time,
	limit int,
) ([]DueResourceHealthCheck, error) {
	if limit < 1 {
		limit = 100
	}
	checks := make([]DueResourceHealthCheck, 0, limit)
	err := db.NewSelect().
		TableExpr("resource_health_checks AS health_check").
		ColumnExpr("health_check.*").
		ColumnExpr("resource.id AS resource_id").
		ColumnExpr("resource.name AS resource_name").
		ColumnExpr("resource.kind AS resource_kind").
		ColumnExpr("COALESCE(endpoint.settings ->> 'database', '') AS resource_database_name").
		ColumnExpr("COALESCE(endpoint.address, '') AS endpoint_address").
		ColumnExpr("COALESCE(endpoint.port, 0) AS endpoint_port").
		ColumnExpr("COALESCE(endpoint.protocol, '') AS endpoint_protocol").
		ColumnExpr("COALESCE(endpoint.tls_mode, '') AS endpoint_tls_mode").
		ColumnExpr("COALESCE(endpoint.settings, '{}'::jsonb) AS endpoint_settings").
		ColumnExpr("credential.username AS credential_username").
		ColumnExpr("credential.enc_payload AS credential_encrypted_payload").
		ColumnExpr("status.health_check_id IS NOT NULL AS status_present").
		ColumnExpr("COALESCE(status.state, 'unknown') AS status_state").
		ColumnExpr("COALESCE(status.consecutive_successes, 0) AS status_consecutive_successes").
		ColumnExpr("COALESCE(status.consecutive_failures, 0) AS status_consecutive_failures").
		ColumnExpr("status.expires_at AS status_expires_at").
		Join("JOIN resources AS resource ON resource.id = health_check.resource_id AND resource.archived_at IS NULL").
		Join("LEFT JOIN resource_installations AS installation ON installation.id = health_check.resource_installation_id AND installation.resource_id = resource.id AND installation.archived_at IS NULL").
		Join("LEFT JOIN resource_endpoints AS endpoint ON endpoint.id = health_check.resource_endpoint_id AND endpoint.resource_id = resource.id AND endpoint.archived_at IS NULL").
		Join("LEFT JOIN resource_credentials AS credential ON credential.id = health_check.resource_credential_id AND credential.resource_id = resource.id AND credential.archived_at IS NULL").
		Join("LEFT JOIN resource_health_check_statuses AS status ON status.health_check_id = health_check.id").
		Where("resource.kind IN ('postgresql', 'clickhouse')").
		Where("health_check.kind = resource.kind").
		Where("health_check.enabled = TRUE").
		Where("health_check.archived_at IS NULL").
		Where("status.health_check_id IS NULL OR status.observed_at + health_check.interval_seconds * INTERVAL '1 second' <= ?", now).
		OrderExpr("COALESCE(status.observed_at, health_check.created_at), health_check.id").
		Limit(limit).
		Scan(ctx, &checks)
	return checks, err
}

type CreateResourceHealthCheckData struct {
	Name                   string
	Kind                   string
	Configuration          json.RawMessage
	IntervalSeconds        int32
	TimeoutSeconds         int32
	FailureThreshold       int32
	SuccessThreshold       int32
	Enabled                bool
	ArchivedAt             sql.NullTime
	ResourceID             uuid.UUID
	ResourceInstallationID *uuid.UUID
	ResourceEndpointID     *uuid.UUID
	ResourceCredentialID   *uuid.UUID
}

func (rhc resourceHealthCheck) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceHealthCheckData,
) (ResourceHealthCheckEntity, error) {
	entity := ResourceHealthCheckEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Kind:                   data.Kind,
		Configuration:          data.Configuration,
		IntervalSeconds:        data.IntervalSeconds,
		TimeoutSeconds:         data.TimeoutSeconds,
		FailureThreshold:       data.FailureThreshold,
		SuccessThreshold:       data.SuccessThreshold,
		Enabled:                data.Enabled,
		ArchivedAt:             data.ArchivedAt,
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		ResourceEndpointID:     data.ResourceEndpointID,
		ResourceCredentialID:   data.ResourceCredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceHealthCheckEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "resource-health-check:"+entity.ResourceID.String()+":"+strings.ToLower(entity.Name), entity.ID, db.NewSelect().Model((*ResourceHealthCheckEntity)(nil)).Where("resource_id = ?", entity.ResourceID).Where("lower(name) = ?", strings.ToLower(entity.Name)), "name", "an active health check already uses this name on the Resource"); err != nil {
		return ResourceHealthCheckEntity{}, err
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceHealthCheckEntity{}, err
	}

	return entity, nil
}

type UpdateResourceHealthCheckData struct {
	ID                     uuid.UUID
	UpdatedAt              time.Time
	Name                   string
	Kind                   string
	Configuration          json.RawMessage
	IntervalSeconds        int32
	TimeoutSeconds         int32
	FailureThreshold       int32
	SuccessThreshold       int32
	Enabled                bool
	ArchivedAt             sql.NullTime
	ResourceID             uuid.UUID
	ResourceInstallationID *uuid.UUID
	ResourceEndpointID     *uuid.UUID
	ResourceCredentialID   *uuid.UUID
}

func (rhc resourceHealthCheck) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateResourceHealthCheckData,
) (ResourceHealthCheckEntity, error) {
	entity := ResourceHealthCheckEntity{
		ID:                     data.ID,
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Kind:                   data.Kind,
		Configuration:          data.Configuration,
		IntervalSeconds:        data.IntervalSeconds,
		TimeoutSeconds:         data.TimeoutSeconds,
		FailureThreshold:       data.FailureThreshold,
		SuccessThreshold:       data.SuccessThreshold,
		Enabled:                data.Enabled,
		ArchivedAt:             data.ArchivedAt,
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		ResourceEndpointID:     data.ResourceEndpointID,
		ResourceCredentialID:   data.ResourceCredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceHealthCheckEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "resource-health-check:"+entity.ResourceID.String()+":"+strings.ToLower(entity.Name), entity.ID, db.NewSelect().Model((*ResourceHealthCheckEntity)(nil)).Where("resource_id = ?", entity.ResourceID).Where("lower(name) = ?", strings.ToLower(entity.Name)), "name", "an active health check already uses this name on the Resource"); err != nil {
		return ResourceHealthCheckEntity{}, err
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("kind").
		Column("configuration").
		Column("interval_seconds").
		Column("timeout_seconds").
		Column("failure_threshold").
		Column("success_threshold").
		Column("enabled").
		Column("archived_at").
		Column("resource_id").
		Column("resource_installation_id").
		Column("resource_endpoint_id").
		Column("resource_credential_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceHealthCheckEntity{}, err
	}

	return entity, nil
}

func (rhc resourceHealthCheck) Destroy(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) error {
	_, err := db.NewDelete().
		Model((*ResourceHealthCheckEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (rhc resourceHealthCheck) All(
	ctx context.Context,
	db storage.Executor,
) ([]ResourceHealthCheckEntity, error) {
	var entities []ResourceHealthCheckEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceHealthChecks struct {
	ResourceHealthChecks []ResourceHealthCheckEntity
	TotalCount           int64
	Page                 int64
	PageSize             int64
	TotalPages           int64
}

func (rhc resourceHealthCheck) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedResourceHealthChecks, error) {
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
		Model(&ResourceHealthCheckEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceHealthChecks{}, err
	}

	entities := make([]ResourceHealthCheckEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceHealthChecks{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceHealthChecks{
		ResourceHealthChecks: entities,
		TotalCount:           int64(totalCount),
		Page:                 page,
		PageSize:             pageSize,
		TotalPages:           totalPages,
	}, nil
}

func (rhc resourceHealthCheck) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceHealthCheckData,
) (ResourceHealthCheckEntity, error) {
	entity := ResourceHealthCheckEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Kind:                   data.Kind,
		Configuration:          data.Configuration,
		IntervalSeconds:        data.IntervalSeconds,
		TimeoutSeconds:         data.TimeoutSeconds,
		FailureThreshold:       data.FailureThreshold,
		SuccessThreshold:       data.SuccessThreshold,
		Enabled:                data.Enabled,
		ArchivedAt:             data.ArchivedAt,
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		ResourceEndpointID:     data.ResourceEndpointID,
		ResourceCredentialID:   data.ResourceCredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceHealthCheckEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "resource-health-check:"+entity.ResourceID.String()+":"+strings.ToLower(entity.Name), entity.ID, db.NewSelect().Model((*ResourceHealthCheckEntity)(nil)).Where("resource_id = ?", entity.ResourceID).Where("lower(name) = ?", strings.ToLower(entity.Name)), "name", "an active health check already uses this name on the Resource"); err != nil {
		return ResourceHealthCheckEntity{}, err
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("kind = excluded.kind").
		Set("configuration = excluded.configuration").
		Set("interval_seconds = excluded.interval_seconds").
		Set("timeout_seconds = excluded.timeout_seconds").
		Set("failure_threshold = excluded.failure_threshold").
		Set("success_threshold = excluded.success_threshold").
		Set("enabled = excluded.enabled").
		Set("archived_at = excluded.archived_at").
		Set("resource_id = excluded.resource_id").
		Set("resource_installation_id = excluded.resource_installation_id").
		Set("resource_endpoint_id = excluded.resource_endpoint_id").
		Set("resource_credential_id = excluded.resource_credential_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceHealthCheckEntity{}, err
	}

	return entity, nil
}
