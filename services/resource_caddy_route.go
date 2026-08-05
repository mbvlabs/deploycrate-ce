package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	caddyclients "deploycrate-ce/clients/caddy"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type ResourceCaddyPublicationInput struct {
	Enabled    bool   `json:"enabled"`
	Hostname   string `json:"hostname"`
	HealthPath string `json:"healthPath"`
}

type ResourceCaddyPublication struct {
	ID                 string `json:"id"`
	ResourceEndpointID string `json:"resourceEndpointId"`
	ExternalID         string `json:"externalId"`
	Hostname           string `json:"hostname"`
	HealthPath         string `json:"healthPath"`
	State              string `json:"state"`
	LastError          string `json:"lastError"`
	AppliedAt          string `json:"appliedAt"`
	ObservedAt         string `json:"observedAt"`
}

type ManagedResourceCaddyRoute struct {
	ID           string `json:"id"`
	ExternalID   string `json:"externalId"`
	Hostname     string `json:"hostname"`
	State        string `json:"state"`
	LastError    string `json:"lastError"`
	ResourceID   string `json:"resourceId"`
	ResourceName string `json:"resourceName"`
	EndpointName string `json:"endpointName"`
	Origin       string `json:"origin"`
	OriginAddress  string `json:"originAddress"`
	OriginPort     int32  `json:"originPort"`
	OriginProtocol string `json:"originProtocol"`
	OriginTLSMode  string `json:"originTlsMode"`
	HealthPath     string `json:"healthPath"`
	AppliedAt    string `json:"appliedAt"`
	ObservedAt   string `json:"observedAt"`
}

type ResourceCaddyRouteUpdateInput struct {
	Hostname       string `json:"hostname"`
	OriginAddress  string `json:"originAddress"`
	OriginPort     int32  `json:"originPort"`
	OriginProtocol string `json:"originProtocol"`
	OriginTLSMode  string `json:"originTlsMode"`
	HealthPath     string `json:"healthPath"`
}

type managedResourceEndpoint struct {
	Endpoint models.ResourceEndpointEntity
	Resource models.ResourceEntity
}

func resourceCaddyRouteID(endpointID uuid.UUID) string {
	return "deploycrate_resource_" + strings.ReplaceAll(endpointID.String(), "-", "_")
}

func resourceCaddyOriginProtocol(protocol, tlsMode string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "https" && strings.EqualFold(strings.TrimSpace(tlsMode), "disable") {
		return "http"
	}
	return protocol
}

func resourceCaddySupportsOrigin(protocol string) bool {
	return protocol == "http" || protocol == "https"
}

func (service CaddyRouteService) ValidateResourcePublication(ctx context.Context, resourceID, endpointID uuid.UUID, originProtocol string, input ResourceCaddyPublicationInput) error {
	if !input.Enabled {
		return nil
	}
	resource, err := models.Resource.Find(ctx, service.db.Executor(), resourceID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && resource.ArchivedAt.Valid) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	originProtocol = strings.ToLower(strings.TrimSpace(originProtocol))
	if !resourceCaddySupportsOrigin(originProtocol) {
		return domainError("protocol", "unsupported", "Caddy publication requires an HTTP or HTTPS origin")
	}
	hostname := models.NormalizeHostname(input.Hostname)
	if !models.IsValidHostname(hostname) {
		return domainError("hostname", "format", "hostname must be a valid fully qualified domain name")
	}
	conflicts, err := service.db.Executor().NewSelect().TableExpr("environment_domains").
		Where("lower(hostname) = ?", hostname).Where("archived_at IS NULL").Count(ctx)
	if err != nil {
		return err
	}
	if conflicts == 0 {
		query := service.db.Executor().NewSelect().TableExpr("resource_endpoints").
			Where("archived_at IS NULL").
			Where("settings ->> 'address_source' = ?", models.ResourceEndpointAddressCaddy).
			Where("lower(address) = ?", hostname)
		if endpointID != uuid.Nil {
			query = query.Where("id <> ?", endpointID)
		}
		conflicts, err = query.Count(ctx)
	}
	if err != nil {
		return err
	}
	if conflicts > 0 {
		return domainError("hostname", "unique", "an active Caddy route already uses this hostname")
	}
	return nil
}

func (service CaddyRouteService) PrepareResourcePublication(ctx context.Context, resourceID, endpointID uuid.UUID, origin ResourceEndpointInput, publication ResourceCaddyPublicationInput) (ResourceEndpointInput, error) {
	if !publication.Enabled {
		return origin, nil
	}
	if err := service.ValidateResourcePublication(ctx, resourceID, endpointID, origin.Protocol, publication); err != nil {
		return ResourceEndpointInput{}, err
	}
	settings := map[string]any{}
	if len(origin.Settings) > 0 {
		if err := json.Unmarshal(origin.Settings, &settings); err != nil {
			return ResourceEndpointInput{}, domainError("settings", "invalid", "endpoint settings must be a JSON object")
		}
	}
	if settings == nil {
		settings = map[string]any{}
	}
	settings["audience"] = models.ResourceEndpointAudiencePublic
	settings["address_source"] = models.ResourceEndpointAddressCaddy
	originProtocol := resourceCaddyOriginProtocol(origin.Protocol, origin.TLSMode)
	settings["caddy"] = models.ResourceEndpointCaddySettings{
		Managed: true, HealthPath: publication.HealthPath,
		OriginAddress: origin.Address, OriginPort: origin.Port,
		OriginProtocol: originProtocol, OriginTLSMode: origin.TLSMode,
		OriginPrivateNetworkID: origin.PrivateNetworkID,
	}
	resource, err := models.Resource.Find(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return ResourceEndpointInput{}, err
	}
	if resource.Engine() == "opentelemetry" {
		settings["exposure"] = models.ResourceEndpointExposurePublic
		settings["transport"] = models.ResourceEndpointTransportOTLPHTTP
		settings["authentication"] = models.ResourceEndpointAuthSignedIdentity
	}
	rawSettings, err := json.Marshal(settings)
	if err != nil {
		return ResourceEndpointInput{}, err
	}
	prepared := origin
	prepared.Address = models.NormalizeHostname(publication.Hostname)
	prepared.Port = 443
	prepared.Protocol = "https"
	prepared.TLSMode = "require"
	prepared.Settings = rawSettings
	prepared.PrivateNetworkID = nil
	return prepared, nil
}

func (service CaddyRouteService) ResourceRouteSnapshot(ctx context.Context) ([]ManagedResourceCaddyRoute, error) {
	items, err := service.managedResourceEndpoints(ctx, uuid.Nil)
	if err != nil {
		return nil, err
	}
	result := make([]ManagedResourceCaddyRoute, 0, len(items))
	for _, item := range items {
		settings := item.Endpoint.ParsedSettings().Caddy
		if settings == nil {
			continue
		}
		state, lastError, observedAt := service.observeResourceRoute(ctx, item.Endpoint.ID)
		result = append(result, ManagedResourceCaddyRoute{
			ID: item.Endpoint.ID.String(), ExternalID: resourceCaddyRouteID(item.Endpoint.ID),
			Hostname: item.Endpoint.Address, State: state, LastError: lastError,
			ResourceID: item.Resource.ID.String(), ResourceName: item.Resource.Name,
			EndpointName: item.Endpoint.Name,
			Origin:       fmt.Sprintf("%s://%s", resourceCaddyOriginProtocol(settings.OriginProtocol, settings.OriginTLSMode), net.JoinHostPort(settings.OriginAddress, fmt.Sprint(settings.OriginPort))),
			OriginAddress: settings.OriginAddress, OriginPort: settings.OriginPort,
			OriginProtocol: settings.OriginProtocol, OriginTLSMode: settings.OriginTLSMode,
			HealthPath: settings.HealthPath,
			ObservedAt:   observedAt,
		})
	}
	return result, nil
}

func (service CaddyRouteService) UpdateResourceRoute(ctx context.Context, endpointID uuid.UUID, input ResourceCaddyRouteUpdateInput) (string, error) {
	endpoint, err := models.ResourceEndpoint.Find(ctx, service.db.Executor(), endpointID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && endpoint.ArchivedAt.Valid) {
		return "", models.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	currentSettings := endpoint.ParsedSettings().Caddy
	if currentSettings == nil || !currentSettings.Managed {
		return "", models.ErrNotFound
	}
	input.Hostname = models.NormalizeHostname(input.Hostname)
	input.OriginAddress = strings.TrimSpace(input.OriginAddress)
	input.OriginProtocol = resourceCaddyOriginProtocol(input.OriginProtocol, input.OriginTLSMode)
	input.OriginTLSMode = strings.ToLower(strings.TrimSpace(input.OriginTLSMode))
	input.HealthPath = strings.TrimSpace(input.HealthPath)
	if err := service.ValidateResourcePublication(ctx, endpoint.ResourceID, endpoint.ID, input.OriginProtocol, ResourceCaddyPublicationInput{Enabled: true, Hostname: input.Hostname, HealthPath: input.HealthPath}); err != nil {
		return "", err
	}
	settings := map[string]any{}
	if err := json.Unmarshal(endpoint.Settings, &settings); err != nil {
		return "", domainError("settings", "invalid", "endpoint settings must be a JSON object")
	}
	settings["caddy"] = models.ResourceEndpointCaddySettings{
		Managed: true, HealthPath: input.HealthPath,
		OriginAddress: input.OriginAddress, OriginPort: input.OriginPort,
		OriginProtocol: input.OriginProtocol, OriginTLSMode: input.OriginTLSMode,
		OriginPrivateNetworkID: currentSettings.OriginPrivateNetworkID,
	}
	rawSettings, err := json.Marshal(settings)
	if err != nil {
		return "", err
	}
	resource, err := models.Resource.Find(ctx, service.db.Executor(), endpoint.ResourceID)
	if err != nil {
		return "", err
	}
	updated := endpoint
	updated.Address = input.Hostname
	updated.Port = 443
	updated.Protocol = "https"
	updated.TlsMode = "require"
	updated.Settings = rawSettings
	if err := updated.ValidateForKind(resource.Engine()); err != nil {
		return "", errors.Join(models.ErrDomainValidation, err)
	}
	if _, err := models.ResourceEndpoint.Update(ctx, service.db.Executor(), models.UpdateResourceEndpointData{
		ID: updated.ID, Name: updated.Name, Role: updated.Role, Address: updated.Address,
		Port: updated.Port, Protocol: updated.Protocol, TlsMode: updated.TlsMode,
		Settings: updated.Settings, ArchivedAt: updated.ArchivedAt, ResourceID: updated.ResourceID,
		PrivateNetworkID: updated.PrivateNetworkID,
	}); err != nil {
		return "", err
	}
	if err := service.SyncResourcePublication(ctx, endpoint.ResourceID, endpoint.ID, ResourceCaddyPublicationInput{Enabled: true, Hostname: input.Hostname, HealthPath: input.HealthPath}); err != nil {
		return "", err
	}
	return resourceCaddyRouteID(endpoint.ID), nil
}

func (service CaddyRouteService) ResourcePublications(ctx context.Context, resourceID uuid.UUID) ([]ResourceCaddyPublication, error) {
	items, err := service.managedResourceEndpoints(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	result := make([]ResourceCaddyPublication, 0, len(items))
	for _, item := range items {
		settings := item.Endpoint.ParsedSettings().Caddy
		if settings == nil {
			continue
		}
		state, lastError, observedAt := service.observeResourceRoute(ctx, item.Endpoint.ID)
		result = append(result, ResourceCaddyPublication{
			ID: item.Endpoint.ID.String(), ResourceEndpointID: item.Endpoint.ID.String(),
			ExternalID: resourceCaddyRouteID(item.Endpoint.ID), Hostname: item.Endpoint.Address,
			HealthPath: settings.HealthPath, State: state, LastError: lastError, ObservedAt: observedAt,
		})
	}
	return result, nil
}

func (service CaddyRouteService) SyncResourcePublication(ctx context.Context, resourceID, endpointID uuid.UUID, input ResourceCaddyPublicationInput) error {
	if !input.Enabled {
		return service.RemoveResourcePublication(ctx, resourceID, endpointID)
	}
	endpoint, err := models.ResourceEndpoint.Find(ctx, service.db.Executor(), endpointID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (endpoint.ResourceID != resourceID || endpoint.ArchivedAt.Valid)) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := service.ReconcileResourceRoute(ctx, endpointID); err != nil {
		slog.WarnContext(ctx, "Resource Caddy endpoint is pending reconciliation", "endpoint_id", endpointID, "error", err)
	}
	return nil
}

func (service CaddyRouteService) RemoveResourcePublication(ctx context.Context, resourceID, endpointID uuid.UUID) error {
	endpoint, err := models.ResourceEndpoint.Find(ctx, service.db.Executor(), endpointID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if endpoint.ResourceID != resourceID {
		return models.ErrNotFound
	}
	return service.caddy.DeleteRoute(ctx, resourceCaddyRouteID(endpointID))
}

func (service CaddyRouteService) ReconcileResourceRoute(ctx context.Context, endpointID uuid.UUID) error {
	endpoint, err := models.ResourceEndpoint.Find(ctx, service.db.Executor(), endpointID)
	if err != nil {
		return err
	}
	settings := endpoint.ParsedSettings().Caddy
	if endpoint.ArchivedAt.Valid || settings == nil || !settings.Managed {
		return nil
	}
	originProtocol := resourceCaddyOriginProtocol(settings.OriginProtocol, settings.OriginTLSMode)
	if !resourceCaddySupportsOrigin(originProtocol) {
		return fmt.Errorf("Caddy publication requires an HTTP or HTTPS origin, got %s", originProtocol)
	}
	return service.caddy.ApplyRoute(ctx, caddyclients.Route{
		ID: resourceCaddyRouteID(endpoint.ID), Domain: endpoint.Address,
		Backends: []caddyclients.Backend{{
			Dial: net.JoinHostPort(settings.OriginAddress, fmt.Sprint(settings.OriginPort)), Weight: 100,
		}},
		HealthPath: settings.HealthPath, DisableActiveHealthChecks: settings.HealthPath == "",
		UpstreamTLS: originProtocol == "https",
	})
}

func (service CaddyRouteService) ReconcileManagedResourceEndpoints(ctx context.Context) error {
	items, err := service.managedResourceEndpoints(ctx, uuid.Nil)
	if err != nil {
		return err
	}
	var result error
	for _, item := range items {
		result = errors.Join(result, service.ReconcileResourceRoute(ctx, item.Endpoint.ID))
	}
	return result
}

func (service CaddyRouteService) managedResourceEndpoints(ctx context.Context, resourceID uuid.UUID) ([]managedResourceEndpoint, error) {
	endpoints := make([]models.ResourceEndpointEntity, 0)
	query := service.db.Executor().NewSelect().Model(&endpoints).
		Where("archived_at IS NULL").
		Where("settings -> 'caddy' ->> 'managed' = 'true'")
	if resourceID != uuid.Nil {
		query = query.Where("resource_id = ?", resourceID)
	}
	if err := query.OrderExpr("created_at").Scan(ctx); err != nil {
		return nil, err
	}
	result := make([]managedResourceEndpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		resource, err := models.Resource.Find(ctx, service.db.Executor(), endpoint.ResourceID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && resource.ArchivedAt.Valid) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, managedResourceEndpoint{
			Endpoint: endpoint,
			Resource: resource,
		})
	}
	return result, nil
}

func (service CaddyRouteService) observeResourceRoute(ctx context.Context, endpointID uuid.UUID) (string, string, string) {
	endpoint, loadErr := models.ResourceEndpoint.Find(ctx, service.db.Executor(), endpointID)
	if loadErr == nil {
		settings := endpoint.ParsedSettings().Caddy
		if settings != nil {
			originProtocol := resourceCaddyOriginProtocol(settings.OriginProtocol, settings.OriginTLSMode)
			if !resourceCaddySupportsOrigin(originProtocol) {
				return models.CaddyRouteFailed.String(), "Caddy publication requires an HTTP or HTTPS origin", time.Now().UTC().Format(time.RFC3339)
			}
		}
	}
	if loadErr != nil {
		return models.CaddyRouteFailed.String(), loadErr.Error(), time.Now().UTC().Format(time.RFC3339)
	}
	err := service.caddy.VerifyRoute(ctx, resourceCaddyRouteID(endpointID))
	observedAt := time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		return models.CaddyRouteFailed.String(), err.Error(), observedAt
	}
	return models.CaddyRouteApplied.String(), "", observedAt
}
