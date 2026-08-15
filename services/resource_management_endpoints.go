package services

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

func (service *ResourceManagement) CreateEndpoint(
	ctx context.Context,
	resourceID uuid.UUID,
	input ResourceEndpointInput,
) (models.ResourceEndpointEntity, error) {
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

func (service *ResourceManagement) createEndpoint(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceEndpointInput,
) (models.ResourceEndpointEntity, error) {
	input.Settings = normalizedJSON(input.Settings)
	entity := models.ResourceEndpointEntity{
		Name: input.Name, Role: input.Role, Address: input.Address, Port: input.Port,
		Protocol: input.Protocol, TLSMode: input.TLSMode, Settings: input.Settings,
		ResourceID:       resource.ID,
		PrivateNetworkID: input.PrivateNetworkID,
	}
	if err := entity.ValidateForKind(resource.Engine()); err != nil {
		return models.ResourceEndpointEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	caddySettings := entity.ParsedSettings().Caddy
	isCaddyEndpoint := caddySettings != nil && caddySettings.Managed
	if !isCaddyEndpoint && input.Role == "primary" && input.PrivateNetworkID == nil {
		primaryEndpoints, err := models.ResourceEndpoint.ActivePrimaryPublicCount(
			ctx,
			db,
			resource.ID,
		)
		if err != nil {
			return models.ResourceEndpointEntity{}, err
		}
		if primaryEndpoints != 0 {
			return models.ResourceEndpointEntity{}, domainError(
				"role",
				"primary",
				"Resource already has a primary origin endpoint",
			)
		}
	}
	if err := service.validateEndpointTopology(ctx, db, resource, nil, input); err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	created, err := models.ResourceEndpoint.Create(ctx, db, models.CreateResourceEndpointData{
		Name: entity.Name, Role: entity.Role, Address: entity.Address, Port: entity.Port,
		Protocol: entity.Protocol, TlsMode: entity.TLSMode, Settings: entity.Settings,
		ResourceID:       resource.ID,
		PrivateNetworkID: entity.PrivateNetworkID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) UpdateEndpoint(
	ctx context.Context,
	resourceID, endpointID uuid.UUID,
	input ResourceEndpointInput,
) (models.ResourceEndpointEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	current, err := models.ResourceEndpoint.LockActiveForResource(ctx, tx, resourceID, endpointID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceEndpointEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if current.Role == "wireguard" && current.PrivateNetworkID != nil &&
		current.Name == "Private access" {
		return models.ResourceEndpointEntity{}, domainError(
			"endpoint",
			"managed",
			"WireGuard access is managed through the Endpoints page",
		)
	}
	currentCaddySettings := current.ParsedSettings().Caddy
	currentIsCaddyEndpoint := currentCaddySettings != nil && currentCaddySettings.Managed
	if !currentIsCaddyEndpoint && current.Role == "primary" && current.PrivateNetworkID == nil &&
		(input.Role != "primary" || input.PrivateNetworkID != nil) {
		return models.ResourceEndpointEntity{}, domainError(
			"role",
			"primary",
			"external Resource primary origin cannot be changed into another endpoint type",
		)
	}
	input.Settings = normalizedJSON(input.Settings)
	entity := models.ResourceEndpointEntity{
		ID:               current.ID,
		Name:             input.Name,
		Role:             input.Role,
		Address:          input.Address,
		Port:             input.Port,
		Protocol:         input.Protocol,
		TLSMode:          input.TLSMode,
		Settings:         input.Settings,
		ResourceID:       resource.ID,
		PrivateNetworkID: input.PrivateNetworkID,
		ArchivedAt:       current.ArchivedAt,
	}
	if err := entity.ValidateForKind(resource.Engine()); err != nil {
		return models.ResourceEndpointEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	inputCaddySettings := entity.ParsedSettings().Caddy
	inputIsCaddyEndpoint := inputCaddySettings != nil && inputCaddySettings.Managed
	if !inputIsCaddyEndpoint && input.Role == "primary" && input.PrivateNetworkID == nil &&
		(current.Role != "primary" || current.PrivateNetworkID != nil) {
		primaryEndpoints, countErr := models.ResourceEndpoint.ActivePrimaryPublicCount(
			ctx,
			tx,
			resourceID,
		)
		if countErr != nil {
			return models.ResourceEndpointEntity{}, countErr
		}
		if primaryEndpoints != 0 {
			return models.ResourceEndpointEntity{}, domainError(
				"role",
				"primary",
				"Resource already has a primary origin endpoint",
			)
		}
	}
	if err := service.validateEndpointTopology(ctx, tx, resource, &current.ID, input); err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	updated, err := models.ResourceEndpoint.Update(ctx, tx, models.UpdateResourceEndpointData{
		ID:               current.ID,
		Name:             entity.Name,
		Role:             entity.Role,
		Address:          entity.Address,
		Port:             entity.Port,
		Protocol:         entity.Protocol,
		TlsMode:          entity.TLSMode,
		Settings:         entity.Settings,
		ArchivedAt:       current.ArchivedAt,
		ResourceID:       resource.ID,
		PrivateNetworkID: entity.PrivateNetworkID,
	})
	if err != nil {
		return models.ResourceEndpointEntity{}, mapResourceConflict(err)
	}
	connections, err := models.EnvironmentResource.ActiveForEndpointID(ctx, tx, endpointID)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if err := service.reconcileEnvironmentResourceConnections(
		ctx,
		tx,
		resource,
		connections,
	); err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceEndpointEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) validateEndpointTopology(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	endpointID *uuid.UUID,
	input ResourceEndpointInput,
) error {
	if input.PrivateNetworkID != nil {
		exists, err := models.PrivateNetwork.ActiveExists(ctx, db, *input.PrivateNetworkID)
		if err != nil {
			return err
		}
		if err := requireChild(
			boolCount(exists),
			"privateNetworkId",
			"private network is unavailable",
		); err != nil {
			return err
		}
		enabled, err := models.ResourceEndpoint.WireGuardGatewayExists(
			ctx, db, resource.ID, *input.PrivateNetworkID,
		)
		if err != nil {
			return err
		}
		if err := requireChild(
			boolCount(enabled),
			"privateNetworkId",
			"turn on WireGuard access before publishing an endpoint through this private network",
		); err != nil {
			return err
		}
		if endpointID != nil {
			incompatible, err := models.EnvironmentResource.IncompatibleEndpointNetworkCount(
				ctx, db, *endpointID, *input.PrivateNetworkID,
			)
			if err != nil {
				return err
			}
			if incompatible > 0 {
				return domainError(
					"privateNetworkId",
					"topology",
					"an existing Connected Environment cannot reach this private network",
				)
			}
		}
	}
	return nil
}

func (service *ResourceManagement) ArchiveEndpoint(
	ctx context.Context,
	resourceID, endpointID uuid.UUID,
) error {
	return service.archiveEndpoint(ctx, resourceID, endpointID, false)
}

func (service *ResourceManagement) ArchiveSystemEndpoint(
	ctx context.Context,
	resourceID, endpointID uuid.UUID,
) error {
	return service.archiveEndpoint(ctx, resourceID, endpointID, true)
}

func (service *ResourceManagement) archiveEndpoint(
	ctx context.Context,
	resourceID, endpointID uuid.UUID,
	systemManaged bool,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	resource, err := service.loadResourceWithSystemPolicy(ctx, tx, resourceID, true, systemManaged)
	if err != nil {
		return err
	}
	if resource.SystemManaged != systemManaged {
		return models.ErrNotFound
	}
	endpoint, err := models.ResourceEndpoint.LockActiveForResource(ctx, tx, resourceID, endpointID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	if endpoint.Role == "wireguard" && endpoint.PrivateNetworkID != nil &&
		endpoint.Name == "Private access" {
		return domainError(
			"endpoint",
			"managed",
			"turn off WireGuard access instead of archiving its gateway endpoint",
		)
	}
	dependencies, err := models.ResourceEndpoint.ActiveDependencyCount(ctx, tx, endpointID)
	if err != nil {
		return err
	}
	if dependencies > 0 {
		return domainError(
			"endpoint",
			"dependency",
			"endpoint is selected by an active binding or health check",
		)
	}
	if !systemManaged && endpoint.Role == "primary" && endpoint.PrivateNetworkID == nil {
		primaryEndpoints, countErr := models.ResourceEndpoint.ActivePrimaryPublicCount(
			ctx,
			tx,
			resourceID,
		)
		if countErr != nil {
			return countErr
		}
		if primaryEndpoints == 1 {
			return domainError(
				"endpoint",
				"required",
				"external Resources must retain one primary origin endpoint",
			)
		}
	}
	now := time.Now().UTC()
	if _, err := models.ResourceEndpoint.ArchiveActive(ctx, tx, endpointID, now); err != nil {
		return err
	}
	return tx.Commit()
}
