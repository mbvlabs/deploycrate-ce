package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ResourceEndpointEntity struct {
	bun.BaseModel    `bun:"table:resource_endpoints,alias:resource_endpoints"`
	ID               uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt        time.Time       `bun:"created_at"`
	UpdatedAt        time.Time       `bun:"updated_at"`
	Name             string          `bun:"name"`
	Role             string          `bun:"role"`
	Address          string          `bun:"address"`
	Port             int32           `bun:"port"`
	Protocol         string          `bun:"protocol"`
	TlsMode          string          `bun:"tls_mode"`
	Settings         json.RawMessage `bun:"settings,type:jsonb"`
	ArchivedAt       sql.NullTime    `bun:"archived_at"`
	ResourceID       uuid.UUID       `bun:"resource_id,type:uuid"`
	PrivateNetworkID *uuid.UUID      `bun:"private_network_id,type:uuid"`
}

const (
	ResourceEndpointExposureSystem      = "system"
	ResourceEndpointExposureEnvironment = "environment"
	ResourceEndpointExposurePublic      = "public"
	ResourceEndpointAuthNone            = "none"
	ResourceEndpointAuthSignedIdentity  = "signed_identity"
	ResourceEndpointTransportOTLPHTTP   = "http/protobuf"
	ResourceEndpointAudienceLocalSystem = "local_system"
	ResourceEndpointAudienceEnvironment = "environment"
	ResourceEndpointAudiencePublic      = "public"
	ResourceEndpointAudienceCustom      = "custom"
	ResourceEndpointAddressLoopback     = "system_loopback"
	ResourceEndpointAddressWireGuard    = "server_wireguard"
	ResourceEndpointAddressCaddy        = "caddy"
	ResourceEndpointAddressManual       = "manual"
)

type ResourceEndpointSettings struct {
	Exposure       string                         `json:"exposure"`
	Transport      string                         `json:"transport"`
	Authentication string                         `json:"authentication"`
	Audience       string                         `json:"audience"`
	AddressSource  string                         `json:"address_source"`
	Caddy          *ResourceEndpointCaddySettings `json:"caddy,omitempty"`
}

type ResourceEndpointCaddySettings struct {
	Managed                bool       `json:"managed"`
	HealthPath             string     `json:"health_path"`
	OriginAddress          string     `json:"origin_address"`
	OriginPort             int32      `json:"origin_port"`
	OriginProtocol         string     `json:"origin_protocol"`
	OriginTLSMode          string     `json:"origin_tls_mode"`
	OriginPrivateNetworkID *uuid.UUID `json:"origin_private_network_id,omitempty"`
}

func (e ResourceEndpointEntity) ParsedSettings() ResourceEndpointSettings {
	var settings ResourceEndpointSettings
	_ = json.Unmarshal(e.Settings, &settings)
	settings.Exposure = strings.ToLower(strings.TrimSpace(settings.Exposure))
	settings.Transport = strings.ToLower(strings.TrimSpace(settings.Transport))
	settings.Authentication = strings.ToLower(strings.TrimSpace(settings.Authentication))
	settings.Audience = strings.ToLower(strings.TrimSpace(settings.Audience))
	settings.AddressSource = strings.ToLower(strings.TrimSpace(settings.AddressSource))
	if settings.Caddy != nil {
		settings.Caddy.HealthPath = strings.TrimSpace(settings.Caddy.HealthPath)
		settings.Caddy.OriginAddress = strings.TrimSpace(settings.Caddy.OriginAddress)
		settings.Caddy.OriginProtocol = strings.ToLower(strings.TrimSpace(settings.Caddy.OriginProtocol))
		settings.Caddy.OriginTLSMode = strings.ToLower(strings.TrimSpace(settings.Caddy.OriginTLSMode))
	}
	if settings.Audience == "" {
		switch settings.Exposure {
		case ResourceEndpointExposureEnvironment:
			settings.Audience = ResourceEndpointAudienceEnvironment
		case ResourceEndpointExposureSystem:
			settings.Audience = ResourceEndpointAudienceLocalSystem
		default:
			settings.Audience = ResourceEndpointAudienceCustom
		}
	}
	if settings.AddressSource == "" {
		if e.PrivateNetworkID != nil {
			settings.AddressSource = ResourceEndpointAddressWireGuard
		} else if slices.Contains([]string{"127.0.0.1", "::1", "localhost"}, strings.ToLower(e.Address)) {
			settings.AddressSource = ResourceEndpointAddressLoopback
		} else {
			settings.AddressSource = ResourceEndpointAddressManual
		}
	}
	return settings
}

func (e ResourceEndpointEntity) IsEnvironmentEndpoint() bool {
	settings := e.ParsedSettings()
	return settings.Audience == ResourceEndpointAudienceEnvironment || settings.Audience == ResourceEndpointAudiencePublic
}

func (e ResourceEndpointEntity) URL() string {
	return (&url.URL{Scheme: e.Protocol, Host: net.JoinHostPort(e.Address, strconv.Itoa(int(e.Port)))}).String()
}

func (e *ResourceEndpointEntity) Validate() error {
	e.Name = strings.TrimSpace(e.Name)
	e.Role = strings.ToLower(strings.TrimSpace(e.Role))
	e.Address = strings.TrimSpace(e.Address)
	e.Protocol = strings.ToLower(strings.TrimSpace(e.Protocol))
	e.TlsMode = strings.ToLower(strings.TrimSpace(e.TlsMode))
	builder := validation.NewBuilder()
	builder.Required("name", e.Name)
	builder.Required("role", e.Role)
	builder.Required("address", e.Address)
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

func (e *ResourceEndpointEntity) ValidateForKind(kind string) error {
	if err := e.Validate(); err != nil {
		return err
	}
	definition, ok := FindResourceEngine(kind)
	if !ok {
		return validation.ValidationErrors{{Field: "kind", Code: "unsupported", Message: "resource kind is not supported"}}
	}
	builder := validation.NewBuilder()
	if !definition.SupportsEndpointRole(e.Role) {
		builder.Add("role", "unsupported", "endpoint role is not supported by this resource kind")
	}
	settings := e.ParsedSettings()
	validatedProtocol := e.Protocol
	if settings.Caddy != nil && settings.Caddy.Managed {
		validatedProtocol = settings.Caddy.OriginProtocol
	}
	if !definition.SupportsProtocol(validatedProtocol) {
		builder.Add("protocol", "unsupported", "protocol is not supported by this resource kind")
	}
	if definition.Engine == "opentelemetry" {
		if settings.Transport != ResourceEndpointTransportOTLPHTTP {
			builder.Add("settings.transport", "unsupported", "OpenTelemetry endpoints require the HTTP/protobuf transport")
		}
		switch settings.Exposure {
		case ResourceEndpointExposureSystem:
			if settings.Authentication != ResourceEndpointAuthNone {
				builder.Add("settings.authentication", "unsupported", "system-only OpenTelemetry endpoints must not require workload authentication")
			}
		case ResourceEndpointExposureEnvironment:
			if settings.Authentication != ResourceEndpointAuthSignedIdentity {
				builder.Add("settings.authentication", "unsupported", "Environment OpenTelemetry endpoints require signed identity authentication")
			}
			if e.PrivateNetworkID == nil {
				builder.Add("privateNetworkId", "required", "Environment OpenTelemetry endpoints require a private network")
			}
		case ResourceEndpointExposurePublic:
			if settings.Authentication != ResourceEndpointAuthSignedIdentity {
				builder.Add("settings.authentication", "unsupported", "public OpenTelemetry endpoints require signed identity authentication")
			}
		default:
			builder.Add("settings.exposure", "unsupported", "OpenTelemetry endpoint exposure is not supported")
		}
	}
	switch settings.Audience {
	case ResourceEndpointAudienceLocalSystem, ResourceEndpointAudienceEnvironment, ResourceEndpointAudiencePublic, ResourceEndpointAudienceCustom:
	default:
		builder.Add("settings.audience", "unsupported", "endpoint audience is not supported")
	}
	switch settings.AddressSource {
	case ResourceEndpointAddressLoopback, ResourceEndpointAddressWireGuard, ResourceEndpointAddressCaddy, ResourceEndpointAddressManual:
	default:
		builder.Add("settings.address_source", "unsupported", "endpoint address source is not supported")
	}
	if settings.Audience == ResourceEndpointAudienceEnvironment && e.PrivateNetworkID == nil {
		builder.Add("privateNetworkId", "required", "Environment endpoints require a private network")
	}
	if settings.AddressSource == ResourceEndpointAddressWireGuard && e.PrivateNetworkID == nil {
		builder.Add("privateNetworkId", "required", "WireGuard address sources require a private network")
	}
	if settings.Audience == ResourceEndpointAudiencePublic || settings.AddressSource == ResourceEndpointAddressCaddy {
		if settings.Caddy == nil || !settings.Caddy.Managed {
			builder.Add("settings.caddy", "required", "public endpoints require managed Caddy settings")
		}
	}
	if settings.Caddy != nil && settings.Caddy.Managed {
		if !IsValidHostname(e.Address) {
			builder.Add("address", "format", "managed Caddy endpoints require a valid hostname")
		}
		if settings.Caddy.OriginAddress == "" {
			builder.Add("settings.caddy.origin_address", "required", "Caddy origin address is required")
		}
		if settings.Caddy.OriginPort < 1 || settings.Caddy.OriginPort > 65535 {
			builder.Add("settings.caddy.origin_port", "range", "Caddy origin port must be between 1 and 65535")
		}
		if settings.Caddy.OriginProtocol == "" {
			builder.Add("settings.caddy.origin_protocol", "required", "Caddy origin protocol is required")
		}
		if settings.Caddy.HealthPath != "" && !strings.HasPrefix(settings.Caddy.HealthPath, "/") {
			builder.Add("settings.caddy.health_path", "format", "Caddy health path must start with /")
		}
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
				for _, forbidden := range []string{"password", "secret", "token", "private_key"} {
					if strings.Contains(normalized, forbidden) {
						return true
					}
				}
				if normalized == "credential" || normalized == "credentials" ||
					strings.HasSuffix(normalized, "_credential") || strings.HasSuffix(normalized, "_credentials") {
					return true
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

func (re resourceEndpoint) FindSystemEnvironmentEndpoint(
	ctx context.Context,
	db storage.Executor,
	engine string,
) (ResourceEndpointEntity, error) {
	endpoints := make([]ResourceEndpointEntity, 0, 1)
	if err := db.NewSelect().Model(&endpoints).
		Join("JOIN resources AS resource ON resource.id = resource_endpoints.resource_id AND resource.archived_at IS NULL").
		Where("resource.system_managed = TRUE").Where("resource.configuration ->> 'engine' = ?", strings.ToLower(strings.TrimSpace(engine))).
		Where("resource_endpoints.settings ->> 'exposure' = ?", ResourceEndpointExposureEnvironment).
		Where(`resource_endpoints.private_network_id IN (
			SELECT membership.private_network_id
			FROM environment_networks AS membership
			JOIN environments AS environment ON environment.id = membership.environment_id AND environment.archived_at IS NULL
			JOIN applications AS application ON application.id = environment.application_id AND application.archived_at IS NULL
			WHERE application.slug = ? AND membership.removed_at IS NULL
		)`, SystemApplicationSlug).
		Where("resource_endpoints.archived_at IS NULL").OrderExpr("resource_endpoints.created_at").Scan(ctx); err != nil {
		return ResourceEndpointEntity{}, err
	}
	if len(endpoints) != 1 {
		return ResourceEndpointEntity{}, fmt.Errorf("exactly one system %s Environment endpoint is required", engine)
	}
	return endpoints[0], nil
}

type CreateResourceEndpointData struct {
	Name             string
	Role             string
	Address          string
	Port             int32
	Protocol         string
	TlsMode          string
	Settings         json.RawMessage
	ArchivedAt       sql.NullTime
	ResourceID       uuid.UUID
	PrivateNetworkID *uuid.UUID
}

func (re resourceEndpoint) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateResourceEndpointData,
) (ResourceEndpointEntity, error) {
	entity := ResourceEndpointEntity{
		ID:               uuid.New(),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		Name:             data.Name,
		Role:             data.Role,
		Address:          data.Address,
		Port:             data.Port,
		Protocol:         data.Protocol,
		TlsMode:          data.TlsMode,
		Settings:         data.Settings,
		ArchivedAt:       data.ArchivedAt,
		ResourceID:       data.ResourceID,
		PrivateNetworkID: data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEndpointEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "resource-endpoint:"+entity.ResourceID.String()+":"+strings.ToLower(entity.Name), entity.ID, db.NewSelect().Model((*ResourceEndpointEntity)(nil)).Where("resource_id = ?", entity.ResourceID).Where("lower(name) = ?", strings.ToLower(entity.Name)), "name", "an active endpoint already uses this name"); err != nil {
		return ResourceEndpointEntity{}, err
	}
	if err := ensureCaddyEndpointHostnameUnique(ctx, db, entity); err != nil {
		return ResourceEndpointEntity{}, err
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
		ResourceType  ResourceTypeEnum `bun:"resource_type"`
		Configuration json.RawMessage  `bun:"configuration"`
	}
	err := db.NewSelect().
		TableExpr("resources AS resource").
		ColumnExpr("resource.resource_type, resource.configuration").
		Where("resource.id = ?", data.ResourceID).
		Where("resource.archived_at IS NULL").
		Where("resource.system_managed = TRUE").
		Limit(1).
		Scan(ctx, &resource)
	if errors.Is(err, sql.ErrNoRows) {
		return ResourceEndpointEntity{}, ErrNotFound
	}
	if err != nil {
		return ResourceEndpointEntity{}, err
	}
	var configuration ResourceConfiguration
	_ = json.Unmarshal(resource.Configuration, &configuration)
	entity := ResourceEndpointEntity{
		Name: data.Name, Role: data.Role, Address: data.Address, Port: data.Port,
		Protocol: data.Protocol, TlsMode: data.TlsMode, Settings: data.Settings,
		ResourceID: data.ResourceID, PrivateNetworkID: data.PrivateNetworkID,
	}
	if err := entity.ValidateForKind(configuration.Engine); err != nil {
		return ResourceEndpointEntity{}, errors.Join(ErrDomainValidation, err)
	}
	data.Name, data.Role, data.Address = entity.Name, entity.Role, entity.Address
	data.Protocol, data.TlsMode, data.Settings = entity.Protocol, entity.TlsMode, entity.Settings
	if entity.PrivateNetworkID != nil {
		var cidr string
		err = db.NewSelect().TableExpr("server_networks AS server_network").
			ColumnExpr("server_network.configuration ->> 'cidr'").
			Where("server_network.private_network_id = ?", *entity.PrivateNetworkID).
			Where("server_network.driver = 'wireguard'").
			Where("server_network.removed_at IS NULL").
			Limit(1).
			Scan(ctx, &cidr)
		address, addressErr := netip.ParseAddr(strings.TrimSpace(entity.Address))
		network, networkErr := netip.ParsePrefix(strings.TrimSpace(cidr))
		if err != nil || addressErr != nil || networkErr != nil || !network.Contains(address) {
			return ResourceEndpointEntity{}, errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "address", Code: "network", Message: "WireGuard endpoint address must be reachable through the selected private network"}})
		}
	}
	return re.Create(ctx, db, data)
}

func resourceSupportsProtocol(kind, protocol string) bool {
	definition, ok := FindResourceEngine(kind)
	return ok && definition.SupportsProtocol(protocol)
}

type UpdateResourceEndpointData struct {
	ID               uuid.UUID
	UpdatedAt        time.Time
	Name             string
	Role             string
	Address          string
	Port             int32
	Protocol         string
	TlsMode          string
	Settings         json.RawMessage
	ArchivedAt       sql.NullTime
	ResourceID       uuid.UUID
	PrivateNetworkID *uuid.UUID
}

func (re resourceEndpoint) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateResourceEndpointData,
) (ResourceEndpointEntity, error) {
	entity := ResourceEndpointEntity{
		ID:               data.ID,
		UpdatedAt:        time.Now(),
		Name:             data.Name,
		Role:             data.Role,
		Address:          data.Address,
		Port:             data.Port,
		Protocol:         data.Protocol,
		TlsMode:          data.TlsMode,
		Settings:         data.Settings,
		ArchivedAt:       data.ArchivedAt,
		ResourceID:       data.ResourceID,
		PrivateNetworkID: data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEndpointEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "resource-endpoint:"+entity.ResourceID.String()+":"+strings.ToLower(entity.Name), entity.ID, db.NewSelect().Model((*ResourceEndpointEntity)(nil)).Where("resource_id = ?", entity.ResourceID).Where("lower(name) = ?", strings.ToLower(entity.Name)), "name", "an active endpoint already uses this name"); err != nil {
		return ResourceEndpointEntity{}, err
	}
	if err := ensureCaddyEndpointHostnameUnique(ctx, db, entity); err != nil {
		return ResourceEndpointEntity{}, err
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
		Column("private_network_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceEndpointEntity{}, err
	}

	return entity, nil
}

func ensureCaddyEndpointHostnameUnique(ctx context.Context, db storage.Executor, entity ResourceEndpointEntity) error {
	settings := entity.ParsedSettings()
	if entity.ArchivedAt.Valid || settings.Caddy == nil || !settings.Caddy.Managed {
		return nil
	}
	hostname := NormalizeHostname(entity.Address)
	if err := lockUnique(ctx, db, "environment-domain-hostname:"+hostname); err != nil {
		return err
	}
	environmentDomains, err := db.NewSelect().Model((*EnvironmentDomainEntity)(nil)).
		Where("lower(hostname) = ?", hostname).
		Where("archived_at IS NULL").
		Count(ctx)
	if err != nil {
		return err
	}
	resourceEndpoints, err := db.NewSelect().Model((*ResourceEndpointEntity)(nil)).
		Where("id <> ?", entity.ID).
		Where("lower(address) = ?", hostname).
		Where("settings ->> 'address_source' = ?", ResourceEndpointAddressCaddy).
		Where("archived_at IS NULL").
		Count(ctx)
	if err != nil {
		return err
	}
	if environmentDomains+resourceEndpoints > 0 {
		return errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: "hostname", Code: "taken", Message: "an active Caddy route already uses this hostname"}})
	}
	return nil
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
		ID:               uuid.New(),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		Name:             data.Name,
		Role:             data.Role,
		Address:          data.Address,
		Port:             data.Port,
		Protocol:         data.Protocol,
		TlsMode:          data.TlsMode,
		Settings:         data.Settings,
		ArchivedAt:       data.ArchivedAt,
		ResourceID:       data.ResourceID,
		PrivateNetworkID: data.PrivateNetworkID,
	}

	if err := validation.Validate(&entity); err != nil {
		return ResourceEndpointEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(ctx, db, "resource-endpoint:"+entity.ResourceID.String()+":"+strings.ToLower(entity.Name), entity.ID, db.NewSelect().Model((*ResourceEndpointEntity)(nil)).Where("resource_id = ?", entity.ResourceID).Where("lower(name) = ?", strings.ToLower(entity.Name)), "name", "an active endpoint already uses this name"); err != nil {
		return ResourceEndpointEntity{}, err
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
		Set("private_network_id = excluded.private_network_id").
		Returning("*").
		Scan(ctx); err != nil {
		return ResourceEndpointEntity{}, err
	}

	return entity, nil
}
