package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"deploycrate-ce/clients/objectstorage"
	"deploycrate-ce/models"
)

type DatabaseRestoreEngine struct {
	artifact  *DatabaseArtifact
	database  *DatabaseBackup
	resources *ResourceManagement
}

func NewDatabaseRestoreEngine(
	artifact *DatabaseArtifact,
	database *DatabaseBackup,
	resources *ResourceManagement,
) *DatabaseRestoreEngine {
	return &DatabaseRestoreEngine{artifact: artifact, database: database, resources: resources}
}

type DatabaseRestoreResult struct {
	CutoverAt  *time.Time
	RolledBack bool
}

func (service *DatabaseRestoreEngine) Run(
	ctx context.Context,
	scope BackupScope,
	target PostgreSQLBackupTarget,
	credential BackupCredentialPayload,
	store objectstorage.Store,
	restoreID string,
) (DatabaseRestoreResult, error) {
	stagingName := "deploycrate_restore_" + strings.ReplaceAll(restoreID, "-", "")[:16]
	rollbackName := "deploycrate_rollback_" + strings.ReplaceAll(restoreID, "-", "")[:16]

	resumed, result, err := service.resumeCutover(ctx, target, stagingName, rollbackName)
	if resumed || err != nil {
		return result, err
	}

	loaded, err := service.artifact.Load(ctx, scope, target, credential, store)
	if err != nil {
		return DatabaseRestoreResult{}, err
	}
	defer loaded.Close()
	if err := service.database.validateDump(ctx, target, loaded.DumpPath); err != nil {
		return DatabaseRestoreResult{}, fmt.Errorf("validate restore dump: %w", err)
	}
	if err := service.dropDatabase(ctx, target, stagingName); err != nil {
		return DatabaseRestoreResult{}, fmt.Errorf("remove stale staging database: %w", err)
	}
	if err := service.sql(
		ctx,
		target,
		"postgres",
		models.PostgreSQLRestoreCreateDatabaseStatement(stagingName, target.Username),
	); err != nil {
		return DatabaseRestoreResult{}, fmt.Errorf("create staging database: %w", err)
	}
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = service.dropDatabase(context.WithoutCancel(ctx), target, stagingName)
		}
	}()
	if err := service.restoreDump(ctx, target, stagingName, loaded.DumpPath); err != nil {
		return DatabaseRestoreResult{}, fmt.Errorf("restore staging database: %w", err)
	}
	if err := service.resources.reconcilePostgreSQLDatabaseCredentials(
		ctx,
		target.ResourceID,
		target.InstallationID,
		stagingName,
	); err != nil {
		return DatabaseRestoreResult{}, fmt.Errorf(
			"reconcile restored database credentials: %w",
			err,
		)
	}
	if err := service.verifyDatabase(ctx, target, stagingName); err != nil {
		return DatabaseRestoreResult{}, fmt.Errorf("verify staging database: %w", err)
	}

	cutoverAt := time.Now().UTC()
	if err := service.setConnections(ctx, target, target.DatabaseName, false); err != nil {
		return DatabaseRestoreResult{}, fmt.Errorf("block target database connections: %w", err)
	}
	if err := service.terminateConnections(ctx, target, target.DatabaseName); err != nil {
		_ = service.setConnections(context.WithoutCancel(ctx), target, target.DatabaseName, true)
		return DatabaseRestoreResult{}, fmt.Errorf("terminate target database connections: %w", err)
	}
	if err := service.renameDatabase(ctx, target, target.DatabaseName, rollbackName); err != nil {
		_ = service.setConnections(context.WithoutCancel(ctx), target, target.DatabaseName, true)
		return DatabaseRestoreResult{}, fmt.Errorf("preserve target database for rollback: %w", err)
	}
	if err := service.renameDatabase(ctx, target, stagingName, target.DatabaseName); err != nil {
		rollbackErr := service.renameDatabase(
			context.WithoutCancel(ctx),
			target,
			rollbackName,
			target.DatabaseName,
		)
		_ = service.setConnections(context.WithoutCancel(ctx), target, target.DatabaseName, true)
		return DatabaseRestoreResult{
			CutoverAt:  &cutoverAt,
			RolledBack: rollbackErr == nil,
		}, errors.Join(
			fmt.Errorf("activate restored database: %w", err),
			rollbackErr,
		)
	}
	cleanupStaging = false
	if err := service.setConnections(ctx, target, target.DatabaseName, true); err != nil {
		return service.rollback(
			ctx,
			target,
			stagingName,
			rollbackName,
			cutoverAt,
			fmt.Errorf("allow restored database connections: %w", err),
		)
	}
	if err := service.verifyDatabase(ctx, target, target.DatabaseName); err != nil {
		return service.rollback(
			ctx,
			target,
			stagingName,
			rollbackName,
			cutoverAt,
			fmt.Errorf("verify restored database after cutover: %w", err),
		)
	}
	if err := service.dropDatabase(ctx, target, rollbackName); err != nil {
		slog.WarnContext(
			ctx,
			"restored database verified but rollback database cleanup failed",
			"rollback_database",
			rollbackName,
			"error",
			err,
		)
	}
	return DatabaseRestoreResult{CutoverAt: &cutoverAt}, nil
}

func (service *DatabaseRestoreEngine) resumeCutover(
	ctx context.Context,
	target PostgreSQLBackupTarget,
	stagingName, rollbackName string,
) (bool, DatabaseRestoreResult, error) {
	rollbackExists, err := service.databaseExists(ctx, target, rollbackName)
	if err != nil || !rollbackExists {
		return false, DatabaseRestoreResult{}, err
	}
	cutoverAt := time.Now().UTC()
	targetExists, err := service.databaseExists(ctx, target, target.DatabaseName)
	if err != nil {
		return true, DatabaseRestoreResult{}, err
	}
	if !targetExists {
		rollbackErr := service.renameDatabase(ctx, target, rollbackName, target.DatabaseName)
		_ = service.setConnections(context.WithoutCancel(ctx), target, target.DatabaseName, true)
		_ = service.dropDatabase(context.WithoutCancel(ctx), target, stagingName)
		return true, DatabaseRestoreResult{
			CutoverAt:  &cutoverAt,
			RolledBack: rollbackErr == nil,
		}, errors.Join(
			errors.New(
				"restore cutover was interrupted and the original database was recovered",
			),
			rollbackErr,
		)
	}
	if err := service.verifyDatabase(ctx, target, target.DatabaseName); err != nil {
		result, rollbackErr := service.rollback(
			ctx,
			target,
			stagingName,
			rollbackName,
			cutoverAt,
			err,
		)
		return true, result, rollbackErr
	}
	if err := service.dropDatabase(ctx, target, rollbackName); err != nil {
		slog.WarnContext(
			ctx,
			"resumed database restore verified but rollback database cleanup failed",
			"rollback_database",
			rollbackName,
			"error",
			err,
		)
	}
	return true, DatabaseRestoreResult{CutoverAt: &cutoverAt}, nil
}

func (service *DatabaseRestoreEngine) rollback(
	ctx context.Context,
	target PostgreSQLBackupTarget,
	stagingName, rollbackName string,
	cutoverAt time.Time,
	operationErr error,
) (DatabaseRestoreResult, error) {
	rollbackContext := context.WithoutCancel(ctx)
	if exists, _ := service.databaseExists(rollbackContext, target, target.DatabaseName); exists {
		_ = service.setConnections(rollbackContext, target, target.DatabaseName, false)
		_ = service.terminateConnections(rollbackContext, target, target.DatabaseName)
		_ = service.dropDatabase(rollbackContext, target, stagingName)
		if err := service.renameDatabase(
			rollbackContext,
			target,
			target.DatabaseName,
			stagingName,
		); err != nil {
			return DatabaseRestoreResult{
				CutoverAt: &cutoverAt,
			}, errors.Join(
				operationErr,
				fmt.Errorf("preserve failed restored database: %w", err),
			)
		}
	}
	if err := service.renameDatabase(
		rollbackContext,
		target,
		rollbackName,
		target.DatabaseName,
	); err != nil {
		return DatabaseRestoreResult{
			CutoverAt: &cutoverAt,
		}, errors.Join(
			operationErr,
			fmt.Errorf("restore original database: %w", err),
		)
	}
	if err := service.setConnections(
		rollbackContext,
		target,
		target.DatabaseName,
		true,
	); err != nil {
		return DatabaseRestoreResult{
			CutoverAt: &cutoverAt,
		}, errors.Join(
			operationErr,
			fmt.Errorf("reopen original database: %w", err),
		)
	}
	_ = service.dropDatabase(rollbackContext, target, stagingName)
	return DatabaseRestoreResult{CutoverAt: &cutoverAt, RolledBack: true}, operationErr
}

func (service *DatabaseRestoreEngine) restoreDump(
	ctx context.Context,
	target PostgreSQLBackupTarget,
	databaseName, dumpPath string,
) error {
	dump, err := os.Open(dumpPath)
	if err != nil {
		return err
	}
	defer dump.Close()
	return service.database.runContainerPostgres(ctx, target, dump, nil, "pg_restore",
		"--username", target.Username, "--dbname", databaseName, "--exit-on-error", "--no-password")
}

func (service *DatabaseRestoreEngine) verifyDatabase(
	ctx context.Context,
	target PostgreSQLBackupTarget,
	databaseName string,
) error {
	var output bytes.Buffer
	if err := service.database.runContainerPostgres(
		ctx,
		target,
		nil,
		&output,
		"psql",
		"--username",
		target.Username,
		"--dbname",
		databaseName,
		"--no-password",
		"--tuples-only",
		"--no-align",
		"--command",
		models.PostgreSQLRestoreVerifyStatement(),
	); err != nil {
		return err
	}
	if strings.TrimSpace(output.String()) != "1" {
		return errors.New("PostgreSQL verification query returned an unexpected result")
	}
	return nil
}

func (service *DatabaseRestoreEngine) databaseExists(
	ctx context.Context,
	target PostgreSQLBackupTarget,
	databaseName string,
) (bool, error) {
	var output bytes.Buffer
	query := models.PostgreSQLRestoreDatabaseExistsStatement(databaseName)
	if err := service.database.runContainerPostgres(
		ctx,
		target,
		nil,
		&output,
		"psql",
		"--username",
		target.Username,
		"--dbname",
		"postgres",
		"--no-password",
		"--tuples-only",
		"--no-align",
		"--command",
		query,
	); err != nil {
		return false, err
	}
	return strings.TrimSpace(output.String()) == "1", nil
}

func (service *DatabaseRestoreEngine) dropDatabase(
	ctx context.Context,
	target PostgreSQLBackupTarget,
	databaseName string,
) error {
	return service.sql(
		ctx,
		target,
		"postgres",
		models.PostgreSQLRestoreDropDatabaseStatement(databaseName),
	)
}

func (service *DatabaseRestoreEngine) renameDatabase(
	ctx context.Context,
	target PostgreSQLBackupTarget,
	from, to string,
) error {
	return service.sql(
		ctx,
		target,
		"postgres",
		models.PostgreSQLRestoreRenameDatabaseStatement(from, to),
	)
}

func (service *DatabaseRestoreEngine) setConnections(
	ctx context.Context,
	target PostgreSQLBackupTarget,
	databaseName string,
	allowed bool,
) error {
	return service.sql(
		ctx,
		target,
		"postgres",
		models.PostgreSQLRestoreAllowConnectionsStatement(databaseName, allowed),
	)
}

func (service *DatabaseRestoreEngine) terminateConnections(
	ctx context.Context,
	target PostgreSQLBackupTarget,
	databaseName string,
) error {
	return service.sql(
		ctx,
		target,
		"postgres",
		models.PostgreSQLRestoreTerminateConnectionsStatement(databaseName),
	)
}

func (service *DatabaseRestoreEngine) sql(
	ctx context.Context,
	target PostgreSQLBackupTarget,
	databaseName, statement string,
) error {
	return service.database.runContainerPostgres(
		ctx,
		target,
		nil,
		nil,
		"psql",
		"--username",
		target.Username,
		"--dbname",
		databaseName,
		"--no-password",
		"--set",
		"ON_ERROR_STOP=1",
		"--command",
		statement,
	)
}
