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

func (service *ResourceManagement) CreateVolume(
	ctx context.Context,
	resourceID uuid.UUID,
	input ResourceVolumeInput,
) (models.ResourceVolumeEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	volume, err := service.createVolume(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceVolumeEntity{}, mapResourceConflict(err)
	}
	return volume, nil
}

func (service *ResourceManagement) createVolume(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceVolumeInput,
) (models.ResourceVolumeEntity, error) {
	volumes, err := models.ResourceVolume.ActiveCountForResource(ctx, db, resource.ID)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	if volumes > 0 {
		return models.ResourceVolumeEntity{}, domainError(
			"volume",
			"topology",
			"only one active volume is supported for a Resource right now",
		)
	}
	if err := service.validatePlacement(
		ctx,
		db,
		input.ServerID,
		managedResourceCapability(resource.Engine()),
	); err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	created, err := models.ResourceVolume.Create(ctx, db, models.CreateResourceVolumeData{
		Name: input.Name, Driver: input.Driver, Configuration: normalizedJSON(input.Configuration),
		ResourceID: resource.ID, ServerID: input.ServerID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) UpdateVolume(
	ctx context.Context,
	resourceID, volumeID uuid.UUID,
	input ResourceVolumeInput,
) (models.ResourceVolumeEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	current, err := models.ResourceVolume.LockActiveForResource(ctx, tx, resourceID, volumeID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceVolumeEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	if err := service.validatePlacement(
		ctx,
		tx,
		input.ServerID,
		managedResourceCapability(resource.Engine()),
	); err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	if current.ServerID != input.ServerID {
		mounts, countErr := models.ResourceVolumeMount.ActiveCountForVolume(ctx, tx, volumeID)
		if countErr != nil {
			return models.ResourceVolumeEntity{}, countErr
		}
		if mounts > 0 {
			return models.ResourceVolumeEntity{}, domainError(
				"serverId",
				"topology",
				"archive volume mounts before changing Servers",
			)
		}
	}
	updated, err := models.ResourceVolume.Update(ctx, tx, models.UpdateResourceVolumeData{
		ID:            current.ID,
		Name:          input.Name,
		Driver:        input.Driver,
		Configuration: normalizedJSON(input.Configuration),
		ArchivedAt:    current.ArchivedAt,
		ResourceID:    resourceID,
		ServerID:      input.ServerID,
	})
	if err != nil {
		return models.ResourceVolumeEntity{}, mapResourceConflict(err)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceVolumeEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) ArchiveVolume(
	ctx context.Context,
	resourceID, volumeID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	_, err = models.ResourceVolume.LockActiveForResource(ctx, tx, resourceID, volumeID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	mounts, err := models.ResourceVolumeMount.ActiveCountForVolume(ctx, tx, volumeID)
	if err != nil {
		return err
	}
	if mounts > 0 {
		return domainError("volume", "dependency", "volume has active mounts")
	}
	now := time.Now().UTC()
	if err := models.ResourceVolume.ArchiveID(ctx, tx, volumeID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) CreateMount(
	ctx context.Context,
	resourceID uuid.UUID,
	input ResourceMountInput,
) (models.ResourceVolumeMountEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	mount, err := service.createMount(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceVolumeMountEntity{}, mapResourceConflict(err)
	}
	return mount, nil
}

func (service *ResourceManagement) createMount(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceMountInput,
) (models.ResourceVolumeMountEntity, error) {
	mounts, err := models.ResourceVolumeMount.ActiveCountForResource(ctx, db, resource.ID)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	if mounts > 0 {
		return models.ResourceVolumeMountEntity{}, domainError(
			"mount",
			"topology",
			"only one active volume mount is supported for a Resource right now",
		)
	}
	topology, err := models.ResourceVolumeMount.Topology(
		ctx, db, input.ResourceVolumeID, input.ResourceInstallationID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceVolumeMountEntity{}, domainError(
			"mount",
			"topology",
			"volume and installation must be active",
		)
	}
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	if topology.VolumeResourceID != resource.ID || topology.InstallationResourceID != resource.ID ||
		topology.VolumeServerID != topology.InstallationServerID {
		return models.ResourceVolumeMountEntity{}, domainError(
			"mount",
			"topology",
			"volume and installation must belong to the same Resource and Server",
		)
	}
	created, err := models.ResourceVolumeMount.Create(ctx, db, models.CreateResourceVolumeMountData{
		MountPath:              input.MountPath,
		ReadOnly:               input.ReadOnly,
		ResourceVolumeID:       input.ResourceVolumeID,
		ResourceInstallationID: input.ResourceInstallationID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) UpdateMount(
	ctx context.Context,
	resourceID, mountID uuid.UUID,
	input ResourceMountInput,
) (models.ResourceVolumeMountEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	current, err := models.ResourceVolumeMount.LockActive(ctx, tx, mountID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceVolumeMountEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	owned, err := models.ResourceVolume.OwnedByResource(
		ctx,
		tx,
		current.ResourceVolumeID,
		resourceID,
	)
	if err != nil || !owned {
		if err != nil {
			return models.ResourceVolumeMountEntity{}, err
		}
		return models.ResourceVolumeMountEntity{}, models.ErrNotFound
	}
	if _, err := service.createMountValidation(ctx, tx, resource, input); err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	updated, err := models.ResourceVolumeMount.Update(ctx, tx, models.UpdateResourceVolumeMountData{
		ID:                     current.ID,
		MountPath:              input.MountPath,
		ReadOnly:               input.ReadOnly,
		ArchivedAt:             current.ArchivedAt,
		ResourceVolumeID:       input.ResourceVolumeID,
		ResourceInstallationID: input.ResourceInstallationID,
	})
	if err != nil {
		return models.ResourceVolumeMountEntity{}, mapResourceConflict(err)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceVolumeMountEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) createMountValidation(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceMountInput,
) (models.ResourceVolumeMountEntity, error) {
	entity := models.ResourceVolumeMountEntity{
		MountPath:              input.MountPath,
		ReadOnly:               input.ReadOnly,
		ResourceVolumeID:       input.ResourceVolumeID,
		ResourceInstallationID: input.ResourceInstallationID,
	}
	if err := entity.Validate(); err != nil {
		return models.ResourceVolumeMountEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	topology, err := models.ResourceVolumeMount.Topology(
		ctx, db, input.ResourceVolumeID, input.ResourceInstallationID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceVolumeMountEntity{}, domainError(
			"mount",
			"topology",
			"volume and installation must be active",
		)
	}
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	if topology.VolumeResourceID != resource.ID || topology.InstallationResourceID != resource.ID ||
		topology.VolumeServerID != topology.InstallationServerID {
		return models.ResourceVolumeMountEntity{}, domainError(
			"mount",
			"topology",
			"volume and installation must belong to the same Resource and Server",
		)
	}
	return entity, nil
}

func (service *ResourceManagement) ArchiveMount(
	ctx context.Context,
	resourceID, mountID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	_, err = models.ResourceVolumeMount.LockActiveForResource(ctx, tx, resourceID, mountID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := models.ResourceVolumeMount.ArchiveID(ctx, tx, mountID, now); err != nil {
		return err
	}
	return tx.Commit()
}
