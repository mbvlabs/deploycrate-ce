package factories

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type EnvironmentProcessFactory struct {
	models.EnvironmentProcessEntity
}
type EnvironmentProcessOption func(*EnvironmentProcessFactory)

func BuildEnvironmentProcess(
	environmentID uuid.UUID,
	opts ...EnvironmentProcessOption,
) models.EnvironmentProcessEntity {
	f := &EnvironmentProcessFactory{EnvironmentProcessEntity: models.EnvironmentProcessEntity{
		Name: "web", Kind: "web", Arguments: json.RawMessage(`[]`), Replicas: 1,
		ContainerPort: sql.NullInt32{Int32: 8080, Valid: true}, EnvironmentID: environmentID,
	}}
	for _, opt := range opts {
		opt(f)
	}
	return f.EnvironmentProcessEntity
}

func CreateEnvironmentProcess(
	ctx context.Context,
	exec storage.Executor,
	environmentID uuid.UUID,
	opts ...EnvironmentProcessOption,
) (models.EnvironmentProcessEntity, error) {
	entity := BuildEnvironmentProcess(environmentID, opts...)
	entity.ID, entity.CreatedAt, entity.UpdatedAt = uuid.New(), time.Now(), time.Now()
	if err := exec.NewInsert().Model(&entity).Returning("*").Scan(ctx); err != nil {
		return models.EnvironmentProcessEntity{}, err
	}
	return entity, nil
}

func WithEnvironmentProcessesName(value string) EnvironmentProcessOption {
	return func(f *EnvironmentProcessFactory) { f.Name = value }
}
func WithEnvironmentProcessesKind(value string) EnvironmentProcessOption {
	return func(f *EnvironmentProcessFactory) { f.Kind = value }
}
func WithEnvironmentProcessesCommand(value sql.NullString) EnvironmentProcessOption {
	return func(f *EnvironmentProcessFactory) { f.Command = value }
}
func WithEnvironmentProcessesArguments(value json.RawMessage) EnvironmentProcessOption {
	return func(f *EnvironmentProcessFactory) { f.Arguments = value }
}
func WithEnvironmentProcessesReplicas(value int32) EnvironmentProcessOption {
	return func(f *EnvironmentProcessFactory) { f.Replicas = value }
}
