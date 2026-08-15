package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

func (service *ResourceManagement) CreateCredential(
	ctx context.Context,
	resourceID uuid.UUID,
	input ResourceCredentialInput,
) (models.ResourceCredentialEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	credential, err := service.createCredential(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	if resource.Engine() == "postgresql" {
		if err := service.reconcilePostgreSQLCredential(
			ctx,
			tx,
			resource,
			credential,
			nil,
		); err != nil {
			return models.ResourceCredentialEntity{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceCredentialEntity{}, mapResourceConflict(err)
	}
	return credential, nil
}

func (service *ResourceManagement) createCredential(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceCredentialInput,
) (models.ResourceCredentialEntity, error) {
	definition, ok := models.FindResourceEngine(resource.Engine())
	if !ok {
		return models.ResourceCredentialEntity{}, domainError(
			"kind",
			"unsupported",
			"resource kind is not supported",
		)
	}
	if resource.Engine() == "postgresql" {
		if err := service.validatePostgreSQLCredential(ctx, db, resource, input, nil); err != nil {
			return models.ResourceCredentialEntity{}, err
		}
	}
	encrypted, digest, err := service.credentialPayload(input, definition)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	created, err := models.ResourceCredential.Create(ctx, db, models.CreateResourceCredentialData{
		Name:       input.Name,
		Username:   nullableString(input.Username),
		Metadata:   normalizedJSON(input.Metadata),
		EncPayload: encrypted,
		Digest:     digest,
		ResourceID: resource.ID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) validatePostgreSQLCredential(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceCredentialInput,
	credentialID *uuid.UUID,
) error {
	username := strings.TrimSpace(input.Username)
	if username == "" {
		return domainError("username", "required", "PostgreSQL credentials require a username")
	}
	purpose := resourceCredentialMetadataPurpose(input.Metadata)
	if purpose != "administrator" && purpose != "application" {
		return domainError(
			"metadata.purpose",
			"unsupported",
			"PostgreSQL credential purpose must be administrator or application",
		)
	}
	if purpose == "application" &&
		!resourceHasDatabase(resource, resourceCredentialMetadataDatabase(input.Metadata)) {
		return domainError(
			"metadata.database",
			"database",
			"application credentials must select a configured Resource database",
		)
	}

	counts, err := models.ResourceCredential.PostgreSQLCounts(
		ctx, db, resource.ID, username, credentialID,
	)
	if err != nil {
		return err
	}
	administrators := counts.Administrators
	if purpose == "administrator" {
		administrators++
	}
	if administrators == 0 {
		return domainError(
			"metadata.purpose",
			"required",
			"PostgreSQL Resources must retain an administrator credential",
		)
	}
	if administrators > 1 {
		return domainError(
			"metadata.purpose",
			"unique",
			"database Resource already has an administrator credential",
		)
	}
	if counts.Usernames != 0 {
		return domainError(
			"username",
			"unique",
			"an active PostgreSQL credential already uses this username",
		)
	}
	return nil
}

func (service *ResourceManagement) UpdateCredentialMetadata(
	ctx context.Context,
	resourceID, credentialID uuid.UUID,
	input ResourceCredentialInput,
) (models.ResourceCredentialEntity, error) {
	return service.updateCredential(ctx, resourceID, credentialID, input, false)
}

func (service *ResourceManagement) RotateCredential(
	ctx context.Context,
	resourceID, credentialID uuid.UUID,
	input ResourceCredentialInput,
) (models.ResourceCredentialEntity, error) {
	return service.updateCredential(ctx, resourceID, credentialID, input, true)
}

func (service *ResourceManagement) updateCredential(
	ctx context.Context,
	resourceID, credentialID uuid.UUID,
	input ResourceCredentialInput,
	rotate bool,
) (models.ResourceCredentialEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	current, err := models.ResourceCredential.LockActiveForResource(
		ctx,
		tx,
		resourceID,
		credentialID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceCredentialEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	if resource.Engine() == "postgresql" {
		if !current.Username.Valid ||
			!strings.EqualFold(
				strings.TrimSpace(current.Username.String),
				strings.TrimSpace(input.Username),
			) {
			return models.ResourceCredentialEntity{}, domainError(
				"username",
				"immutable",
				"PostgreSQL credential usernames cannot be changed",
			)
		}
		if err := service.validatePostgreSQLCredential(
			ctx,
			tx,
			resource,
			input,
			&current.ID,
		); err != nil {
			return models.ResourceCredentialEntity{}, err
		}
	}
	encrypted, digest := current.EncPayload, current.Digest
	if rotate {
		definition, _ := models.FindResourceEngine(resource.Engine())
		encrypted, digest, err = service.credentialPayload(input, definition)
		if err != nil {
			return models.ResourceCredentialEntity{}, err
		}
	}
	candidate := models.ResourceCredentialEntity{
		ID: current.ID, Name: input.Name, Username: nullableString(input.Username),
		Metadata: normalizedJSON(input.Metadata), EncPayload: encrypted, Digest: digest,
		ArchivedAt: current.ArchivedAt, ResourceID: resourceID,
	}
	if err := candidate.Validate(); err != nil {
		return models.ResourceCredentialEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	reconciledPostgreSQL := false
	if resource.Engine() == "postgresql" {
		if err := service.reconcilePostgreSQLCredential(
			ctx,
			tx,
			resource,
			candidate,
			&current,
		); err != nil {
			return models.ResourceCredentialEntity{}, err
		}
		reconciledPostgreSQL = true
	}
	compensatePostgreSQL := func(db storage.Executor) error {
		if !reconciledPostgreSQL || !current.Username.Valid || !candidate.Username.Valid ||
			!strings.EqualFold(
				strings.TrimSpace(current.Username.String),
				strings.TrimSpace(candidate.Username.String),
			) {
			return nil
		}
		return service.reconcilePostgreSQLCredential(
			context.WithoutCancel(ctx),
			db,
			resource,
			current,
			&candidate,
		)
	}
	updated, err := models.ResourceCredential.Update(ctx, tx, models.UpdateResourceCredentialData{
		ID: current.ID, Name: input.Name, Username: nullableString(input.Username),
		Metadata: normalizedJSON(input.Metadata), EncPayload: encrypted, Digest: digest,
		ArchivedAt: current.ArchivedAt, ResourceID: resourceID,
	})
	if err != nil {
		return models.ResourceCredentialEntity{}, errors.Join(
			mapResourceConflict(err),
			compensatePostgreSQL(service.db.Executor()),
		)
	}
	connections, err := models.EnvironmentResource.ActiveForCredentialID(ctx, tx, credentialID)
	if err != nil {
		return models.ResourceCredentialEntity{}, errors.Join(
			err,
			compensatePostgreSQL(service.db.Executor()),
		)
	}
	if err := service.reconcileEnvironmentResourceConnections(
		ctx,
		tx,
		resource,
		connections,
	); err != nil {
		return models.ResourceCredentialEntity{}, errors.Join(
			err,
			compensatePostgreSQL(service.db.Executor()),
		)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceCredentialEntity{}, errors.Join(
			mapResourceConflict(err),
			compensatePostgreSQL(service.db.Executor()),
		)
	}
	return updated, nil
}

func connectionEnvironmentKeys(
	resource models.ResourceEntity,
	configuration environmentResourceConfiguration,
) map[string]string {
	keys := resource.EnvironmentKeys()
	maps.Copy(keys, configuration.EnvironmentKeyOverrides)
	return keys
}

func normalizeConnectionEnvironmentKeys(
	resource models.ResourceEntity,
	requested map[string]string,
) (map[string]string, map[string]string, error) {
	configuration := resource.ParsedConfiguration()
	configuration.EnvironmentKeys = maps.Clone(requested)
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return nil, nil, err
	}
	candidate := resource
	candidate.Configuration = encoded
	if err := validation.Validate(&candidate); err != nil {
		return nil, nil, errors.Join(models.ErrDomainValidation, err)
	}
	effective := candidate.EnvironmentKeys()
	defaults := resource.EnvironmentKeys()
	overrides := make(map[string]string)
	for logicalName, key := range effective {
		if defaults[logicalName] != key {
			overrides[logicalName] = key
		}
	}
	return effective, overrides, nil
}

func (service *ResourceManagement) UpdateConnectionEnvironmentKeys(
	ctx context.Context,
	resourceID, connectionID uuid.UUID,
	requested map[string]string,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return err
	}
	connection, err := models.EnvironmentResource.LockActiveConnection(
		ctx,
		tx,
		resourceID,
		connectionID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Join(
				models.ErrDomainValidation,
				errors.New("Resource connection is unavailable"),
			)
		}
		return err
	}
	effectiveKeys, overrides, err := normalizeConnectionEnvironmentKeys(resource, requested)
	if err != nil {
		return err
	}
	configuration, err := parseEnvironmentResourceConfiguration(connection.Configuration)
	if err != nil {
		return err
	}
	endpoint, err := models.ResourceEndpoint.Find(ctx, tx, connection.ResourceEndpointID)
	if err != nil || endpoint.ArchivedAt.Valid || endpoint.ResourceID != resource.ID {
		return errors.New("Environment Resource endpoint is unavailable during projection")
	}
	var credential *models.ResourceCredentialEntity
	credentialValues := make(map[string]string)
	if connection.ResourceCredentialID != nil {
		selected, findErr := models.ResourceCredential.Find(
			ctx,
			tx,
			*connection.ResourceCredentialID,
		)
		if findErr != nil || selected.ArchivedAt.Valid || selected.ResourceID != resource.ID {
			return errors.New("Environment Resource credential is unavailable during projection")
		}
		credentialValues, err = service.credentialSecretValues(selected)
		if err != nil {
			return err
		}
		credential = &selected
	}
	values, projectedKeys, err := service.resourceProjectionValuesForEnvironment(
		connection.EnvironmentID,
		resource,
		endpoint,
		credential,
		credentialValues,
		configuration.CredentialProjection,
		effectiveKeys,
	)
	if err != nil {
		return err
	}
	database := ""
	if credential != nil {
		database = resourceCredentialMetadataDatabase(credential.Metadata)
	}
	if err := service.secrets.ReconcileManagedResource(
		ctx,
		tx,
		connection,
		values,
		projectedKeys,
		overrides,
		database,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) reconcileEnvironmentResourceConnections(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	connections []models.EnvironmentResourceEntity,
) error {
	for _, connection := range connections {
		if connection.ResourceID != resource.ID {
			return errors.New(
				"Environment Resource connection does not belong to the changed Resource",
			)
		}
		endpoint, err := models.ResourceEndpoint.Find(ctx, db, connection.ResourceEndpointID)
		if err != nil || endpoint.ArchivedAt.Valid || endpoint.ResourceID != resource.ID {
			return errors.New("Environment Resource endpoint is unavailable during projection")
		}
		var credential *models.ResourceCredentialEntity
		credentialValues := make(map[string]string)
		if connection.ResourceCredentialID != nil {
			selected, findErr := models.ResourceCredential.Find(
				ctx,
				db,
				*connection.ResourceCredentialID,
			)
			if findErr != nil || selected.ArchivedAt.Valid || selected.ResourceID != resource.ID {
				return errors.New(
					"Environment Resource credential is unavailable during projection",
				)
			}
			credentialValues, err = service.credentialSecretValues(selected)
			if err != nil {
				return err
			}
			credential = &selected
		}
		configuration, err := parseEnvironmentResourceConfiguration(connection.Configuration)
		if err != nil {
			return err
		}
		effectiveKeys := connectionEnvironmentKeys(resource, configuration)
		values, environmentKeys, err := service.resourceProjectionValuesForEnvironment(
			connection.EnvironmentID,
			resource,
			endpoint,
			credential,
			credentialValues,
			configuration.CredentialProjection,
			effectiveKeys,
		)
		if err != nil {
			return err
		}
		database := ""
		if credential != nil {
			database = resourceCredentialMetadataDatabase(credential.Metadata)
		}
		if err := service.secrets.ReconcileManagedResource(
			ctx,
			db,
			connection,
			values,
			environmentKeys,
			configuration.EnvironmentKeyOverrides,
			database,
		); err != nil {
			return err
		}
	}
	return nil
}

func (service *ResourceManagement) resourceProjectionValuesForEnvironment(
	environmentID uuid.UUID,
	resource models.ResourceEntity,
	endpoint models.ResourceEndpointEntity,
	credential *models.ResourceCredentialEntity,
	credentialValues map[string]string,
	projection string,
	resourceKeys map[string]string,
) (map[string]string, map[string]string, error) {
	if resource.Engine() == "opentelemetry" {
		if credential == nil ||
			resourceCredentialMetadataEnvironmentID(credential.Metadata) != environmentID {
			return nil, nil, errors.New(
				"OpenTelemetry Resource credential does not belong to this Environment",
			)
		}
	}
	identityToken := credentialValues["token"]
	return resourceProjectionValues(
		resource,
		endpoint,
		credential,
		credentialValues,
		projection,
		resourceKeys,
		identityToken,
	)
}

func (service *ResourceManagement) ArchiveCredential(
	ctx context.Context,
	resourceID, credentialID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	_, err = models.ResourceCredential.LockActiveForResource(ctx, tx, resourceID, credentialID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	dependencies, err := models.ResourceCredential.ActiveDependencyCount(ctx, tx, credentialID)
	if err != nil {
		return err
	}
	if dependencies > 0 {
		return domainError(
			"credential",
			"dependency",
			"credential is selected by an active binding or health check",
		)
	}
	now := time.Now().UTC()
	if err := models.ResourceCredential.ArchiveID(ctx, tx, credentialID, now); err != nil {
		return err
	}
	return tx.Commit()
}
