package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	postgresqlclient "deploycrate-ce/clients/postgresql"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

func (service *ResourceManagement) CreateDatabase(
	ctx context.Context,
	resourceID uuid.UUID,
	input CreateResourceDatabaseInput,
) (result models.ResourceEntity, err error) {
	database := input.Database
	database.Name = strings.TrimSpace(database.Name)
	database.Encoding = strings.TrimSpace(database.Encoding)
	database.Collation = strings.TrimSpace(database.Collation)
	if (input.CredentialID == nil) == (input.Credential == nil) {
		return models.ResourceEntity{}, domainError(
			"credential",
			"required",
			"select an existing application credential or create a new one",
		)
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	if resource.ResourceType != models.ResourceTypeDatabase || resource.Engine() != "postgresql" {
		return models.ResourceEntity{}, domainError(
			"database",
			"resource_type",
			"database creation currently requires a PostgreSQL Resource",
		)
	}
	if database.Name == "" || strings.EqualFold(database.Name, "postgres") ||
		resourceHasDatabase(resource, database.Name) {
		return models.ResourceEntity{}, domainError(
			"name",
			"unavailable",
			"database name must be unique and cannot be postgres",
		)
	}
	connection, err := service.postgreSQLAdministratorConnection(ctx, tx, resource)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	createdDatabase, err := service.postgres.CreateDatabase(
		ctx,
		connection,
		database.Name,
		database.Encoding,
		database.Collation,
	)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	completed := false
	var reconciledCredential *models.ResourceCredentialEntity
	var previousCredential *models.ResourceCredentialEntity
	defer func() {
		if completed {
			return
		}
		compensationContext := context.WithoutCancel(ctx)
		compensationErrors := make([]error, 0, 3)
		if previousCredential != nil && reconciledCredential != nil {
			compensationErrors = append(
				compensationErrors,
				service.reconcilePostgreSQLCredential(
					compensationContext,
					service.db.Executor(),
					resource,
					*previousCredential,
					reconciledCredential,
				),
			)
		}
		if createdDatabase {
			compensationErrors = append(
				compensationErrors,
				service.postgres.DropDatabase(compensationContext, connection, database.Name),
			)
		}
		if previousCredential == nil && reconciledCredential != nil &&
			reconciledCredential.Username.Valid {
			if !createdDatabase {
				compensationErrors = append(
					compensationErrors,
					service.postgres.RevokeLoginRoleDatabase(
						compensationContext,
						connection,
						database.Name,
						reconciledCredential.Username.String,
					),
				)
			}
			compensationErrors = append(
				compensationErrors,
				service.postgres.DropLoginRole(
					compensationContext,
					connection,
					reconciledCredential.Username.String,
				),
			)
		}
		err = errors.Join(err, errors.Join(compensationErrors...))
	}()
	configuration := resource.ParsedConfiguration()
	configuration.Databases = append(configuration.Databases, database)
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	updated, err := models.Resource.Update(ctx, tx, models.UpdateResourceData{
		ID:                    resource.ID,
		Name:                  resource.Name,
		Slug:                  resource.Slug,
		ResourceType:          resource.ResourceType,
		Configuration:         encoded,
		SystemManaged:         resource.SystemManaged,
		EnvironmentAttachable: resource.EnvironmentAttachable,
		ArchivedAt:            resource.ArchivedAt,
	})
	if err != nil {
		return models.ResourceEntity{}, err
	}
	resource = updated
	if input.CredentialID != nil {
		current, findErr := models.ResourceCredential.LockActiveForResource(
			ctx, tx, resource.ID, *input.CredentialID,
		)
		if errors.Is(findErr, sql.ErrNoRows) {
			return models.ResourceEntity{}, domainError(
				"credentialId",
				"unavailable",
				"selected application credential is unavailable",
			)
		}
		if findErr != nil {
			return models.ResourceEntity{}, findErr
		}
		if resourceCredentialMetadataPurpose(current.Metadata) != "application" {
			return models.ResourceEntity{}, domainError(
				"credentialId",
				"application",
				"only application credentials can be attached to a Database",
			)
		}
		metadata, metadataErr := resourceCredentialMetadataForDatabase(
			current.Metadata,
			database.Name,
		)
		if metadataErr != nil {
			return models.ResourceEntity{}, metadataErr
		}
		candidate := current
		candidate.Metadata = metadata
		credentialInput := ResourceCredentialInput{
			Name: current.Name, Username: current.Username.String,
			Metadata: metadata,
		}
		if validationErr := service.validatePostgreSQLCredential(
			ctx,
			tx,
			resource,
			credentialInput,
			&current.ID,
		); validationErr != nil {
			return models.ResourceEntity{}, validationErr
		}
		if reconcileErr := service.reconcilePostgreSQLCredential(
			ctx,
			tx,
			resource,
			candidate,
			&current,
		); reconcileErr != nil {
			return models.ResourceEntity{}, reconcileErr
		}
		reconciledCredential = &candidate
		previousCredential = &current
		if _, updateErr := models.ResourceCredential.Update(
			ctx,
			tx,
			models.UpdateResourceCredentialData{
				ID: current.ID, Name: current.Name, Username: current.Username,
				Metadata: metadata, EncPayload: current.EncPayload, Digest: current.Digest,
				ArchivedAt: current.ArchivedAt, ResourceID: resource.ID,
			},
		); updateErr != nil {
			return models.ResourceEntity{}, mapResourceConflict(updateErr)
		}
	} else {
		credentialInput := *input.Credential
		metadata, metadataErr := resourceCredentialMetadataForDatabase(
			credentialInput.Metadata,
			database.Name,
		)
		if metadataErr != nil {
			return models.ResourceEntity{}, metadataErr
		}
		credentialInput.Metadata = metadata
		createdCredential, createCredentialErr := service.createCredential(
			ctx,
			tx,
			resource,
			credentialInput,
		)
		if createCredentialErr != nil {
			return models.ResourceEntity{}, prefixResourceValidation(
				createCredentialErr,
				"credential",
			)
		}
		if reconcileErr := service.reconcilePostgreSQLCredential(
			ctx,
			tx,
			resource,
			createdCredential,
			nil,
		); reconcileErr != nil {
			return models.ResourceEntity{}, prefixResourceValidation(
				reconcileErr,
				"credential",
			)
		}
		reconciledCredential = &createdCredential
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceEntity{}, err
	}
	completed = true
	return updated, nil
}

func (service *ResourceManagement) DestroyDatabase(
	ctx context.Context,
	resourceID uuid.UUID,
	databaseName string,
) error {
	databaseName = strings.TrimSpace(databaseName)
	if databaseName == "" || strings.EqualFold(databaseName, "postgres") {
		return domainError(
			"database",
			"unavailable",
			"select a configured application Database",
		)
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return err
	}
	if resource.ResourceType != models.ResourceTypeDatabase || resource.Engine() != "postgresql" {
		return domainError(
			"database",
			"resource_type",
			"database deletion currently requires a PostgreSQL Resource",
		)
	}
	databaseIndex := -1
	configuration := resource.ParsedConfiguration()
	for index, database := range configuration.Databases {
		if strings.EqualFold(database.Name, databaseName) {
			databaseIndex = index
			databaseName = database.Name
			break
		}
	}
	if databaseIndex < 0 {
		return models.ErrNotFound
	}
	if _, policyErr := models.BackupPolicy.FindForResourceDatabase(
		ctx,
		tx,
		resource.ID,
		databaseName,
	); policyErr == nil {
		return domainError(
			"database",
			"dependency",
			"archive the Database backup policy before deleting this Database",
		)
	} else if !errors.Is(policyErr, sql.ErrNoRows) {
		return policyErr
	}
	activeRestores, err := models.ResourceRestore.ActiveCountForResourceDatabase(
		ctx,
		tx,
		resource.ID,
		databaseName,
	)
	if err != nil {
		return err
	}
	if activeRestores > 0 {
		return domainError(
			"database",
			"dependency",
			"wait for the active Database restore to finish before deleting this Database",
		)
	}
	credentials, err := models.ResourceCredential.LockActiveApplicationsForDatabase(
		ctx,
		tx,
		resource.ID,
		databaseName,
	)
	if err != nil {
		return err
	}
	for _, credential := range credentials {
		dependencies, dependencyErr := models.ResourceCredential.ActiveDependencyCount(
			ctx,
			tx,
			credential.ID,
		)
		if dependencyErr != nil {
			return dependencyErr
		}
		if dependencies > 0 {
			return domainError(
				"database",
				"dependency",
				"detach Environments and health checks using this Database before deleting it",
			)
		}
	}
	connection, err := service.postgreSQLAdministratorConnection(ctx, tx, resource)
	if err != nil {
		return err
	}
	configuration.Databases = slices.Delete(configuration.Databases, databaseIndex, databaseIndex+1)
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return err
	}
	if _, err := models.Resource.Update(ctx, tx, models.UpdateResourceData{
		ID: resource.ID, Name: resource.Name, Slug: resource.Slug,
		ResourceType: resource.ResourceType, Configuration: encoded,
		SystemManaged: resource.SystemManaged, EnvironmentAttachable: resource.EnvironmentAttachable,
		ArchivedAt: resource.ArchivedAt,
	}); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, credential := range credentials {
		if err := models.ResourceCredential.ArchiveID(ctx, tx, credential.ID, now); err != nil {
			return err
		}
	}
	if err := service.postgres.DropDatabase(ctx, connection, databaseName); err != nil {
		return err
	}
	for _, credential := range credentials {
		if !credential.Username.Valid {
			continue
		}
		if cleanupErr := service.postgres.DropLoginRole(
			ctx,
			connection,
			credential.Username.String,
		); cleanupErr != nil {
			slog.WarnContext(
				ctx,
				"database deleted but PostgreSQL credential role cleanup failed",
				"resource_id", resource.ID,
				"database", databaseName,
				"credential_id", credential.ID,
				"username", credential.Username.String,
				"error", cleanupErr,
			)
		}
	}
	return tx.Commit()
}

func (service *ResourceManagement) reconcilePostgreSQLCredential(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	credential models.ResourceCredentialEntity,
	previous *models.ResourceCredentialEntity,
) error {
	purpose := resourceCredentialMetadataPurpose(credential.Metadata)
	if purpose == "administrator" {
		if previous == nil {
			return nil
		}
		if resourceCredentialMetadataPurpose(previous.Metadata) != "administrator" ||
			!credential.Username.Valid ||
			!previous.Username.Valid ||
			!strings.EqualFold(
				strings.TrimSpace(credential.Username.String),
				strings.TrimSpace(previous.Username.String),
			) {
			return domainError(
				"metadata.purpose",
				"administrator",
				"PostgreSQL administrator identity cannot be changed",
			)
		}
		currentValues, err := service.credentialSecretValues(*previous)
		if err != nil {
			return fmt.Errorf("load current PostgreSQL administrator credential: %w", err)
		}
		desiredValues, err := service.credentialSecretValues(credential)
		if err != nil {
			return fmt.Errorf("load desired PostgreSQL administrator credential: %w", err)
		}
		currentPassword, desiredPassword := currentValues["password"], desiredValues["password"]
		if currentPassword == "" || desiredPassword == "" {
			return errors.New("PostgreSQL Resource administrator credential is incomplete")
		}
		if currentPassword == desiredPassword {
			return nil
		}
		endpoint, err := models.ResourceEndpoint.FindPrimary(ctx, db, resource.ID)
		if err != nil {
			return fmt.Errorf("load PostgreSQL Resource primary endpoint: %w", err)
		}
		connection := postgresqlclient.Connection{
			Host: endpoint.Address, Port: endpoint.Port,
			Username: previous.Username.String, Password: currentPassword,
		}
		if err := service.postgres.RotateAdministratorPassword(
			ctx,
			connection,
			desiredPassword,
		); err != nil {
			return err
		}
		return nil
	}
	if purpose != "application" {
		return domainError(
			"metadata.purpose",
			"unsupported",
			"PostgreSQL credential purpose must be administrator or application",
		)
	}
	databaseName := resourceCredentialMetadataDatabase(credential.Metadata)
	if databaseName == "" || !resourceHasDatabase(resource, databaseName) {
		return domainError(
			"metadata.database",
			"database",
			"application credentials must select a configured Resource database",
		)
	}
	reconciliation, err := service.preparePostgreSQLCredentialReconciliation(
		ctx,
		db,
		resource,
		credential,
		previous,
	)
	if err != nil {
		return err
	}
	if err := service.postgres.ReconcileLoginRoleAcrossDatabases(
		ctx,
		reconciliation.Connection,
		[]string{databaseName},
		reconciliation.Username,
		reconciliation.Password,
		reconciliation.PreviousPassword,
	); err != nil {
		return fmt.Errorf(
			"reconcile PostgreSQL login role %q across Resource Databases: %w",
			reconciliation.Username,
			err,
		)
	}
	previousDatabase := ""
	if previous != nil && resourceCredentialMetadataPurpose(previous.Metadata) == "application" {
		previousDatabase = resourceCredentialMetadataDatabase(previous.Metadata)
	}
	if previousDatabase == "" || strings.EqualFold(previousDatabase, databaseName) {
		return nil
	}
	if err := service.postgres.RevokeLoginRoleDatabase(
		ctx,
		reconciliation.Connection,
		previousDatabase,
		reconciliation.Username,
	); err != nil {
		rollbackReconciliation, prepareErr := service.preparePostgreSQLCredentialReconciliation(
			context.WithoutCancel(ctx),
			db,
			resource,
			*previous,
			&credential,
		)
		var rollbackErr error
		if prepareErr == nil {
			rollbackErr = service.postgres.ReconcileLoginRoleAcrossDatabases(
				context.WithoutCancel(
					ctx,
				),
				rollbackReconciliation.Connection,
				[]string{previousDatabase},
				rollbackReconciliation.Username,
				rollbackReconciliation.Password,
				rollbackReconciliation.PreviousPassword,
			)
		}
		revokeTargetErr := service.postgres.RevokeLoginRoleDatabase(
			context.WithoutCancel(ctx),
			reconciliation.Connection,
			databaseName,
			reconciliation.Username,
		)
		return errors.Join(
			fmt.Errorf(
				"revoke PostgreSQL database %q access from role %q: %w",
				previousDatabase,
				reconciliation.Username,
				err,
			),
			prepareErr,
			rollbackErr,
			revokeTargetErr,
		)
	}
	return nil
}

func (service *ResourceManagement) reconcilePostgreSQLDatabaseCredentials(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
	database string,
) error {
	resource, err := models.Resource.Find(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return err
	}
	if resource.ArchivedAt.Valid || resource.Engine() != "postgresql" {
		return errors.New(
			"PostgreSQL credential reconciliation requires an active managed Resource",
		)
	}
	_ = installationID
	credentials, err := models.ResourceCredential.ActiveForResourceAll(
		ctx, service.db.Executor(), resourceID,
	)
	if err != nil {
		return err
	}
	for _, credential := range credentials {
		if resourceCredentialMetadataPurpose(credential.Metadata) != "application" ||
			!strings.EqualFold(resourceCredentialMetadataDatabase(credential.Metadata), database) {
			continue
		}
		if err := service.reconcilePostgreSQLCredentialInDatabase(
			ctx,
			service.db.Executor(),
			resource,
			credential,
			database,
		); err != nil {
			return err
		}
	}
	return nil
}

func (service *ResourceManagement) reconcilePostgreSQLCredentialInDatabase(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	credential models.ResourceCredentialEntity,
	database string,
) error {
	reconciliation, err := service.preparePostgreSQLCredentialReconciliation(
		ctx,
		db,
		resource,
		credential,
		&credential,
	)
	if err != nil {
		return err
	}
	if err := service.postgres.GrantLoginRoleDatabase(
		ctx,
		reconciliation.Connection,
		database,
		reconciliation.Username,
	); err != nil {
		return fmt.Errorf(
			"grant PostgreSQL database %q access to role %q: %w",
			database,
			reconciliation.Username,
			err,
		)
	}
	return nil
}

type postgreSQLCredentialReconciliation struct {
	Connection       postgresqlclient.Connection
	Username         string
	Password         string
	PreviousPassword string
}

func (service *ResourceManagement) preparePostgreSQLCredentialReconciliation(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	credential models.ResourceCredentialEntity,
	previous *models.ResourceCredentialEntity,
) (postgreSQLCredentialReconciliation, error) {
	topology, err := models.Resource.PostgreSQLCredentialTopology(ctx, db, resource.ID)
	if err != nil {
		return postgreSQLCredentialReconciliation{}, fmt.Errorf(
			"load Resource administrator and primary endpoint: %w",
			err,
		)
	}
	administratorPlaintext, err := secretcrypto.DecryptForPurpose(
		topology.AdministratorPayload,
		service.config.App.SessionEncryptionKey,
		resourceCredentialPurpose,
	)
	if err != nil {
		return postgreSQLCredentialReconciliation{}, fmt.Errorf(
			"decrypt Resource administrator credential: %w",
			err,
		)
	}
	var administratorPayload struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}
	if json.Unmarshal(administratorPlaintext, &administratorPayload) != nil ||
		administratorPayload.SchemaVersion != 1 {
		return postgreSQLCredentialReconciliation{}, errors.New(
			"Resource administrator credential is invalid",
		)
	}
	administratorPassword := administratorPayload.Values["password"]
	if administratorPassword == "" {
		return postgreSQLCredentialReconciliation{}, errors.New(
			"Resource administrator credential has no PostgreSQL password",
		)
	}
	targetValues, err := service.credentialSecretValues(credential)
	if err != nil {
		return postgreSQLCredentialReconciliation{}, fmt.Errorf(
			"load PostgreSQL login credential: %w",
			err,
		)
	}
	if !credential.Username.Valid || targetValues["password"] == "" {
		return postgreSQLCredentialReconciliation{}, errors.New(
			"PostgreSQL login credential requires a username and password",
		)
	}
	if strings.EqualFold(
		strings.TrimSpace(credential.Username.String),
		strings.TrimSpace(topology.AdministratorUsername),
	) {
		return postgreSQLCredentialReconciliation{}, domainError(
			"username",
			"administrator",
			"Application username must be different from the Resource administrator",
		)
	}
	previousPassword := ""
	if previous != nil && previous.Username.Valid &&
		strings.EqualFold(
			strings.TrimSpace(previous.Username.String),
			strings.TrimSpace(credential.Username.String),
		) {
		previousValues, err := service.credentialSecretValues(*previous)
		if err != nil {
			return postgreSQLCredentialReconciliation{}, fmt.Errorf(
				"load previous PostgreSQL login credential: %w",
				err,
			)
		}
		previousPassword = previousValues["password"]
	}
	return postgreSQLCredentialReconciliation{
		Connection: postgresqlclient.Connection{
			Host: topology.Address, Port: topology.Port,
			Username: topology.AdministratorUsername, Password: administratorPassword,
		},
		Username: strings.TrimSpace(
			credential.Username.String,
		),
		Password:         targetValues["password"],
		PreviousPassword: previousPassword,
	}, nil
}
