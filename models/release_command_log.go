package models

import (
	"context"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const MaxReleaseCommandLogMessageBytes = 65536

type ReleaseCommandLogEntity struct {
	bun.BaseModel             `bun:"table:release_command_logs,alias:release_command_logs"`
	ID                        uuid.UUID `bun:"id,pk,type:uuid"                                       json:"id"`
	CreatedAt                 time.Time `bun:"created_at"                                            json:"createdAt"`
	UpdatedAt                 time.Time `bun:"updated_at"                                            json:"-"`
	Attempt                   int32     `bun:"attempt"                                               json:"attempt"`
	Sequence                  int64     `bun:"sequence"                                              json:"sequence"`
	Stream                    string    `bun:"stream"                                                json:"stream"`
	Message                   string    `bun:"message"                                               json:"message"`
	OccurredAt                time.Time `bun:"occurred_at"                                           json:"occurredAt"`
	ReleaseCommandExecutionID uuid.UUID `bun:"release_command_execution_id,type:uuid"                json:"-"`
}

func (entity *ReleaseCommandLogEntity) Validate() error {
	builder := validation.NewBuilder()
	if entity.ID == uuid.Nil || entity.ReleaseCommandExecutionID == uuid.Nil ||
		entity.Attempt < 1 ||
		entity.Sequence < 1 {
		builder.Add("id", "required", "release command log identity is invalid")
	}
	if !slices.Contains([]string{"system", "stdout", "stderr"}, entity.Stream) {
		builder.Add("stream", "invalid", "release command log stream is invalid")
	}
	if strings.TrimSpace(entity.Message) == "" ||
		len(entity.Message) > MaxReleaseCommandLogMessageBytes ||
		!utf8.ValidString(entity.Message) {
		builder.Add("message", "invalid", "release command log message is invalid")
	}
	return builder.Err()
}

func (releaseCommandLog) Create(
	ctx context.Context,
	db storage.Executor,
	executionID uuid.UUID,
	attempt int32,
	sequence int64,
	stream, message string,
) (ReleaseCommandLogEntity, error) {
	now := time.Now().UTC()
	entity := ReleaseCommandLogEntity{
		ID:                        uuid.New(),
		CreatedAt:                 now,
		UpdatedAt:                 now,
		Attempt:                   attempt,
		Sequence:                  sequence,
		Stream:                    stream,
		Message:                   message,
		OccurredAt:                now,
		ReleaseCommandExecutionID: executionID,
	}
	if err := validation.Validate(&entity); err != nil {
		return ReleaseCommandLogEntity{}, errors.Join(ErrDomainValidation, err)
	}
	_, err := db.NewInsert().Model(&entity).Exec(ctx)
	return entity, err
}

func (releaseCommandLog) NextSequence(
	ctx context.Context,
	db storage.Executor,
	executionID uuid.UUID,
	attempt int32,
) (int64, error) {
	var sequence int64
	err := db.NewSelect().
		TableExpr("release_command_logs").
		ColumnExpr("COALESCE(MAX(sequence), 0) + 1").
		Where("release_command_execution_id = ?", executionID).
		Where("attempt = ?", attempt).
		Scan(ctx, &sequence)
	return sequence, err
}

func (releaseCommandLog) ForExecution(
	ctx context.Context,
	db storage.Executor,
	executionID uuid.UUID,
) ([]ReleaseCommandLogEntity, error) {
	logs := make([]ReleaseCommandLogEntity, 0)
	err := db.NewSelect().
		Model(&logs).
		Where("release_command_execution_id = ?", executionID).
		OrderExpr("attempt, sequence").
		Limit(1000).
		Scan(ctx)
	return logs, err
}
