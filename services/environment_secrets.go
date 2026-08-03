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
	"deploycrate-ce/queue/jobs"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	db     storage.Pool
	queue  storage.InsertQueue
	config config.Config
}

func NewEnvironmentSecrets(db storage.Pool, queue storage.InsertQueue, cfg config.Config) *EnvironmentSecrets {
	return &EnvironmentSecrets{db: db, queue: queue, config: cfg}
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
		return PreparedEnvironmentSecret{}, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: "value", Code: "required", Message: "secret value is required"}})
	}
	if len(value) > models.EnvironmentSecretValueMaxBytes {
		return PreparedEnvironmentSecret{}, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: "value", Code: "max_length", Message: "secret value must not exceed 65536 bytes"}})
	}
	if environmentID == uuid.Nil || sourceID == uuid.Nil {
		return PreparedEnvironmentSecret{}, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: "sourceId", Code: "required", Message: "Environment and secret owner are required"}})
	}
	if sourceType != models.EnvironmentSecretSourceUser && sourceType != models.EnvironmentSecretSourceResource {
		return PreparedEnvironmentSecret{}, errors.Join(models.ErrDomainValidation, validation.ValidationErrors{{Field: "sourceType", Code: "unsupported", Message: "secret source type is unsupported"}})
	}
	encrypted, err := secretcrypto.EncryptForPurpose([]byte(value), service.config.App.SessionEncryptionKey, environmentSecretEncryptionPurpose)
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
		Key: prepared.Key, EncValue: prepared.EncValue, Digest: prepared.Digest,
		SourceType: prepared.SourceType, SourceID: prepared.SourceID, EnvironmentID: prepared.EnvironmentID,
	})
}

func (service *EnvironmentSecrets) CreateUser(
	ctx context.Context,
	applicationID, environmentID, userID uuid.UUID,
	key, value string,
) (EnvironmentSecretMutation, error) {
	prepared, err := service.Prepare(environmentID, key, value, models.EnvironmentSecretSourceUser, userID)
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
	if environment.ApplicationID != applicationID || !setupComplete || environment.ArchivedAt.Valid {
		return EnvironmentSecretMutation{}, errors.Join(models.ErrDomainValidation, errors.New("Environment setup must be complete"))
	}
	secret, err := service.CreatePrepared(ctx, tx, prepared)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	revision, err := service.commitSecretRevision(ctx, tx, environment, userID, "secret_create", nil, &secret)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	if err := tx.Commit(); err != nil {
		return EnvironmentSecretMutation{}, err
	}
	return EnvironmentSecretMutation{Secret: secret.Sanitized(), Revision: revision.ID}, nil
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
	if environment.ApplicationID != applicationID || !setupComplete || environment.ArchivedAt.Valid {
		return EnvironmentSecretMutation{}, errors.Join(models.ErrDomainValidation, errors.New("Environment setup must be complete"))
	}
	current, err := models.EnvironmentSecret.FindForEnvironment(ctx, tx, environmentID, secretID)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	if current.ArchivedAt.Valid || current.SourceType != models.EnvironmentSecretSourceUser {
		return EnvironmentSecretMutation{}, errors.Join(models.ErrDomainValidation, errors.New("only an active user-owned secret can be rotated"))
	}
	prepared, err := service.Prepare(environmentID, current.Key, value, models.EnvironmentSecretSourceUser, userID)
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
	revision, err := service.commitSecretRevision(ctx, tx, environment, userID, "secret_rotate", &current, &next)
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
	if environment.ApplicationID != applicationID || !setupComplete || environment.ArchivedAt.Valid {
		return EnvironmentSecretMutation{}, errors.Join(models.ErrDomainValidation, errors.New("Environment setup must be complete"))
	}
	current, err := models.EnvironmentSecret.FindForEnvironment(ctx, tx, environmentID, secretID)
	if err != nil {
		return EnvironmentSecretMutation{}, err
	}
	if current.ArchivedAt.Valid || current.SourceType != models.EnvironmentSecretSourceUser {
		return EnvironmentSecretMutation{}, errors.Join(models.ErrDomainValidation, errors.New("only an active user-owned secret can be archived"))
	}
	if err := models.EnvironmentSecret.Archive(ctx, tx, environmentID, current.ID); err != nil {
		return EnvironmentSecretMutation{}, err
	}
	revision, err := service.commitSecretRevision(ctx, tx, environment, userID, "secret_archive", &current, nil)
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
	secrets, err := models.EnvironmentStateRevision.ResolveSecrets(ctx, service.db.Executor(), revision)
	if err != nil {
		return nil, err
	}
	resolved := make([]ResolvedEnvironmentSecret, 0, len(secrets))
	for _, secret := range secrets {
		plaintext, err := secretcrypto.DecryptForPurpose(secret.EncValue, service.config.App.SessionEncryptionKey, environmentSecretEncryptionPurpose)
		if err != nil {
			return nil, fmt.Errorf("decrypt Environment secret %s: %w", secret.Key, err)
		}
		resolved = append(resolved, ResolvedEnvironmentSecret{Key: secret.Key, Value: string(plaintext)})
	}
	return resolved, nil
}

func (service *EnvironmentSecrets) RotateManagedResource(
	ctx context.Context,
	db storage.Executor,
	connection models.EnvironmentResourceEntity,
	values map[string]string,
) error {
	if connection.ArchivedAt.Valid || len(values) == 0 {
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
	replacements := make(map[uuid.UUID]models.EnvironmentSecretEntity)
	previous := make(map[uuid.UUID]models.EnvironmentSecretEntity)
	for _, descriptor := range state.Secrets {
		value, requested := values[descriptor.Key]
		if !requested || descriptor.SourceType != models.EnvironmentSecretSourceResource || descriptor.SourceID != connection.ID {
			continue
		}
		current, findErr := models.EnvironmentSecret.FindForEnvironment(ctx, db, environment.ID, descriptor.ID)
		if findErr != nil || current.ArchivedAt.Valid {
			return errors.New("active Resource-managed secret value is unavailable")
		}
		prepared, prepareErr := service.Prepare(environment.ID, descriptor.Key, value, models.EnvironmentSecretSourceResource, connection.ID)
		if prepareErr != nil {
			return prepareErr
		}
		if hmac.Equal(current.Digest, prepared.Digest) {
			continue
		}
		if err := models.EnvironmentSecret.Archive(ctx, db, environment.ID, current.ID); err != nil {
			return err
		}
		next, createErr := service.CreatePrepared(ctx, db, prepared)
		if createErr != nil {
			return createErr
		}
		previous[current.ID] = current
		replacements[current.ID] = next
	}
	if len(replacements) == 0 {
		return nil
	}
	for index, descriptor := range state.Secrets {
		if replacement, exists := replacements[descriptor.ID]; exists {
			state.Secrets[index] = models.EnvironmentSecretDescriptorFromEntity(replacement)
		}
	}
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
		Sequence: sequence, Kind: "resource_secret_rotation", TriggerType: "system", ActorType: "system",
		CauseSystem: sql.NullString{String: "resource", Valid: true}, CauseReference: sql.NullString{String: connection.ResourceID.String(), Valid: true},
		CorrelationID: uuid.New(), Summary: "Rotate Resource-managed Environment secrets", Status: "committed",
		RequestedAt: now, CommittedAt: sql.NullTime{Time: now, Valid: true}, EnvironmentID: environment.ID,
	})
	if err != nil {
		return err
	}
	for oldID, replacement := range replacements {
		oldValue, _ := json.Marshal(models.EnvironmentSecretDescriptorFromEntity(previous[oldID]))
		newValue, _ := json.Marshal(models.EnvironmentSecretDescriptorFromEntity(replacement))
		if _, err := models.ChangeItem.Create(ctx, db, models.CreateChangeItemData{
			Action: "rotate", SubjectType: "environment_secret", SubjectID: replacement.ID,
			PreviousValue: oldValue, RequestedValue: newValue, ChangeID: change.ID,
		}); err != nil {
			return err
		}
	}
	revision, err := models.EnvironmentStateRevision.Create(ctx, db, models.CreateEnvironmentStateRevisionData{State: canonical, EnvironmentID: environment.ID, ChangeID: change.ID})
	if err != nil {
		return err
	}
	if _, err := models.ChangeStateRevision.Create(ctx, db, models.CreateChangeStateRevisionData{Role: "base", ChangeID: change.ID, EnvironmentStateRevisionID: base.ID}); err != nil {
		return err
	}
	if _, err := models.ChangeStateRevision.Create(ctx, db, models.CreateChangeStateRevisionData{Role: "result", ChangeID: change.ID, EnvironmentStateRevisionID: revision.ID}); err != nil {
		return err
	}
	if _, err := db.NewUpdate().TableExpr("environment_target_states AS state").Set("desired_revision_id = ?", revision.ID).Set("state = 'pending'").Set("updated_at = ?", now).
		Where("EXISTS (SELECT 1 FROM environment_targets target WHERE target.id = state.environment_target_id AND target.environment_id = ? AND target.detached_at IS NULL)", environment.ID).Exec(ctx); err != nil {
		return err
	}
	return service.queueRevisionDeployment(ctx, db, change, revision, state)
}

func (service *EnvironmentSecrets) digest(environmentID uuid.UUID, key, value string) ([]byte, error) {
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

func (service *EnvironmentSecrets) commitSecretRevision(
	ctx context.Context,
	db storage.Executor,
	environment models.EnvironmentEntity,
	actorID uuid.UUID,
	kind string,
	previous, requested *models.EnvironmentSecretEntity,
) (models.EnvironmentStateRevisionEntity, error) {
	if _, err := models.Environment.Lock(ctx, db, environment.ID); err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	base, err := models.EnvironmentStateRevision.LatestCommitted(ctx, db, environment.ID)
	if err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	state, err := models.ParseEnvironmentDesiredState(base.State)
	if err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	secrets := make([]models.EnvironmentSecretDescriptor, 0, len(state.Secrets)+1)
	for _, descriptor := range state.Secrets {
		if previous != nil && descriptor.ID == previous.ID {
			continue
		}
		secrets = append(secrets, descriptor)
	}
	if requested != nil {
		secrets = append(secrets, models.EnvironmentSecretDescriptorFromEntity(*requested))
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
		Sequence: sequence, Kind: kind, TriggerType: "user", ActorType: "user", ActorID: &actorID,
		CorrelationID: uuid.New(), Summary: strings.ReplaceAll(kind, "_", " "), Status: "committed",
		RequestedAt: now, CommittedAt: sql.NullTime{Time: now, Valid: true}, EnvironmentID: environment.ID,
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
	subjectID := uuid.Nil
	if requested != nil {
		subjectID = requested.ID
	} else if previous != nil {
		subjectID = previous.ID
	}
	if _, err := models.ChangeItem.Create(ctx, db, models.CreateChangeItemData{
		Action: kind, SubjectType: "environment_secret", SubjectID: subjectID,
		PreviousValue: changeValue(previous), RequestedValue: changeValue(requested), ChangeID: change.ID,
	}); err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	revision, err := models.EnvironmentStateRevision.Create(ctx, db, models.CreateEnvironmentStateRevisionData{
		State: canonical, EnvironmentID: environment.ID, ChangeID: change.ID,
	})
	if err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	if _, err := models.ChangeStateRevision.Create(ctx, db, models.CreateChangeStateRevisionData{Role: "base", ChangeID: change.ID, EnvironmentStateRevisionID: base.ID}); err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	if _, err := models.ChangeStateRevision.Create(ctx, db, models.CreateChangeStateRevisionData{Role: "result", ChangeID: change.ID, EnvironmentStateRevisionID: revision.ID}); err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	if _, err := db.NewUpdate().TableExpr("environment_target_states AS state").
		Set("desired_revision_id = ?", revision.ID).Set("state = 'pending'").Set("updated_at = ?", now).
		Where("EXISTS (SELECT 1 FROM environment_targets target WHERE target.id = state.environment_target_id AND target.environment_id = ? AND target.detached_at IS NULL)", environment.ID).
		Exec(ctx); err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	return revision, nil
}

func (service *EnvironmentSecrets) queueRevisionDeployment(
	ctx context.Context,
	db storage.Executor,
	change models.ChangeEntity,
	revision models.EnvironmentStateRevisionEntity,
	state models.EnvironmentDesiredState,
) error {
	var release models.ReleaseEntity
	err := db.NewSelect().Model(&release).Where("environment_id = ?", revision.EnvironmentID).
		Where("build_id IS NOT NULL").OrderExpr("created_at DESC").Limit(1).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	targets, err := models.EnvironmentTarget.ActiveForEnvironmentAll(ctx, db, revision.EnvironmentID)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("Environment has no runtime Server targets")
	}
	if _, err := models.ChangeRelease.Create(ctx, db, models.CreateChangeReleaseData{ChangeID: change.ID, ReleaseID: release.ID}); err != nil {
		return err
	}
	runtimeSnapshot, _ := json.Marshal(state.Runtime)
	tx, ok := db.(bun.Tx)
	if !ok {
		return errors.New("secret revision deployment requires a database transaction")
	}
	for _, target := range targets {
		deployment, createErr := models.Deployment.Create(ctx, db, models.CreateDeploymentData{
			Attempt: 1, Strategy: json.RawMessage(`{"type":"blue_green","replicas":1}`), RuntimeConfiguration: runtimeSnapshot,
			Status: "queued", CurrentStep: sql.NullString{String: "queued", Valid: true}, ChangeID: change.ID,
			ReleaseID: release.ID, EnvironmentTargetID: target.ID,
		})
		if createErr != nil {
			return createErr
		}
		if _, createErr := models.Instance.Create(ctx, db, models.CreateInstanceData{
			ExternalID: "pending:" + deployment.ID.String(), Slot: "candidate", ReplicaKey: "primary", State: "candidate",
			Ports: json.RawMessage(`{}`), ObservedAt: time.Now().UTC(), DeploymentID: deployment.ID,
			ReleaseID: release.ID, EnvironmentTargetID: target.ID,
		}); createErr != nil {
			return createErr
		}
		if _, createErr := service.queue.InsertTx(ctx, tx.Tx, jobs.DeployReleaseArgs{DeploymentID: deployment.ID}, jobs.DeployReleaseInsertOpts(deployment.ID)); createErr != nil {
			return createErr
		}
	}
	return nil
}
