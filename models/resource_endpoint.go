package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ResourceEndpointEntity struct {
	bun.BaseModel          `bun:"table:resource_endpoints,alias:resource_endpoints"`
	ID                     uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt              time.Time       `bun:"created_at"`
	UpdatedAt              time.Time       `bun:"updated_at"`
	Name                   string          `bun:"name"`
	Role                   string          `bun:"role"`
	Address                string          `bun:"address"`
	Port                   int32           `bun:"port"`
	Protocol               string          `bun:"protocol"`
	TlsMode                string          `bun:"tls_mode"`
	Settings               json.RawMessage `bun:"settings,type:jsonb"`
	ArchivedAt             sql.NullTime    `bun:"archived_at"`
	ResourceID             uuid.UUID       `bun:"resource_id,type:uuid"`
	ResourceInstallationID *uuid.UUID      `bun:"resource_installation_id,type:uuid"`
	PrivateNetworkID       *uuid.UUID      `bun:"private_network_id,type:uuid"`
}

func (e *ResourceEndpointEntity) Validate() error {
	builder := validation.NewBuilder()
	builder.Required("name", strings.TrimSpace(e.Name))
	if e.Role != "primary" && e.Role != "wireguard" {
		builder.Add("role", "unsupported", "role must be primary or wireguard")
	}
	builder.Required("address", strings.TrimSpace(e.Address))
	if e.Port < 1 || e.Port > 65535 {
		builder.Add("port", "range", "port must be between 1 and 65535")
	}
	if e.Protocol == "" {
		builder.Add("protocol", "required", "protocol is required")
	}
	switch e.TlsMode {
	case "disable", "prefer", "require", "verify-ca", "verify-full":
	default:
		builder.Add("tlsMode", "unsupported", "TLS mode is not supported")
	}
	if len(e.Settings) == 0 || !json.Valid(e.Settings) {
		builder.Add("settings", "invalid", "settings must be valid JSON")
	} else if settingsContainSecret(e.Settings) {
		builder.Add("settings", "secret", "settings must not contain raw credentials")
	}
	if e.ResourceID == uuid.Nil {
		builder.Add("resourceId", "required", "resource is required")
	}
	return builder.Err()
}

func settingsContainSecret(settings json.RawMessage) bool {
	var value any
	if json.Unmarshal(settings, &value) != nil {
		return false
	}
	var contains func(any) bool
	contains = func(candidate any) bool {
		switch typed := candidate.(type) {
		case map[string]any:
			for key, nested := range typed {
				normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
				for _, forbidden := range []string{"password", "secret", "token", "private_key", "credential"} {
					if strings.Contains(normalized, forbidden) {
						return true
					}
				}
				if contains(nested) {
					return true
				}
			}
		case []any:
			if slices.ContainsFunc(typed, contains) {
				return true
			}
		}
		return false
	}
	return contains(value)
}

func (re resourceEndpoint) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ResourceEndpointEntity, error) {
	var entity ResourceEndpointEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return ResourceEndpointEntity{}, err
	}

	return entity, nil
}

type CreateResourceEndpointData struct {
	Name                   string
	Role                   string
	Address                string
	Port                   int32
	Protocol               string
	TlsMode                string
	Settings               json.RawMessage
	ArchivedAt             sql.NullTime
	ResourceID             uuid.UUID
	ResourceInstallationID *uuid.UUID
	PrivateNetworkID       *uuid.UUID
}

func (re resourceEndpoint) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceEndpointData,
) (ResourceEndpointEntity, error) {
	entity := ResourceEndpointEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Role:                   data.Role,
		Address:                data.Address,
		Port:                   data.Port,
		Protocol:               data.Protocol,
		TlsMode:                data.TlsMode,
		Settings:               data.Settings,
		ArchivedAt:             data.ArchivedAt,
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		PrivateNetworkID:       data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEndpointEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return ResourceEndpointEntity{}, err
	}

	return entity, nil
}

func (re resourceEndpoint) CreateForSystemResource(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceEndpointData,
) (ResourceEndpointEntity, error) {
	var resource struct {
		Kind string `bun:"kind"`
	}
	err := db.NewSelect().
		TableExpr("resources AS resource").
		ColumnExpr("resource.kind AS kind").
		Join("JOIN environment_resources AS binding ON binding.resource_id = resource.id AND binding.archived_at IS NULL").
		Join("JOIN environments AS environment ON environment.id = binding.environment_id AND environment.archived_at IS NULL").
		Join("JOIN applications AS application ON application.id = environment.application_id AND application.archived_at IS NULL").
		Where("resource.id = ?", data.ResourceID).
		Where("resource.archived_at IS NULL").
		Where("application.slug = ?", SystemApplicationSlug).
		Limit(1).
		Scan(ctx, &resource)
	if errors.Is(err, sql.ErrNoRows) {
		return ResourceEndpointEntity{}, ErrNotFound
	}
	if err != nil {
		return ResourceEndpointEntity{}, err
	}
	if !resourceSupportsProtocol(resource.Kind, data.Protocol) {
		return ResourceEndpointEntity{}, errors.Join(
			ErrDomainValidation,
			validation.ValidationErrors{{Field: "protocol", Code: "unsupported", Message: fmt.Sprintf("protocol %q is not supported by %s", data.Protocol, resource.Kind)}},
		)
	}
	if data.ResourceInstallationID != nil {
		count, countErr := db.NewSelect().TableExpr("resource_installations").
			Where("id = ?", *data.ResourceInstallationID).
			Where("resource_id = ?", data.ResourceID).
			Where("archived_at IS NULL").
			Count(ctx)
		if countErr != nil {
			return ResourceEndpointEntity{}, countErr
		}
		if count != 1 {
			return ResourceEndpointEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "resourceInstallationId", Code: "mismatch", Message: "installation must belong to this resource"}})
		}
	}
	if data.Role == "wireguard" {
		if data.PrivateNetworkID == nil {
			return ResourceEndpointEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "privateNetworkId", Code: "required", Message: "WireGuard endpoints require a private network"}})
		}
		var cidr string
		err = db.NewSelect().TableExpr("server_networks AS server_network").
			ColumnExpr("server_network.configuration ->> 'cidr'").
			Where("server_network.private_network_id = ?", *data.PrivateNetworkID).
			Where("server_network.driver = 'wireguard'").
			Where("server_network.removed_at IS NULL").
			Limit(1).
			Scan(ctx, &cidr)
		address, addressErr := netip.ParseAddr(strings.TrimSpace(data.Address))
		network, networkErr := netip.ParsePrefix(strings.TrimSpace(cidr))
		if err != nil || addressErr != nil || networkErr != nil || !network.Contains(address) {
			return ResourceEndpointEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "address", Code: "network", Message: "WireGuard endpoint address must be reachable through the selected private network"}})
		}
	}
	return re.Create(ctx, db, data)
}

func resourceSupportsProtocol(kind, protocol string) bool {
	supported := map[string][]string{
		"postgresql":    {"postgresql"},
		"redis":         {"redis"},
		"elasticsearch": {"http", "https"},
		"clickhouse":    {"http", "https", "clickhouse"},
	}
	return slices.Contains(supported[kind], protocol)
}

type UpdateResourceEndpointData struct {
	ID                     uuid.UUID
	UpdatedAt              time.Time
	Name                   string
	Role                   string
	Address                string
	Port                   int32
	Protocol               string
	TlsMode                string
	Settings               json.RawMessage
	ArchivedAt             sql.NullTime
	ResourceID             uuid.UUID
	ResourceInstallationID *uuid.UUID
	PrivateNetworkID       *uuid.UUID
}

func (re resourceEndpoint) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateResourceEndpointData,
) (ResourceEndpointEntity, error) {
	entity := ResourceEndpointEntity{
		ID:                     data.ID,
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Role:                   data.Role,
		Address:                data.Address,
		Port:                   data.Port,
		Protocol:               data.Protocol,
		TlsMode:                data.TlsMode,
		Settings:               data.Settings,
		ArchivedAt:             data.ArchivedAt,
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		PrivateNetworkID:       data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEndpointEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("role").
		Column("address").
		Column("port").
		Column("protocol").
		Column("tls_mode").
		Column("settings").
		Column("archived_at").
		Column("resource_id").
		Column("resource_installation_id").
		Column("private_network_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceEndpointEntity{}, err
	}

	return entity, nil
}

func (re resourceEndpoint) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*ResourceEndpointEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (re resourceEndpoint) All(
	ctx context.Context,
	db storage.Executor,
) ([]ResourceEndpointEntity, error) {
	var entities []ResourceEndpointEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedResourceEndpoints struct {
	ResourceEndpoints []ResourceEndpointEntity
	TotalCount        int64
	Page              int64
	PageSize          int64
	TotalPages        int64
}

func (re resourceEndpoint) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedResourceEndpoints, error) {
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
		Model(&ResourceEndpointEntity{}).Count(ctx)
	if err != nil {
		return PaginatedResourceEndpoints{}, err
	}

	entities := make([]ResourceEndpointEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedResourceEndpoints{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedResourceEndpoints{
		ResourceEndpoints: entities,
		TotalCount:        int64(totalCount),
		Page:              page,
		PageSize:          pageSize,
		TotalPages:        totalPages,
	}, nil
}

func (re resourceEndpoint) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceEndpointData,
) (ResourceEndpointEntity, error) {
	entity := ResourceEndpointEntity{
		ID:                     uuid.New(),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		Name:                   data.Name,
		Role:                   data.Role,
		Address:                data.Address,
		Port:                   data.Port,
		Protocol:               data.Protocol,
		TlsMode:                data.TlsMode,
		Settings:               data.Settings,
		ArchivedAt:             data.ArchivedAt,
		ResourceID:             data.ResourceID,
		ResourceInstallationID: data.ResourceInstallationID,
		PrivateNetworkID:       data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEndpointEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("role = excluded.role").
		Set("address = excluded.address").
		Set("port = excluded.port").
		Set("protocol = excluded.protocol").
		Set("tls_mode = excluded.tls_mode").
		Set("settings = excluded.settings").
		Set("archived_at = excluded.archived_at").
		Set("resource_id = excluded.resource_id").
		Set("resource_installation_id = excluded.resource_installation_id").
		Set("private_network_id = excluded.private_network_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceEndpointEntity{}, err
	}

	return entity, nil
}
