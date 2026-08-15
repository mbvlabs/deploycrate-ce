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

func (service *ResourceManagement) CreateHealthCheck(
	ctx context.Context,
	resourceID uuid.UUID,
	input ResourceHealthCheckInput,
) (models.ResourceHealthCheckEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	check, err := service.createHealthCheck(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceHealthCheckEntity{}, mapResourceConflict(err)
	}
	return check, nil
}

func (service *ResourceManagement) createHealthCheck(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceHealthCheckInput,
) (models.ResourceHealthCheckEntity, error) {
	entity := models.ResourceHealthCheckEntity{
		Name:                 input.Name,
		Kind:                 input.Kind,
		Configuration:        normalizedJSON(input.Configuration),
		IntervalSeconds:      input.IntervalSeconds,
		TimeoutSeconds:       input.TimeoutSeconds,
		FailureThreshold:     input.FailureThreshold,
		SuccessThreshold:     input.SuccessThreshold,
		Enabled:              input.Enabled,
		ResourceID:           resource.ID,
		ResourceEndpointID:   input.ResourceEndpointID,
		ResourceCredentialID: input.ResourceCredentialID,
	}
	if err := entity.ValidateForKind(resource.Engine()); err != nil {
		return models.ResourceHealthCheckEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	if err := service.validateHealthTopology(ctx, db, resource.ID, input); err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	created, err := models.ResourceHealthCheck.Create(ctx, db, models.CreateResourceHealthCheckData{
		Name:                 entity.Name,
		Kind:                 entity.Kind,
		Configuration:        entity.Configuration,
		IntervalSeconds:      entity.IntervalSeconds,
		TimeoutSeconds:       entity.TimeoutSeconds,
		FailureThreshold:     entity.FailureThreshold,
		SuccessThreshold:     entity.SuccessThreshold,
		Enabled:              entity.Enabled,
		ResourceID:           entity.ResourceID,
		ResourceEndpointID:   entity.ResourceEndpointID,
		ResourceCredentialID: entity.ResourceCredentialID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) validateHealthTopology(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
	input ResourceHealthCheckInput,
) error {
	resourceType, err := models.Resource.TypeForID(ctx, db, resourceID)
	if err != nil {
		return err
	}
	if input.ResourceEndpointID == nil {
		return domainError("resourceEndpointId", "required", "health checks require an endpoint")
	}
	if resourceType == models.ResourceTypeDatabase && input.ResourceCredentialID == nil {
		return domainError(
			"resourceCredentialId",
			"required",
			"database Resource access checks require a credential",
		)
	}
	if input.ResourceEndpointID != nil {
		belongs, countErr := models.ResourceEndpoint.ActiveBelongsToResource(
			ctx, db, *input.ResourceEndpointID, resourceID,
		)
		if countErr != nil {
			return countErr
		}
		if err := requireChild(
			boolCount(belongs),
			"resourceEndpointId",
			"endpoint does not belong to this installation topology",
		); err != nil {
			return err
		}
	}
	if input.ResourceCredentialID != nil {
		belongs, countErr := models.ResourceCredential.ActiveBelongsToResource(
			ctx, db, *input.ResourceCredentialID, resourceID,
		)
		if countErr != nil {
			return countErr
		}
		if err := requireChild(
			boolCount(belongs),
			"resourceCredentialId",
			"credential does not belong to this installation topology",
		); err != nil {
			return err
		}
	}
	return nil
}

func (service *ResourceManagement) UpdateHealthCheck(
	ctx context.Context,
	resourceID, healthCheckID uuid.UUID,
	input ResourceHealthCheckInput,
) (models.ResourceHealthCheckEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	current, err := models.ResourceHealthCheck.LockActiveForResource(
		ctx, tx, resourceID, healthCheckID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceHealthCheckEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	entity := models.ResourceHealthCheckEntity{
		ID:                   current.ID,
		Name:                 input.Name,
		Kind:                 input.Kind,
		Configuration:        normalizedJSON(input.Configuration),
		IntervalSeconds:      input.IntervalSeconds,
		TimeoutSeconds:       input.TimeoutSeconds,
		FailureThreshold:     input.FailureThreshold,
		SuccessThreshold:     input.SuccessThreshold,
		Enabled:              input.Enabled,
		ArchivedAt:           current.ArchivedAt,
		ResourceID:           resource.ID,
		ResourceEndpointID:   input.ResourceEndpointID,
		ResourceCredentialID: input.ResourceCredentialID,
	}
	if err := entity.ValidateForKind(resource.Engine()); err != nil {
		return models.ResourceHealthCheckEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	if err := service.validateHealthTopology(ctx, tx, resourceID, input); err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	updated, err := models.ResourceHealthCheck.Update(ctx, tx, models.UpdateResourceHealthCheckData{
		ID:                   current.ID,
		Name:                 entity.Name,
		Kind:                 entity.Kind,
		Configuration:        entity.Configuration,
		IntervalSeconds:      entity.IntervalSeconds,
		TimeoutSeconds:       entity.TimeoutSeconds,
		FailureThreshold:     entity.FailureThreshold,
		SuccessThreshold:     entity.SuccessThreshold,
		Enabled:              entity.Enabled,
		ArchivedAt:           current.ArchivedAt,
		ResourceID:           entity.ResourceID,
		ResourceEndpointID:   entity.ResourceEndpointID,
		ResourceCredentialID: entity.ResourceCredentialID,
	})
	if err != nil {
		return models.ResourceHealthCheckEntity{}, mapResourceConflict(err)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceHealthCheckEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) ArchiveHealthCheck(
	ctx context.Context,
	resourceID, healthCheckID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	_, err = models.ResourceHealthCheck.LockActiveForResource(ctx, tx, resourceID, healthCheckID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := models.ResourceHealthCheck.ArchiveID(ctx, tx, healthCheckID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func requireChild(count int, field, message string) error {
	if count != 1 {
		return domainError(field, "topology", message)
	}
	return nil
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
