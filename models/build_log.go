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

const MaxBuildLogMessageBytes = 65536

type BuildLogEntity struct {
	bun.BaseModel `bun:"table:build_logs,alias:build_logs"`
	ID            uuid.UUID `bun:"id,pk,type:uuid"                   json:"id"`
	CreatedAt     time.Time `bun:"created_at"                        json:"createdAt"`
	UpdatedAt     time.Time `bun:"updated_at"                        json:"-"`
	Sequence      int64     `bun:"sequence"                          json:"sequence"`
	Stream        string    `bun:"stream"                            json:"stream"`
	Message       string    `bun:"message"                           json:"message"`
	OccurredAt    time.Time `bun:"occurred_at"                       json:"occurredAt"`
	BuildID       uuid.UUID `bun:"build_id,type:uuid"                json:"-"`
}

func (entity *BuildLogEntity) Validate() error {
	builder := validation.NewBuilder()
	if entity.ID == uuid.Nil || entity.BuildID == uuid.Nil {
		builder.Add("id", "required", "Build log ownership identifiers are required")
	}
	if entity.Sequence < 1 {
		builder.Add("sequence", "invalid", "Build log sequence must be positive")
	}
	if !slices.Contains([]string{"system", "pack"}, entity.Stream) {
		builder.Add("stream", "invalid", "Build log stream is invalid")
	}
	if strings.TrimSpace(entity.Message) == "" || len(entity.Message) > MaxBuildLogMessageBytes ||
		!utf8.ValidString(entity.Message) {
		builder.Add("message", "invalid", "Build log message is invalid")
	}
	if entity.OccurredAt.IsZero() {
		builder.Add("occurredAt", "required", "Build log occurrence time is required")
	}
	return builder.Err()
}

type CreateBuildLogData struct {
	Sequence   int64
	Stream     string
	Message    string
	OccurredAt time.Time
	BuildID    uuid.UUID
}

func (buildLog) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateBuildLogData,
) (BuildLogEntity, error) {
	now := time.Now().UTC()
	entity := BuildLogEntity{
		ID:         uuid.New(),
		CreatedAt:  now,
		UpdatedAt:  now,
		Sequence:   data.Sequence,
		Stream:     data.Stream,
		Message:    data.Message,
		OccurredAt: data.OccurredAt,
		BuildID:    data.BuildID,
	}
	if err := validation.Validate(&entity); err != nil {
		return BuildLogEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return BuildLogEntity{}, err
	}
	return entity, nil
}

func (buildLog) NextSequence(
	ctx context.Context,
	db storage.Executor,
	buildID uuid.UUID,
) (int64, error) {
	var sequence int64
	err := db.NewSelect().TableExpr("build_logs").ColumnExpr("COALESCE(MAX(sequence), 0) + 1").
		Where("build_id = ?", buildID).Scan(ctx, &sequence)
	return sequence, err
}

func (buildLog) PackBytes(
	ctx context.Context,
	db storage.Executor,
	buildID uuid.UUID,
) (int64, error) {
	var total int64
	err := db.NewSelect().
		TableExpr("build_logs").
		ColumnExpr("COALESCE(SUM(octet_length(message)), 0)").
		Where("build_id = ?", buildID).
		Where("stream = 'pack'").
		Scan(ctx, &total)
	return total, err
}

func (buildLog) ForBuildAfter(
	ctx context.Context,
	db storage.Executor,
	buildID uuid.UUID,
	after int64,
	limit int,
) ([]BuildLogEntity, error) {
	if limit < 1 {
		limit = 200
	}
	if limit > 501 {
		limit = 501
	}
	logs := make([]BuildLogEntity, 0, limit)
	err := db.NewSelect().Model(&logs).Where("build_id = ?", buildID).Where("sequence > ?", after).
		OrderExpr("sequence ASC").Limit(limit).Scan(ctx)
	return logs, err
}
