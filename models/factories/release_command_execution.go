package factories

import (
	"context"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"time"

	"github.com/google/uuid"
)

type ReleaseCommandExecutionFactory struct {
	models.ReleaseCommandExecutionEntity
}
type ReleaseCommandExecutionOption func(*ReleaseCommandExecutionFactory)

func BuildReleaseCommandExecution(releaseID, revisionID, targetID, changeID uuid.UUID, opts ...ReleaseCommandExecutionOption) models.ReleaseCommandExecutionEntity {
	command := "/app/server"
	configuration, digest, _ := models.CanonicalReleaseCommandConfiguration(models.EnvironmentProcessState{Name: "release", Kind: "release", Command: &command, Arguments: []string{"migrate"}, Replicas: 1, TimeoutSeconds: 900})
	f := &ReleaseCommandExecutionFactory{ReleaseCommandExecutionEntity: models.ReleaseCommandExecutionEntity{
		Status: "queued", Attempt: 1, Configuration: configuration, ConfigurationDigest: digest,
		ReleaseID: releaseID, EnvironmentStateRevisionID: revisionID, EnvironmentTargetID: targetID, ChangeID: changeID,
	}}
	for _, opt := range opts {
		opt(f)
	}
	return f.ReleaseCommandExecutionEntity
}

func CreateReleaseCommandExecution(ctx context.Context, exec storage.Executor, releaseID, revisionID, targetID, changeID uuid.UUID, opts ...ReleaseCommandExecutionOption) (models.ReleaseCommandExecutionEntity, error) {
	entity := BuildReleaseCommandExecution(releaseID, revisionID, targetID, changeID, opts...)
	entity.ID, entity.CreatedAt, entity.UpdatedAt = uuid.New(), time.Now(), time.Now()
	if err := exec.NewInsert().Model(&entity).Returning("*").Scan(ctx); err != nil {
		return models.ReleaseCommandExecutionEntity{}, err
	}
	return entity, nil
}

func WithReleaseCommandExecutionsStatus(value string) ReleaseCommandExecutionOption {
	return func(f *ReleaseCommandExecutionFactory) { f.Status = value }
}
func WithReleaseCommandExecutionsAttempt(value int32) ReleaseCommandExecutionOption {
	return func(f *ReleaseCommandExecutionFactory) { f.Attempt = value }
}
