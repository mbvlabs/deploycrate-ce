package models

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type CustomCaddyRouteEntity struct {
	bun.BaseModel  `bun:"table:custom_caddy_routes,alias:custom_caddy_routes"`
	ID             uuid.UUID           `bun:"id,pk,type:uuid"`
	CreatedAt      time.Time           `bun:"created_at"`
	UpdatedAt      time.Time           `bun:"updated_at"`
	ExternalID     string              `bun:"external_id"`
	Hostname       string              `bun:"hostname"`
	OriginAddress  string              `bun:"origin_address"`
	OriginPort     int32               `bun:"origin_port"`
	OriginProtocol string              `bun:"origin_protocol"`
	OriginTLSMode  string              `bun:"origin_tls_mode"`
	HealthPath     string              `bun:"health_path"`
	State          CaddyRouteStateEnum `bun:"state"`
	LastError      sql.NullString      `bun:"last_error"`
	AppliedAt      sql.NullTime        `bun:"applied_at"`
	ObservedAt     sql.NullTime        `bun:"observed_at"`
	RemovedAt      sql.NullTime        `bun:"removed_at"`
}

func (entity *CustomCaddyRouteEntity) Validate() error {
	entity.ExternalID = strings.TrimSpace(entity.ExternalID)
	entity.Hostname = NormalizeHostname(entity.Hostname)
	entity.OriginAddress = strings.TrimSpace(entity.OriginAddress)
	entity.OriginProtocol = strings.ToLower(strings.TrimSpace(entity.OriginProtocol))
	entity.OriginTLSMode = strings.ToLower(strings.TrimSpace(entity.OriginTLSMode))
	entity.HealthPath = strings.TrimSpace(entity.HealthPath)
	builder := validation.NewBuilder()
	builder.Required("externalId", entity.ExternalID)
	if !IsValidHostname(entity.Hostname) {
		builder.Add("hostname", "format", "hostname must be a valid fully qualified domain name")
	}
	builder.Required("originAddress", entity.OriginAddress)
	if entity.OriginPort < 1 || entity.OriginPort > 65535 {
		builder.Add("originPort", "range", "origin port must be between 1 and 65535")
	}
	if entity.OriginProtocol != "http" && entity.OriginProtocol != "https" {
		builder.Add("originProtocol", "unsupported", "origin protocol must be HTTP or HTTPS")
	}
	switch entity.OriginTLSMode {
	case "disable", "prefer", "require", "verify-ca", "verify-full":
	default:
		builder.Add("originTlsMode", "unsupported", "origin TLS mode is not supported")
	}
	if !entity.State.IsValid() {
		builder.Add("state", "unsupported", "Caddy route state is not supported")
	}
	return builder.Err()
}

type SaveCustomCaddyRouteData struct {
	ID             uuid.UUID
	ExternalID     string
	Hostname       string
	OriginAddress  string
	OriginPort     int32
	OriginProtocol string
	OriginTLSMode  string
	HealthPath     string
	State          CaddyRouteStateEnum
	LastError      sql.NullString
	AppliedAt      sql.NullTime
	ObservedAt     sql.NullTime
	RemovedAt      sql.NullTime
}

func (customCaddyRoute) Create(ctx context.Context, db storage.Executor, data SaveCustomCaddyRouteData) (CustomCaddyRouteEntity, error) {
	now := time.Now().UTC()
	entity := CustomCaddyRouteEntity{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, ExternalID: data.ExternalID,
		Hostname: data.Hostname, OriginAddress: data.OriginAddress, OriginPort: data.OriginPort,
		OriginProtocol: data.OriginProtocol, OriginTLSMode: data.OriginTLSMode,
		HealthPath: data.HealthPath, State: data.State,
	}
	if err := validation.Validate(&entity); err != nil {
		return CustomCaddyRouteEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return CustomCaddyRouteEntity{}, err
	}
	return entity, nil
}

func (customCaddyRoute) Update(ctx context.Context, db storage.Executor, data SaveCustomCaddyRouteData) (CustomCaddyRouteEntity, error) {
	entity := CustomCaddyRouteEntity{
		ID: data.ID, UpdatedAt: time.Now().UTC(), ExternalID: data.ExternalID,
		Hostname: data.Hostname, OriginAddress: data.OriginAddress, OriginPort: data.OriginPort,
		OriginProtocol: data.OriginProtocol, OriginTLSMode: data.OriginTLSMode,
		HealthPath: data.HealthPath, State: data.State, LastError: data.LastError,
		AppliedAt: data.AppliedAt, ObservedAt: data.ObservedAt, RemovedAt: data.RemovedAt,
	}
	if err := validation.Validate(&entity); err != nil {
		return CustomCaddyRouteEntity{}, errors.Join(ErrDomainValidation, err)
	}
	err := db.NewUpdate().Model(&entity).
		Column("updated_at", "external_id", "hostname", "origin_address", "origin_port", "origin_protocol", "origin_tls_mode", "health_path", "state", "last_error", "applied_at", "observed_at", "removed_at").
		WherePK().Returning("*").Scan(ctx)
	return entity, err
}

func (customCaddyRoute) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (CustomCaddyRouteEntity, error) {
	var entity CustomCaddyRouteEntity
	err := db.NewSelect().Model(&entity).Where("id = ?", id).Scan(ctx)
	return entity, err
}

func (customCaddyRoute) FindActiveByExternalID(ctx context.Context, db storage.Executor, externalID string) (CustomCaddyRouteEntity, error) {
	var entity CustomCaddyRouteEntity
	err := db.NewSelect().Model(&entity).Where("external_id = ?", strings.TrimSpace(externalID)).
		Where("removed_at IS NULL").Limit(1).Scan(ctx)
	return entity, err
}

func (customCaddyRoute) Active(ctx context.Context, db storage.Executor) ([]CustomCaddyRouteEntity, error) {
	rows := make([]CustomCaddyRouteEntity, 0)
	err := db.NewSelect().Model(&rows).Where("removed_at IS NULL OR state = ?", CaddyRouteRemovalPending).
		OrderExpr("hostname").Scan(ctx)
	return rows, err
}
