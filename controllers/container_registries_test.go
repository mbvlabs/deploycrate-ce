package controllers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"deploycrate-ce/database/seeds"
	"deploycrate-ce/models"
	"deploycrate-ce/models/factories"
	"deploycrate-ce/queue"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/labstack/echo/v5"
)

func TestRegistryResourcesControllerIndexUsesSeedData(t *testing.T) {
	db := newControllerTestDB(t, seeds.UI)
	configuration := controllerTestConfig(t)
	identity := services.NewIdentity(db, queue.InsertOnly{}, configuration)
	controller := NewRegistryResources(
		services.NewRegistryResources(db, configuration, identity),
	)

	page := requireControllerComponent(t, controllerRequest(
		t,
		http.MethodGet,
		routes.RegistryResources.URL(),
		nil,
		nil,
		controller.Index,
	), "Connections/Registries")
	var registries []json.RawMessage
	if err := json.Unmarshal(page.Props["registries"], &registries); err != nil {
		t.Fatalf("decode Registry props: %v", err)
	}
	if len(registries) != 1 {
		t.Fatalf("Registry count = %d, want 1", len(registries))
	}
}

func TestRegistryResourcesControllerCreateValidationDoesNotWriteDatabase(t *testing.T) {
	db := newControllerTestDB(t, nil)
	configuration := controllerTestConfig(t)
	controller := NewRegistryResources(services.NewRegistryResources(
		db,
		configuration,
		services.NewIdentity(db, queue.InsertOnly{}, configuration),
	))

	page := requireControllerComponent(t, controllerRequest(
		t,
		http.MethodPost,
		routes.RegistryResourceCreate.URL(),
		map[string]string{"name": "", "endpoint": "bad endpoint"},
		nil,
		controller.Create,
	), "Connections/Registries")
	var validationErrors map[string]string
	if err := json.Unmarshal(page.Props["errors"], &validationErrors); err != nil {
		t.Fatalf("decode Registry validation errors: %v", err)
	}
	if validationErrors["name"] == "" || validationErrors["endpoint"] == "" ||
		validationErrors["accessToken"] == "" {
		t.Fatalf("Registry validation errors = %v", validationErrors)
	}
	count, err := db.Executor().NewSelect().Model((*models.ResourceEntity)(nil)).Count(t.Context())
	if err != nil {
		t.Fatalf("count Resources after invalid Registry create: %v", err)
	}
	if count != 0 {
		t.Fatalf("Resource count after invalid Registry create = %d, want 0", count)
	}
}

func TestRegistryResourcesControllerDestroyArchivesExternalRegistry(t *testing.T) {
	db := newControllerTestDB(t, nil)
	configuration := controllerTestConfig(t)
	resource, err := factories.CreateResource(
		t.Context(),
		db.Executor(),
		factories.WithResourcesName("External Registry"),
		factories.WithResourcesSlug("external-registry"),
		factories.WithResourcesResourceType(models.ResourceTypeService),
		factories.WithResourcesConfiguration(json.RawMessage(`{"engine":"registry"}`)),
		factories.WithResourcesSystemManaged(false),
		factories.WithResourcesEnvironmentAttachable(false),
		factories.WithResourcesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		t.Fatalf("create Registry Resource fixture: %v", err)
	}
	if _, err := factories.CreateRegistryResource(t.Context(), db.Executor(), resource.ID); err != nil {
		t.Fatalf("create Registry backing fixture: %v", err)
	}
	controller := NewRegistryResources(services.NewRegistryResources(
		db,
		configuration,
		services.NewIdentity(db, queue.InsertOnly{}, configuration),
	))

	response := controllerRequest(
		t,
		http.MethodDelete,
		routes.RegistryResourceDestroy.URL(resource.ID),
		nil,
		echo.PathValues{{Name: "id", Value: resource.ID.String()}},
		controller.Destroy,
	)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("destroy status = %d, want %d; body: %s", response.Code, http.StatusSeeOther, response.Body)
	}
	archived, err := models.Resource.Find(t.Context(), db.Executor(), resource.ID)
	if err != nil {
		t.Fatalf("load archived Registry Resource: %v", err)
	}
	if !archived.ArchivedAt.Valid {
		t.Fatal("external Registry Resource was not archived")
	}
}
