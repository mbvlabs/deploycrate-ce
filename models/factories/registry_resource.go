package factories

import (
	"context"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"encoding/json"

	"github.com/google/uuid"
)

func BuildRegistryResource(resourceID uuid.UUID) models.RegistryResourceEntity {
	return models.RegistryResourceEntity{
		ResourceID:    resourceID,
		Provider:      "distribution",
		Configuration: json.RawMessage(`{"schema_version":1}`),
	}
}

func CreateRegistryResource(
	ctx context.Context,
	exec storage.Executor,
	resourceID uuid.UUID,
) (models.RegistryResourceEntity, error) {
	built := BuildRegistryResource(resourceID)
	return models.RegistryResource.Create(
		ctx,
		exec,
		models.CreateRegistryResourceData{
			ResourceID:    built.ResourceID,
			Provider:      built.Provider,
			Configuration: built.Configuration,
		},
	)
}
