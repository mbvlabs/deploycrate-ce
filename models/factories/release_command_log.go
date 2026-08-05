package factories

import (
	"context"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"time"

	"github.com/google/uuid"
)

type ReleaseCommandLogFactory struct{ models.ReleaseCommandLogEntity }
type ReleaseCommandLogOption func(*ReleaseCommandLogFactory)

func BuildReleaseCommandLog(executionID uuid.UUID, opts ...ReleaseCommandLogOption) models.ReleaseCommandLogEntity {
	f := &ReleaseCommandLogFactory{ReleaseCommandLogEntity: models.ReleaseCommandLogEntity{Attempt: 1, Sequence: 1, Stream: "system", Message: "release command event", OccurredAt: time.Now(), ReleaseCommandExecutionID: executionID}}
	for _, opt := range opts {
		opt(f)
	}
	return f.ReleaseCommandLogEntity
}

func CreateReleaseCommandLog(ctx context.Context, exec storage.Executor, executionID uuid.UUID, opts ...ReleaseCommandLogOption) (models.ReleaseCommandLogEntity, error) {
	entity := BuildReleaseCommandLog(executionID, opts...)
	entity.ID, entity.CreatedAt, entity.UpdatedAt = uuid.New(), time.Now(), time.Now()
	if err := exec.NewInsert().Model(&entity).Returning("*").Scan(ctx); err != nil {
		return models.ReleaseCommandLogEntity{}, err
	}
	return entity, nil
}

func WithReleaseCommandLogsAttempt(value int32) ReleaseCommandLogOption {
	return func(f *ReleaseCommandLogFactory) { f.Attempt = value }
}
func WithReleaseCommandLogsStream(value string) ReleaseCommandLogOption {
	return func(f *ReleaseCommandLogFactory) { f.Stream = value }
}
func WithReleaseCommandLogsMessage(value string) ReleaseCommandLogOption {
	return func(f *ReleaseCommandLogFactory) { f.Message = value }
}
