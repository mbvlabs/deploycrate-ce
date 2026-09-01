package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	environmentSecretEncryptionPurpose = "environment-secret/v1"
	environmentSecretDigestKeyContext  = "environment-secret/digest-key/v1"
	environmentSecretValueContext      = "environment-secret/value/v1\x00"
)

type EnvironmentSecrets struct {
	db       storage.Pool
	config   config.Config
	releases *ReleaseDeployment
}

func NewEnvironmentSecrets(
	db storage.Pool,
	cfg config.Config,
	releases *ReleaseDeployment,
) *EnvironmentSecrets {
	return &EnvironmentSecrets{db: db, config: cfg, releases: releases}
}

type PreparedEnvironmentSecret struct {
	Key           string
	EncValue      []byte
	Digest        []byte
	SourceType    string
	SourceID      uuid.UUID
	EnvironmentID uuid.UUID
}

type EnvironmentSecretMutation struct {
	Secret   models.EnvironmentSecretMetadata `json:"secret"`
	Revision uuid.UUID                        `json:"revisionId"`
	NoOp     bool                             `json:"noOp"`
}

type EnvironmentSecretInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ResolvedEnvironmentSecret struct {
	Key   string
	Value string
}

func (service *EnvironmentSecrets) Prepare(
	environmentID uuid.UUID,
	key, value, sourceType string,
	sourceID uuid.UUID,
) (PreparedEnvironmentSecret, error) {
	key = models.NormalizeEnvironmentSecretKey(key)
	if err := models.ValidateEnvironmentSecretKey(key, false); err != nil {
		return PreparedEnvironmentSecret{}, errors.Join(models.ErrDomainValidation, err)
	}
	if value == "" {
		return PreparedEnvironmentSecret{}, errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{Field: "value", Code: "required", Message: "secret value is required"},
			},
		)
	}
	if len(value) > models.EnvironmentSecretValueMaxBytes {
		return PreparedEnvironmentSecret{}, errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "value",
					Code:    "max_length",
					Message: "secret value must not exceed 65536 bytes",
				},
			},
		)
	}
	if environmentID == uuid.Nil || sourceID == uuid.Nil {
		return PreparedEnvironmentSecret{}, errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "sourceId",
					Code:    "required",
					Message: "Environment and secret owner are required",
				},
			},
		)
	}
	if sourceType != models.EnvironmentSecretSourceUser &&
		sourceType != models.EnvironmentSecretSourceResource {
		return PreparedEnvironmentSecret{}, errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "sourceType",
					Code:    "unsupported",
					Message: "secret source type is unsupported",
				},
			},
		)
	}
	encrypted, err := secretcrypto.EncryptForPurpose(
		[]byte(value),
		service.config.App.SessionEncryptionKey,
		environmentSecretEncryptionPurpose,
	)
	if err != nil {
		return PreparedEnvironmentSecret{}, fmt.Errorf("encrypt Environment secret: %w", err)
	}
	digest, err := service.digest(environmentID, key, value)
	if err != nil {
		return PreparedEnvironmentSecret{}, err
	}
	return PreparedEnvironmentSecret{
		Key: key, EncValue: encrypted, Digest: digest, SourceType: sourceType,
		SourceID: sourceID, EnvironmentID: environmentID,
	}, nil
}

func (service *EnvironmentSecrets) CreatePrepared(
	ctx context.Context,
	db storage.Executor,
	prepared PreparedEnvironmentSecret,
) (models.EnvironmentSecretEntity, error) {
	return models.EnvironmentSecret.Create(ctx, db, models.CreateEnvironmentSecretData{
		Key:           prepared.Key,
		EncValue:      prepared.EncValue,
		Digest:        prepared.Digest,
		SourceType:    prepared.SourceType,
		SourceID:      prepared.SourceID,
		EnvironmentID: prepared.EnvironmentID,
	})
}

func (service *EnvironmentSecrets) CreateUser(
	ctx context.Context,
	applicationID, environmentID, userID uuid.UUID,
	key, value string,
) (EnvironmentSecretMutation, error) {
	prepared, err := service.Prepare(
		environmentID,
		key,
		value,
		models.EnvironmentSecretSourceUser,
		userID,
	)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	defer tx.Rollback()
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	setupComplete, err := models.Environment.SetupComplete(ctx, tx, environmentID)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	if environment.ApplicationID != applicationID || !setupComplete ||
		environment.ArchivedAt.Valid {
		return EnvironmentSecretMutation{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment setup must be complete"),
		)
	}
	secret, err := service.CreatePrepared(ctx, tx, prepared)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	revision, err := service.commitSecretRevision(
		ctx,
		tx,
		environment,
		userID,
		"secret_create",
		nil,
		&secret,
	)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	if err := tx.Commit(); err != nil {
		return EnvironmentSecretMutation{}, err
	}
	return EnvironmentSecretMutation{Secret: secret.Sanitized(), Revision: revision.ID}, nil
}

func (service *EnvironmentSecrets) BulkCreateUser(
	ctx context.Context,
	applicationID, environmentID, userID uuid.UUID,
	secrets []EnvironmentSecretInput,
) (EnvironmentSecretMutation, error) {
	if len(secrets) == 0 {
		return EnvironmentSecretMutation{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("at least one secret is required"),
		)
	}
	prepared := make([]PreparedEnvironmentSecret, 0, len(secrets))
	seen := make(map[string]struct{}, len(secrets))
	for _, secret := range secrets {
		key := models.NormalizeEnvironmentSecretKey(secret.Key)
		if _, exists := seen[key]; exists {
			return EnvironmentSecretMutation{}, errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{{
					Field:   "key",
					Code:    "duplicate",
					Message: fmt.Sprintf("secret key %q is listed more than once", key),
				}},
			)
		}
		seen[key] = struct{}{}
		item, err := service.Prepare(
			environmentID,
			key,
			secret.Value,
			models.EnvironmentSecretSourceUser,
			userID,
		)
		if err != nil {
			return EnvironmentSecretMutation{}, err
		}
		prepared = append(prepared, item)
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	defer tx.Rollback()
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	setupComplete, err := models.Environment.SetupComplete(ctx, tx, environmentID)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	if environment.ApplicationID != applicationID || !setupComplete ||
		environment.ArchivedAt.Valid {
		return EnvironmentSecretMutation{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment setup must be complete"),
		)
	}
	base, err := models.EnvironmentStateRevision.LatestCommitted(ctx, tx, environment.ID)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	state, err := models.ParseEnvironmentDesiredState(base.State)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	existing := make(map[string]struct{}, len(state.Secrets)+len(prepared))
	for _, descriptor := range state.Secrets {
		existing[descriptor.Key] = struct{}{}
	}
	changes := make([]secretRevisionChange, 0, len(prepared))
	for _, item := range prepared {
		if _, exists := existing[item.Key]; exists {
			return EnvironmentSecretMutation{}, errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{{
					Field:   "key",
					Code:    "reserved",
					Message: fmt.Sprintf("secret key %q already exists", item.Key),
				}},
			)
		}
		existing[item.Key] = struct{}{}
		secret, err := service.CreatePrepared(ctx, tx, item)
		if err != nil {
			return EnvironmentSecretMutation{}, err
		}
		changes = append(changes, secretRevisionChange{requested: &secret})
	}
	revision, err := service.commitSecretsRevision(
		ctx,
		tx,
		environment,
		userID,
		"secret_create",
		changes,
	)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	if err := tx.Commit(); err != nil {
		return EnvironmentSecretMutation{}, err
	}
	return EnvironmentSecretMutation{Revision: revision.ID}, nil
}

func (service *EnvironmentSecrets) RotateUser(
	ctx context.Context,
	applicationID, environmentID, secretID, userID uuid.UUID,
	value string,
) (EnvironmentSecretMutation, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	defer tx.Rollback()
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	setupComplete, err := models.Environment.SetupComplete(ctx, tx, environmentID)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	if environment.ApplicationID != applicationID || !setupComplete ||
		environment.ArchivedAt.Valid {
		return EnvironmentSecretMutation{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment setup must be complete"),
		)
	}
	current, err := models.EnvironmentSecret.FindForEnvironment(ctx, tx, environmentID, secretID)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	if current.ArchivedAt.Valid || current.SourceType != models.EnvironmentSecretSourceUser {
		return EnvironmentSecretMutation{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("only an active user-owned secret can be rotated"),
		)
	}
	prepared, err := service.Prepare(
		environmentID,
		current.Key,
		value,
		models.EnvironmentSecretSourceUser,
		userID,
	)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	if hmac.Equal(current.Digest, prepared.Digest) {
		return EnvironmentSecretMutation{Secret: current.Sanitized(), NoOp: true}, tx.Commit()
	}
	if err := models.EnvironmentSecret.Archive(ctx, tx, environmentID, current.ID); err != nil {
		return EnvironmentSecretMutation{}, err
	}
	next, err := service.CreatePrepared(ctx, tx, prepared)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	revision, err := service.commitSecretRevision(
		ctx,
		tx,
		environment,
		userID,
		"secret_rotate",
		&current,
		&next,
	)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	if err := tx.Commit(); err != nil {
		return EnvironmentSecretMutation{}, err
	}
	return EnvironmentSecretMutation{Secret: next.Sanitized(), Revision: revision.ID}, nil
}

func (service *EnvironmentSecrets) ArchiveUser(
	ctx context.Context,
	applicationID, environmentID, secretID, userID uuid.UUID,
) (EnvironmentSecretMutation, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	defer tx.Rollback()
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	setupComplete, err := models.Environment.SetupComplete(ctx, tx, environmentID)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	if environment.ApplicationID != applicationID || !setupComplete ||
		environment.ArchivedAt.Valid {
		return EnvironmentSecretMutation{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment setup must be complete"),
		)
	}
	current, err := models.EnvironmentSecret.FindForEnvironment(ctx, tx, environmentID, secretID)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	if current.ArchivedAt.Valid || current.SourceType != models.EnvironmentSecretSourceUser {
		return EnvironmentSecretMutation{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("only an active user-owned secret can be archived"),
		)
	}
	if err := models.EnvironmentSecret.Archive(ctx, tx, environmentID, current.ID); err != nil {
		return EnvironmentSecretMutation{}, err
	}
	revision, err := service.commitSecretRevision(
		ctx,
		tx,
		environment,
		userID,
		"secret_archive",
		&current,
		nil,
	)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	if err := tx.Commit(); err != nil {
		return EnvironmentSecretMutation{}, err
	}
	return EnvironmentSecretMutation{Secret: current.Sanitized(), Revision: revision.ID}, nil
}

func (service *EnvironmentSecrets) ResolveRevision(
	ctx context.Context,
	revision models.EnvironmentStateRevisionEntity,
) ([]ResolvedEnvironmentSecret, error) {
	secrets, err := models.EnvironmentStateRevision.ResolveSecrets(
		ctx,
		service.db.Executor(),
		revision,
	)
	if err != nil {
		return nil, err
	}
	resolved := make([]ResolvedEnvironmentSecret, 0, len(secrets))
	for _, secret := range secrets {
		plaintext, err := secretcrypto.DecryptForPurpose(
			secret.EncValue,
			service.config.App.SessionEncryptionKey,
			environmentSecretEncryptionPurpose,
		)
		if err != nil {
			return nil, fmt.Errorf("decrypt Environment secret %s: %w", secret.Key, err)
		}
		resolved = append(
			resolved,
			ResolvedEnvironmentSecret{Key: secret.Key, Value: string(plaintext)},
		)
	}
	return resolved, nil
}

func (service *EnvironmentSecrets) ReconcileManagedResource(
	ctx context.Context,
	db storage.Executor,
	connection models.EnvironmentResourceEntity,
	values map[string]string,
	environmentKeys map[string]string,
	environmentKeyOverrides map[string]string,
	database string,
) error {
	if connection.ArchivedAt.Valid || len(values) == 0 || len(environmentKeys) == 0 {
		return errors.New("active Resource connection values are required")
	}
	environment, err := models.Environment.Lock(ctx, db, connection.EnvironmentID)
	if err != nil || environment.ArchivedAt.Valid {
		return errors.New("Resource connection Environment is unavailable")
	}
	setupComplete, err := models.Environment.SetupComplete(ctx, db, environment.ID)
	if err != nil || !setupComplete {
		return errors.New("Resource connection Environment is unavailable")
	}
	base, err := models.EnvironmentStateRevision.LatestCommitted(ctx, db, environment.ID)
	if err != nil {
		return err
	}
	state, err := models.ParseEnvironmentDesiredState(base.State)
	if err != nil {
		return err
	}
	connectionConfiguration, err := parseEnvironmentResourceConfiguration(connection.Configuration)
	if err != nil {
		return err
	}
	resourceStateIndex := -1
	for index := range state.Resources {
		if state.Resources[index].EnvironmentResourceID == connection.ID {
			resourceStateIndex = index
			break
		}
	}
	if resourceStateIndex < 0 {
		return errors.New("Resource connection is missing from the current Environment revision")
	}
	currentDescriptors := make(map[string]models.EnvironmentSecretDescriptor)
	currentEntities := make(map[string]models.EnvironmentSecretEntity)
	for _, descriptor := range state.Secrets {
		if descriptor.SourceType != models.EnvironmentSecretSourceResource ||
			descriptor.SourceID != connection.ID {
			continue
		}
		current, findErr := models.EnvironmentSecret.FindForEnvironment(
			ctx,
			db,
			environment.ID,
			descriptor.ID,
		)
		if findErr != nil {
			return errors.New("active Resource-managed secret value is unavailable")
		}
		currentDescriptors[descriptor.Key] = descriptor
		currentEntities[descriptor.Key] = current
	}
	desiredKeys := make(map[string]struct{}, len(environmentKeys))
	for _, key := range environmentKeys {
		desiredKeys[models.NormalizeEnvironmentSecretKey(key)] = struct{}{}
	}
	for _, resourceState := range state.Resources {
		if resourceState.EnvironmentResourceID == connection.ID {
			continue
		}
		for key := range resourceState.Variables {
			if _, conflicts := desiredKeys[models.NormalizeEnvironmentSecretKey(key)]; conflicts {
				return errors.Join(models.ErrDomainValidation, validation.ValidationErrors{
					{
						Field:   "configuration.environment_keys",
						Code:    "duplicate",
						Message: "Resource key " + key + " conflicts with a legacy value in Environment " + environment.Name,
					},
				})
			}
		}
		for _, key := range resourceState.EnvironmentKeys {
			if _, conflicts := desiredKeys[models.NormalizeEnvironmentSecretKey(key)]; conflicts {
				return errors.Join(models.ErrDomainValidation, validation.ValidationErrors{
					{
						Field:   "configuration.environment_keys",
						Code:    "duplicate",
						Message: "Resource key " + key + " is already managed in Environment " + environment.Name,
					},
				})
			}
		}
	}
	activeSecrets, err := models.EnvironmentSecret.ActiveForEnvironment(ctx, db, environment.ID)
	if err != nil {
		return err
	}
	for _, active := range activeSecrets {
		if active.SourceType == models.EnvironmentSecretSourceResource &&
			active.SourceID == connection.ID {
			continue
		}
		if _, conflicts := desiredKeys[models.NormalizeEnvironmentSecretKey(active.Key)]; conflicts {
			return errors.Join(models.ErrDomainValidation, validation.ValidationErrors{
				{
					Field:   "configuration.environment_keys",
					Code:    "duplicate",
					Message: "Resource key " + active.Key + " conflicts in Environment " + environment.Name,
				},
			})
		}
	}
	preparedByKey := make(map[string]PreparedEnvironmentSecret, len(values))
	unchanged := make(map[string]models.EnvironmentSecretDescriptor, len(values))
	previousResourceState := state.Resources[resourceStateIndex]
	resourceStateChanged := !maps.Equal(previousResourceState.EnvironmentKeys, environmentKeys) ||
		previousResourceState.Database != database
	connectionConfigurationChanged := !maps.Equal(
		connectionConfiguration.EnvironmentKeys,
		environmentKeys,
	) ||
		!maps.Equal(connectionConfiguration.EnvironmentKeyOverrides, environmentKeyOverrides)
	changed := resourceStateChanged || len(currentDescriptors) != len(values)
	for key, value := range values {
		prepared, prepareErr := service.Prepare(
			environment.ID,
			key,
			value,
			models.EnvironmentSecretSourceResource,
			connection.ID,
		)
		if prepareErr != nil {
			return prepareErr
		}
		preparedByKey[key] = prepared
		if current, exists := currentEntities[key]; exists && !current.ArchivedAt.Valid &&
			hmac.Equal(current.Digest, prepared.Digest) {
			unchanged[key] = currentDescriptors[key]
		} else {
			changed = true
		}
	}
	connectionConfiguration.EnvironmentKeys = maps.Clone(environmentKeys)
	connectionConfiguration.EnvironmentKeyOverrides = maps.Clone(environmentKeyOverrides)
	encodedConfiguration, err := json.Marshal(connectionConfiguration)
	if err != nil {
		return err
	}
	if !changed {
		if !connectionConfigurationChanged {
			return nil
		}
		_, err := models.EnvironmentResource.Update(ctx, db, models.UpdateEnvironmentResourceData{
			ID: connection.ID, Alias: connection.Alias, Configuration: encodedConfiguration,
			ArchivedAt: connection.ArchivedAt, EnvironmentID: connection.EnvironmentID,
			ResourceID: connection.ResourceID, ResourceEndpointID: connection.ResourceEndpointID,
			ResourceCredentialID: connection.ResourceCredentialID,
		})
		return err
	}
	activeBuilds, err := models.Build.ActiveCountForEnvironment(ctx, db, environment.ID)
	if err != nil {
		return err
	}
	if activeBuilds > 0 {
		return errors.Join(models.ErrDomainValidation, validation.ValidationErrors{
			{
				Field:   "resource",
				Code:    "build_active",
				Message: "wait for active Builds in Environment " + environment.Name + " before changing this Resource",
			},
		})
	}
	activeDeployments, err := models.Deployment.ActiveCountForEnvironment(ctx, db, environment.ID)
	if err != nil {
		return err
	}
	if activeDeployments > 0 {
		return errors.Join(models.ErrDomainValidation, validation.ValidationErrors{
			{
				Field:   "resource",
				Code:    "deployment_active",
				Message: "wait for active deployments in Environment " + environment.Name + " before changing this Resource",
			},
		})
	}
	for key, current := range currentEntities {
		if _, keep := unchanged[key]; keep {
			continue
		}
		if !current.ArchivedAt.Valid {
			if err := models.EnvironmentSecret.Archive(
				ctx,
				db,
				environment.ID,
				current.ID,
			); err != nil {
				return err
			}
		}
	}
	descriptors := make(map[string]models.EnvironmentSecretDescriptor, len(values))
	maps.Copy(descriptors, unchanged)
	created := make(map[string]models.EnvironmentSecretEntity)
	for key, prepared := range preparedByKey {
		if _, keep := unchanged[key]; keep {
			continue
		}
		next, createErr := service.CreatePrepared(ctx, db, prepared)
		if createErr != nil {
			return createErr
		}
		created[key] = next
		descriptors[key] = models.EnvironmentSecretDescriptorFromEntity(next)
	}
	nextSecrets := make(
		[]models.EnvironmentSecretDescriptor,
		0,
		len(state.Secrets)-len(currentDescriptors)+len(descriptors),
	)
	for _, descriptor := range state.Secrets {
		if descriptor.SourceType == models.EnvironmentSecretSourceResource &&
			descriptor.SourceID == connection.ID {
			continue
		}
		nextSecrets = append(nextSecrets, descriptor)
	}
	for _, descriptor := range descriptors {
		nextSecrets = append(nextSecrets, descriptor)
	}
	state.Secrets = nextSecrets
	state.Resources[resourceStateIndex].EnvironmentKeys = maps.Clone(environmentKeys)
	state.Resources[resourceStateIndex].Database = database
	state.Resources[resourceStateIndex].Variables = nil
	canonical, err := models.CanonicalEnvironmentDesiredState(state)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	sequence, err := models.Change.NextSequence(ctx, db, environment.ID)
	if err != nil {
		return err
	}
	change, err := models.Change.Create(ctx, db, models.CreateChangeData{
		Sequence:    sequence,
		Kind:        "resource_projection",
		TriggerType: "system",
		ActorType:   "system",
		CauseSystem: sql.NullString{
			String: "resource",
			Valid:  true,
		},
		CauseReference: sql.NullString{String: connection.ResourceID.String(), Valid: true},
		CorrelationID:  uuid.New(),
		Summary:        "Update Resource-managed Environment secrets",
		Status:         "committed",
		RequestedAt:    now,
		CommittedAt:    sql.NullTime{Time: now, Valid: true},
		EnvironmentID:  environment.ID,
	})
	if err != nil {
		return err
	}
	changeKeys := make(map[string]struct{}, len(currentDescriptors)+len(created))
	for key := range currentDescriptors {
		changeKeys[key] = struct{}{}
	}
	for key := range created {
		changeKeys[key] = struct{}{}
	}
	for key := range changeKeys {
		if _, keep := unchanged[key]; keep {
			continue
		}
		previous, hadPrevious := currentDescriptors[key]
		next, hasNext := descriptors[key]
		action := "rotate"
		subjectID := next.ID
		if !hadPrevious {
			action = "create"
		} else if !hasNext {
			action = "archive"
			subjectID = previous.ID
		}
		oldValue := json.RawMessage(`null`)
		newValue := json.RawMessage(`null`)
		if hadPrevious {
			oldValue, _ = json.Marshal(previous)
		}
		if hasNext {
			newValue, _ = json.Marshal(next)
		}
		if _, err := models.ChangeItem.Create(ctx, db, models.CreateChangeItemData{
			Action: action, SubjectType: "environment_secret", SubjectID: subjectID,
			PreviousValue: oldValue, RequestedValue: newValue, ChangeID: change.ID,
		}); err != nil {
			return err
		}
	}
	if resourceStateChanged {
		previousValue, _ := json.Marshal(previousResourceState)
		requestedValue, _ := json.Marshal(state.Resources[resourceStateIndex])
		if _, err := models.ChangeItem.Create(ctx, db, models.CreateChangeItemData{
			Action: "update", SubjectType: "environment_resource", SubjectID: connection.ID,
			PreviousValue: previousValue, RequestedValue: requestedValue, ChangeID: change.ID,
		}); err != nil {
			return err
		}
	} else if connectionConfigurationChanged {
		if _, err := models.ChangeItem.Create(ctx, db, models.CreateChangeItemData{
			Action:         "update",
			SubjectType:    "environment_resource",
			SubjectID:      connection.ID,
			PreviousValue:  connection.Configuration,
			RequestedValue: encodedConfiguration,
			ChangeID:       change.ID,
		}); err != nil {
			return err
		}
	}
	revision, err := models.EnvironmentStateRevision.Create(
		ctx,
		db,
		models.CreateEnvironmentStateRevisionData{
			State:         canonical,
			EnvironmentID: environment.ID,
			ChangeID:      change.ID,
		},
	)
	if err != nil {
		return err
	}
	if _, err := models.ChangeStateRevision.Create(
		ctx,
		db,
		models.CreateChangeStateRevisionData{
			Role:                       "base",
			ChangeID:                   change.ID,
			EnvironmentStateRevisionID: base.ID,
		},
	); err != nil {
		return err
	}
	if _, err := models.ChangeStateRevision.Create(
		ctx,
		db,
		models.CreateChangeStateRevisionData{
			Role:                       "result",
			ChangeID:                   change.ID,
			EnvironmentStateRevisionID: revision.ID,
		},
	); err != nil {
		return err
	}
	if err := models.EnvironmentTargetState.SetDesiredRevisionForEnvironment(
		ctx,
		db,
		environment.ID,
		revision.ID,
		now,
	); err != nil {
		return err
	}
	if _, err := models.EnvironmentResource.Update(ctx, db, models.UpdateEnvironmentResourceData{
		ID: connection.ID, Alias: connection.Alias, Configuration: encodedConfiguration,
		ArchivedAt: connection.ArchivedAt, EnvironmentID: connection.EnvironmentID,
		ResourceID: connection.ResourceID, ResourceEndpointID: connection.ResourceEndpointID,
		ResourceCredentialID: connection.ResourceCredentialID,
	}); err != nil {
		return err
	}
	return service.queueRevisionDeployment(ctx, db, change, revision)
}

func (service *EnvironmentSecrets) digest(
	environmentID uuid.UUID,
	key, value string,
) ([]byte, error) {
	masterKey, err := hex.DecodeString(service.config.App.SessionEncryptionKey)
	if err != nil || len(masterKey) != 32 {
		return nil, errors.New("Environment secret digest key is invalid")
	}
	keyMAC := hmac.New(sha256.New, masterKey)
	_, _ = keyMAC.Write([]byte(environmentSecretDigestKeyContext))
	digestKey := keyMAC.Sum(nil)
	valueMAC := hmac.New(sha256.New, digestKey)
	_, _ = valueMAC.Write([]byte(environmentSecretValueContext))
	_, _ = valueMAC.Write([]byte(environmentID.String()))
	_, _ = valueMAC.Write([]byte{0})
	_, _ = valueMAC.Write([]byte(key))
	_, _ = valueMAC.Write([]byte{0})
	_, _ = valueMAC.Write([]byte(value))
	return valueMAC.Sum(nil), nil
}

type secretRevisionChange struct {
	previous, requested *models.EnvironmentSecretEntity
}

func (service *EnvironmentSecrets) secretCommitBaseRevision(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
) (models.EnvironmentStateRevisionEntity, error) {
	latest, err := models.EnvironmentStateRevision.LatestCommitted(ctx, db, environmentID)
	if err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	targetStates, err := models.EnvironmentTargetState.ActiveForEnvironment(ctx, db, environmentID)
	if err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	if len(targetStates) == 0 {
		return latest, nil
	}
	var consensusDesired *uuid.UUID
	for _, targetState := range targetStates {
		if targetState.DesiredRevisionID == nil {
			return latest, nil
		}
		if consensusDesired == nil {
			consensusDesired = targetState.DesiredRevisionID
			continue
		}
		if *consensusDesired != *targetState.DesiredRevisionID {
			return latest, nil
		}
	}
	if consensusDesired == nil || *consensusDesired == latest.ID {
		return latest, nil
	}
	allAppliedMatchDesired := true
	for _, targetState := range targetStates {
		if targetState.AppliedRevisionID == nil ||
			*targetState.AppliedRevisionID != *consensusDesired {
			allAppliedMatchDesired = false
			break
		}
	}
	if !allAppliedMatchDesired {
		return latest, nil
	}
	return models.EnvironmentStateRevision.Find(ctx, db, *consensusDesired)
}

func (service *EnvironmentSecrets) commitSecretRevision(
	ctx context.Context,
	db storage.Executor,
	environment models.EnvironmentEntity,
	actorID uuid.UUID,
	kind string,
	previous, requested *models.EnvironmentSecretEntity,
) (models.EnvironmentStateRevisionEntity, error) {
	return service.commitSecretsRevision(
		ctx,
		db,
		environment,
		actorID,
		kind,
		[]secretRevisionChange{{previous: previous, requested: requested}},
	)
}

func (service *EnvironmentSecrets) commitSecretsRevision(
	ctx context.Context,
	db storage.Executor,
	environment models.EnvironmentEntity,
	actorID uuid.UUID,
	kind string,
	changes []secretRevisionChange,
) (models.EnvironmentStateRevisionEntity, error) {
	if _, err := models.Environment.Lock(ctx, db, environment.ID); err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	base, err := service.secretCommitBaseRevision(ctx, db, environment.ID)
	if err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	state, err := models.ParseEnvironmentDesiredState(base.State)
	if err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	secrets := make([]models.EnvironmentSecretDescriptor, 0, len(state.Secrets)+len(changes))
	for _, descriptor := range state.Secrets {
		removed := false
		for _, change := range changes {
			if change.previous != nil && descriptor.ID == change.previous.ID {
				removed = true
				break
			}
		}
		if removed {
			continue
		}
		secret, findErr := models.EnvironmentSecret.FindForEnvironment(
			ctx,
			db,
			environment.ID,
			descriptor.ID,
		)
		if findErr != nil {
			return models.EnvironmentStateRevisionEntity{}, findErr
		}
		if secret.ArchivedAt.Valid {
			continue
		}
		secrets = append(secrets, models.EnvironmentSecretDescriptorFromEntity(secret))
	}
	for _, change := range changes {
		if change.requested != nil {
			secrets = append(
				secrets,
				models.EnvironmentSecretDescriptorFromEntity(*change.requested),
			)
		}
	}
	state.Secrets = secrets
	canonical, err := models.CanonicalEnvironmentDesiredState(state)
	if err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	now := time.Now().UTC()
	sequence, err := models.Change.NextSequence(ctx, db, environment.ID)
	if err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	change, err := models.Change.Create(ctx, db, models.CreateChangeData{
		Sequence:      sequence,
		Kind:          kind,
		TriggerType:   "user",
		ActorType:     "user",
		ActorID:       &actorID,
		CorrelationID: uuid.New(),
		Summary:       strings.ReplaceAll(kind, "_", " "),
		Status:        "committed",
		RequestedAt:   now,
		CommittedAt:   sql.NullTime{Time: now, Valid: true},
		EnvironmentID: environment.ID,
	})
	if err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	changeValue := func(secret *models.EnvironmentSecretEntity) json.RawMessage {
		if secret == nil {
			return nil
		}
		value, _ := json.Marshal(models.EnvironmentSecretDescriptorFromEntity(*secret))
		return value
	}
	for _, item := range changes {
		subjectID := uuid.Nil
		if item.requested != nil {
			subjectID = item.requested.ID
		} else if item.previous != nil {
			subjectID = item.previous.ID
		}
		if _, err := models.ChangeItem.Create(ctx, db, models.CreateChangeItemData{
			Action:         kind,
			SubjectType:    "environment_secret",
			SubjectID:      subjectID,
			PreviousValue:  changeValue(item.previous),
			RequestedValue: changeValue(item.requested),
			ChangeID:       change.ID,
		}); err != nil {
			return models.EnvironmentStateRevisionEntity{}, err
		}
	}
	revision, err := models.EnvironmentStateRevision.Create(
		ctx,
		db,
		models.CreateEnvironmentStateRevisionData{
			State: canonical, EnvironmentID: environment.ID, ChangeID: change.ID,
		},
	)
	if err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	if _, err := models.ChangeStateRevision.Create(
		ctx,
		db,
		models.CreateChangeStateRevisionData{
			Role:                       "base",
			ChangeID:                   change.ID,
			EnvironmentStateRevisionID: base.ID,
		},
	); err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	if _, err := models.ChangeStateRevision.Create(
		ctx,
		db,
		models.CreateChangeStateRevisionData{
			Role:                       "result",
			ChangeID:                   change.ID,
			EnvironmentStateRevisionID: revision.ID,
		},
	); err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	if err := models.EnvironmentTargetState.SetDesiredRevisionForEnvironment(
		ctx,
		db,
		environment.ID,
		revision.ID,
		now,
	); err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	return revision, nil
}

func (service *EnvironmentSecrets) queueRevisionDeployment(
	ctx context.Context,
	db storage.Executor,
	change models.ChangeEntity,
	revision models.EnvironmentStateRevisionEntity,
) error {
	release, err := models.Release.LatestForEnvironment(ctx, db, revision.EnvironmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := models.ChangeRelease.Create(
		ctx,
		db,
		models.CreateChangeReleaseData{ChangeID: change.ID, ReleaseID: release.ID},
	); err != nil {
		return err
	}
	tx, ok := db.(bun.Tx)
	if !ok {
		return errors.New("secret revision deployment requires a database transaction")
	}
	_, err = service.releases.OrchestrateTx(ctx, tx, release, change, revision)
	return err
}
