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

func (service *ResourceManagement) GrantEnvironment(ctx context.Context, resourceID, environmentID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockResourceForGrantChange(ctx, tx, resourceID); err != nil {
		return err
	}
	if _, err := models.ResourceEnvironmentGrant.Create(ctx, tx, resourceID, environmentID); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) RevokeEnvironmentGrant(ctx context.Context, resourceID, environmentID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockResourceForGrantChange(ctx, tx, resourceID); err != nil {
		return err
	}
	connections, err := tx.NewSelect().TableExpr("environment_resources").Where("resource_id = ?", resourceID).Where("environment_id = ?", environmentID).Where("archived_at IS NULL").Count(ctx)
	if err != nil {
		return err
	}
	if connections != 0 {
		return domainError("grant", "connection", "disconnect the Environment before revoking its Resource grant")
	}
	now := time.Now().UTC()
	_, err = tx.NewUpdate().TableExpr("resource_environment_grants").Set("archived_at = ?", now).Set("updated_at = ?", now).
		Where("resource_id = ?", resourceID).Where("environment_id = ?", environmentID).Where("archived_at IS NULL").Exec(ctx)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) GrantApplication(ctx context.Context, resourceID, applicationID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockResourceForGrantChange(ctx, tx, resourceID); err != nil {
		return err
	}
	if _, err := models.ResourceApplicationGrant.Create(ctx, tx, resourceID, applicationID); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) RevokeApplicationGrant(ctx context.Context, resourceID, applicationID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockResourceForGrantChange(ctx, tx, resourceID); err != nil {
		return err
	}
	connections, err := tx.NewSelect().TableExpr("environment_resources AS connection").
		Join("JOIN environments AS environment ON environment.id = connection.environment_id").
		Where("connection.resource_id = ?", resourceID).Where("environment.application_id = ?", applicationID).
		Where("connection.archived_at IS NULL").Count(ctx)
	if err != nil {
		return err
	}
	if connections != 0 {
		return domainError("grant", "connection", "disconnect the Application Environments before revoking its Resource grant")
	}
	now := time.Now().UTC()
	if _, err := tx.NewUpdate().TableExpr("resource_application_grants").Set("archived_at = ?", now).Set("updated_at = ?", now).
		Where("resource_id = ?", resourceID).Where("application_id = ?", applicationID).Where("archived_at IS NULL").Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func lockResourceForGrantChange(ctx context.Context, db storage.Executor, resourceID uuid.UUID) error {
	var lockedID uuid.UUID
	err := db.NewSelect().TableExpr("resources").ColumnExpr("id").Where("id = ?", resourceID).Where("archived_at IS NULL").For("UPDATE").Scan(ctx, &lockedID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	return err
}
