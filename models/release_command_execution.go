package models

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type ReleaseCommandConfiguration struct {
	SchemaVersion  int      `json:"schema_version"`
	Name           string   `json:"name"`
	Command        string   `json:"command"`
	Arguments      []string `json:"arguments"`
	TimeoutSeconds int32    `json:"timeout_seconds"`
}

type ReleaseCommandExecutionEntity struct {
	bun.BaseModel              `bun:"table:release_command_executions,alias:release_command_executions"`
	ID                         uuid.UUID       `bun:"id,pk,type:uuid"                                                   json:"id"`
	CreatedAt                  time.Time       `bun:"created_at"                                                        json:"createdAt"`
	UpdatedAt                  time.Time       `bun:"updated_at"                                                        json:"updatedAt"`
	Status                     string          `bun:"status"                                                            json:"status"`
	Attempt                    int32           `bun:"attempt"                                                           json:"attempt"`
	Configuration              json.RawMessage `bun:"configuration,type:jsonb"                                          json:"-"`
	ConfigurationDigest        []byte          `bun:"configuration_digest"                                              json:"-"`
	ExternalID                 sql.NullString  `bun:"external_id"                                                       json:"externalId"`
	ExitCode                   sql.NullInt32   `bun:"exit_code"                                                         json:"exitCode"`
	StartedAt                  sql.NullTime    `bun:"started_at"                                                        json:"startedAt"`
	FinishedAt                 sql.NullTime    `bun:"finished_at"                                                       json:"finishedAt"`
	Error                      sql.NullString  `bun:"error"                                                             json:"error"`
	RetryRequestedBy           *uuid.UUID      `bun:"retry_requested_by,type:uuid"                                      json:"-"`
	ReleaseID                  uuid.UUID       `bun:"release_id,type:uuid"                                              json:"releaseId"`
	EnvironmentStateRevisionID uuid.UUID       `bun:"environment_state_revision_id,type:uuid"                           json:"-"`
	EnvironmentTargetID        uuid.UUID       `bun:"environment_target_id,type:uuid"                                   json:"environmentTargetId"`
	ChangeID                   uuid.UUID       `bun:"change_id,type:uuid"                                               json:"-"`
}

func CanonicalReleaseCommandConfiguration(
	process EnvironmentProcessState,
) (json.RawMessage, []byte, error) {
	configuration := ReleaseCommandConfiguration{
		SchemaVersion:  1,
		Name:           process.Name,
		Arguments:      process.Arguments,
		TimeoutSeconds: process.TimeoutSeconds,
	}
	if process.Command != nil {
		configuration.Command = *process.Command
	}
	input := EnvironmentProcessInput{
		Name:           configuration.Name,
		Kind:           EnvironmentProcessRelease,
		Command:        &configuration.Command,
		Arguments:      configuration.Arguments,
		Replicas:       1,
		TimeoutSeconds: &configuration.TimeoutSeconds,
	}
	if _, err := ValidateEnvironmentProcessFormation(
		[]EnvironmentProcessInput{input, defaultCompanionForValidation(EnvironmentProcessRelease)},
	); err != nil {
		return nil, nil, err
	}
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return nil, nil, err
	}
	digest := sha256.Sum256(encoded)
	return encoded, digest[:], nil
}

func ParseReleaseCommandConfiguration(raw json.RawMessage) (ReleaseCommandConfiguration, error) {
	var configuration ReleaseCommandConfiguration
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&configuration) != nil || configuration.SchemaVersion != 1 {
		return ReleaseCommandConfiguration{}, errors.New("release command configuration is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ReleaseCommandConfiguration{}, errors.New("release command configuration is invalid")
	}
	process := EnvironmentProcessState{
		Name:           configuration.Name,
		Kind:           EnvironmentProcessRelease,
		Command:        &configuration.Command,
		Arguments:      configuration.Arguments,
		Replicas:       1,
		TimeoutSeconds: configuration.TimeoutSeconds,
	}
	_, _, err := CanonicalReleaseCommandConfiguration(process)
	return configuration, err
}

func ParseReleaseCommandConfigurationSnapshot(
	raw json.RawMessage,
	digest []byte,
) (ReleaseCommandConfiguration, error) {
	configuration, err := ParseReleaseCommandConfiguration(raw)
	if err != nil {
		return ReleaseCommandConfiguration{}, err
	}
	process := EnvironmentProcessState{
		Name:           configuration.Name,
		Kind:           EnvironmentProcessRelease,
		Command:        &configuration.Command,
		Arguments:      configuration.Arguments,
		Replicas:       1,
		TimeoutSeconds: configuration.TimeoutSeconds,
	}
	_, expectedDigest, err := CanonicalReleaseCommandConfiguration(process)
	if err != nil || !bytes.Equal(expectedDigest, digest) {
		return ReleaseCommandConfiguration{}, errors.New(
			"release command configuration digest is invalid",
		)
	}
	return configuration, nil
}

func (entity *ReleaseCommandExecutionEntity) Validate() error {
	builder := validation.NewBuilder()
	if entity.ID == uuid.Nil || entity.ReleaseID == uuid.Nil ||
		entity.EnvironmentStateRevisionID == uuid.Nil ||
		entity.EnvironmentTargetID == uuid.Nil ||
		entity.ChangeID == uuid.Nil {
		builder.Add("id", "required", "release command ownership identifiers are required")
	}
	if !slices.Contains(
		[]string{"queued", "running", "succeeded", "failed", "ambiguous"},
		entity.Status,
	) ||
		entity.Attempt < 1 {
		builder.Add("status", "invalid", "release command execution state is invalid")
	}
	if _, err := ParseReleaseCommandConfigurationSnapshot(
		entity.Configuration,
		entity.ConfigurationDigest,
	); err != nil {
		builder.Add("configuration", "invalid", "release command configuration snapshot is invalid")
	}
	return builder.Err()
}

type CreateReleaseCommandExecutionData struct {
	Configuration              json.RawMessage
	ConfigurationDigest        []byte
	ReleaseID                  uuid.UUID
	EnvironmentStateRevisionID uuid.UUID
	EnvironmentTargetID        uuid.UUID
	ChangeID                   uuid.UUID
}

func (releaseCommandExecution) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateReleaseCommandExecutionData,
) (ReleaseCommandExecutionEntity, error) {
	switch db.(type) {
	case bun.Tx, *bun.Tx:
	default:
		return ReleaseCommandExecutionEntity{}, errors.New(
			"release command execution creation requires a transaction",
		)
	}
	if _, err := db.ExecContext(
		ctx,
		"SELECT pg_advisory_xact_lock(hashtextextended(?, 0))",
		"release-command:"+data.ReleaseID.String(),
	); err != nil {
		return ReleaseCommandExecutionEntity{}, err
	}
	count, err := db.NewSelect().
		Model((*ReleaseCommandExecutionEntity)(nil)).
		Where("release_id = ?", data.ReleaseID).
		Count(ctx)
	if err != nil {
		return ReleaseCommandExecutionEntity{}, err
	}
	if count != 0 {
		return ReleaseCommandExecutionEntity{}, errors.Join(
			ErrDomainValidation,
			errors.New("Release already has a release command execution"),
		)
	}
	now := time.Now().UTC()
	entity := ReleaseCommandExecutionEntity{
		ID:                         uuid.New(),
		CreatedAt:                  now,
		UpdatedAt:                  now,
		Status:                     "queued",
		Attempt:                    1,
		Configuration:              data.Configuration,
		ConfigurationDigest:        data.ConfigurationDigest,
		ReleaseID:                  data.ReleaseID,
		EnvironmentStateRevisionID: data.EnvironmentStateRevisionID,
		EnvironmentTargetID:        data.EnvironmentTargetID,
		ChangeID:                   data.ChangeID,
	}
	if err := validation.Validate(&entity); err != nil {
		return ReleaseCommandExecutionEntity{}, errors.Join(ErrDomainValidation, err)
	}
	_, err = db.NewInsert().Model(&entity).Exec(ctx)
	return entity, err
}

func (releaseCommandExecution) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ReleaseCommandExecutionEntity, error) {
	var entity ReleaseCommandExecutionEntity
	err := db.NewSelect().Model(&entity).Where("id = ?", id).Scan(ctx)
	return entity, err
}

func (releaseCommandExecution) SetExternalID(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	externalID string,
	at time.Time,
) error {
	_, err := db.NewUpdate().
		TableExpr("release_command_executions").
		Set("external_id = ?", externalID).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Where("status = 'running'").
		Exec(ctx)
	return err
}

func (releaseCommandExecution) SetEnvironmentTarget(
	ctx context.Context,
	db storage.Executor,
	id, environmentTargetID uuid.UUID,
) error {
	_, err := db.NewUpdate().
		TableExpr("release_command_executions").
		Set("environment_target_id = ?", environmentTargetID).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (releaseCommandExecution) MarkSucceeded(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	externalID string,
	exitCode int32,
	at time.Time,
) error {
	_, err := db.NewUpdate().
		TableExpr("release_command_executions").
		Set("status = 'succeeded'").
		Set("external_id = ?", externalID).
		Set("exit_code = ?", exitCode).
		Set("finished_at = ?", at).
		Set("error = NULL").
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Where("status = 'running'").
		Exec(ctx)
	return err
}

func (releaseCommandExecution) Lock(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (ReleaseCommandExecutionEntity, error) {
	var entity ReleaseCommandExecutionEntity
	err := db.NewSelect().Model(&entity).Where("id = ?", id).For("UPDATE").Scan(ctx)
	return entity, err
}

func (releaseCommandExecution) ForRelease(
	ctx context.Context,
	db storage.Executor,
	releaseID uuid.UUID,
) (ReleaseCommandExecutionEntity, error) {
	var entity ReleaseCommandExecutionEntity
	err := db.NewSelect().Model(&entity).Where("release_id = ?", releaseID).Limit(1).Scan(ctx)
	return entity, err
}

func (releaseCommandExecution) LatestForEnvironment(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
) (ReleaseCommandExecutionEntity, error) {
	var entity ReleaseCommandExecutionEntity
	err := db.NewSelect().
		Model(&entity).
		Join("JOIN releases AS release ON release.id = release_command_executions.release_id").
		Where("release.environment_id = ?", environmentID).
		OrderExpr("release_command_executions.created_at DESC").
		Limit(1).
		Scan(ctx)
	return entity, err
}

func (releaseCommandExecution) Running(
	ctx context.Context,
	db storage.Executor,
) ([]ReleaseCommandExecutionEntity, error) {
	items := make([]ReleaseCommandExecutionEntity, 0)
	err := db.NewSelect().Model(&items).
		Where("status = 'running'").OrderExpr("created_at").Scan(ctx)
	return items, err
}

func (releaseCommandExecution) MarkRunning(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	at time.Time,
) error {
	_, err := db.NewUpdate().
		TableExpr("release_command_executions").
		Set("status = 'running'").
		Set("started_at = ?", at).
		Set("finished_at = NULL").
		Set("error = NULL").
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Where("status = 'queued'").
		Exec(ctx)
	return err
}

func boundedReleaseCommandError(operationErr error) string {
	message := strings.TrimSpace(operationErr.Error())
	if len(message) > 2048 {
		message = message[:2048]
	}
	return message
}

func (releaseCommandExecution) MarkFailed(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	status string,
	exitCode *int32,
	operationErr error,
	at time.Time,
) error {
	if status != "failed" && status != "ambiguous" {
		return errors.New("release command failure status is invalid")
	}
	query := db.NewUpdate().
		TableExpr("release_command_executions").
		Set("status = ?", status).
		Set("finished_at = ?", at).
		Set("error = ?", boundedReleaseCommandError(operationErr)).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Where("status IN ('queued', 'running')")
	if exitCode != nil {
		query = query.Set("exit_code = ?", *exitCode)
	}
	_, err := query.Exec(ctx)
	return err
}

func (releaseCommandExecution) ResetForRetry(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	actorID *uuid.UUID,
	at time.Time,
) error {
	result, err := db.NewUpdate().
		TableExpr("release_command_executions").
		Set("status = 'queued'").
		Set("attempt = attempt + 1").
		Set("external_id = NULL").
		Set("exit_code = NULL").
		Set("started_at = NULL").
		Set("finished_at = NULL").
		Set("error = NULL").
		Set("retry_requested_by = ?", actorID).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Where("status IN ('failed', 'ambiguous')").
		Exec(ctx)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return errors.New("release command is not retryable")
	}
	return nil
}
