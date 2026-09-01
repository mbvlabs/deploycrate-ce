package controllers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	containerclient "deploycrate-ce/clients/container"
	"deploycrate-ce/database/seeds"
	"deploycrate-ce/models"
	"deploycrate-ce/models/factories"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type fakeResourceContainers struct {
	inspectCalls int
	stopCalls    int
	removeCalls  int
}

func TestResourcesControllerCredentialLifecycleUsesDatabase(t *testing.T) {
	db := newControllerTestDB(t, nil)
	resource, err := factories.CreateResource(
		t.Context(),
		db.Executor(),
		factories.WithResourcesName("Credential Cache"),
		factories.WithResourcesSlug("credential-cache"),
		factories.WithResourcesResourceType(models.ResourceTypeCache),
		factories.WithResourcesConfiguration(json.RawMessage(`{"engine":"redis"}`)),
		factories.WithResourcesSystemManaged(false),
		factories.WithResourcesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		t.Fatalf("create Resource fixture: %v", err)
	}
	service := services.NewResourceManagement(
		db,
		controllerTestConfig(t),
		nil,
		&fakeResourceContainers{},
	)
	controller := NewResources(
		service,
		services.NewResourcePrivateAccess(db, nil),
		nil,
		nil,
		nil,
		nil,
		services.NewCaddyRouteService(db, &fakeCaddyClient{}),
	)

	createResponse := controllerRequest(
		t,
		http.MethodPost,
		routes.ResourceCredentialCreate.URL(resource.ID),
		map[string]any{
			"name":         "Application password",
			"username":     "cache-user",
			"metadata":     json.RawMessage(`{"purpose":"application"}`),
			"secretValues": map[string]string{"password": "integration-secret"},
		},
		echo.PathValues{{Name: "id", Value: resource.ID.String()}},
		controller.CreateCredential,
	)
	if createResponse.Code != http.StatusSeeOther {
		t.Fatalf(
			"create credential status = %d, want %d; body: %s",
			createResponse.Code,
			http.StatusSeeOther,
			createResponse.Body,
		)
	}
	var credentials []models.ResourceCredentialEntity
	if err := db.Executor().NewSelect().
		Model(&credentials).
		Where("resource_id = ?", resource.ID).
		Scan(t.Context()); err != nil {
		t.Fatalf("load created Resource credential: %v", err)
	}
	if len(credentials) != 1 {
		t.Fatalf("Resource credential count = %d, want 1", len(credentials))
	}
	credential := credentials[0]
	if bytes.Contains(credential.EncPayload, []byte("integration-secret")) {
		t.Fatal("Resource credential payload contains the plaintext password")
	}

	params := routes.ResourceCredentialParams{
		ResourceID:   resource.ID.String(),
		CredentialID: credential.ID.String(),
	}
	updateResponse := controllerRequest(
		t,
		http.MethodPatch,
		routes.ResourceCredentialUpdate.URL(params),
		map[string]any{
			"name":     "Renamed password",
			"username": "renamed-user",
			"metadata": json.RawMessage(`{"purpose":"application","label":"renamed"}`),
			"rotate":   false,
		},
		echo.PathValues{
			{Name: "id", Value: resource.ID.String()},
			{Name: "credentialID", Value: credential.ID.String()},
		},
		controller.UpdateCredential,
	)
	if updateResponse.Code != http.StatusSeeOther {
		t.Fatalf(
			"update credential status = %d, want %d; body: %s",
			updateResponse.Code,
			http.StatusSeeOther,
			updateResponse.Body,
		)
	}
	updated, err := models.ResourceCredential.Find(t.Context(), db.Executor(), credential.ID)
	if err != nil {
		t.Fatalf("load updated Resource credential: %v", err)
	}
	if updated.Name != "Renamed password" || !updated.Username.Valid ||
		updated.Username.String != "renamed-user" {
		t.Fatalf("updated Resource credential = %+v", updated)
	}
	if !bytes.Equal(updated.EncPayload, credential.EncPayload) {
		t.Fatal("metadata-only update unexpectedly rotated encrypted credential payload")
	}

	destroyResponse := controllerRequest(
		t,
		http.MethodDelete,
		routes.ResourceCredentialDestroy.URL(params),
		nil,
		echo.PathValues{
			{Name: "id", Value: resource.ID.String()},
			{Name: "credentialID", Value: credential.ID.String()},
		},
		controller.DestroyCredential,
	)
	if destroyResponse.Code != http.StatusSeeOther {
		t.Fatalf(
			"destroy credential status = %d, want %d; body: %s",
			destroyResponse.Code,
			http.StatusSeeOther,
			destroyResponse.Body,
		)
	}
	archived, err := models.ResourceCredential.Find(t.Context(), db.Executor(), credential.ID)
	if err != nil {
		t.Fatalf("load archived Resource credential: %v", err)
	}
	if !archived.ArchivedAt.Valid {
		t.Fatal("Resource credential was not archived")
	}
}

func TestResourcesControllerEndpointAndHealthCheckLifecycleUsesDatabase(t *testing.T) {
	db := newControllerTestDB(t, nil)
	resource, err := factories.CreateResource(
		t.Context(),
		db.Executor(),
		factories.WithResourcesName("Health Cache"),
		factories.WithResourcesSlug("health-cache"),
		factories.WithResourcesResourceType(models.ResourceTypeCache),
		factories.WithResourcesConfiguration(json.RawMessage(`{"engine":"redis"}`)),
		factories.WithResourcesSystemManaged(false),
		factories.WithResourcesArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		t.Fatalf("create Resource fixture: %v", err)
	}
	caddyClient := &fakeCaddyClient{}
	controller := NewResources(
		services.NewResourceManagement(
			db,
			controllerTestConfig(t),
			nil,
			&fakeResourceContainers{},
		),
		services.NewResourcePrivateAccess(db, nil),
		nil,
		nil,
		nil,
		nil,
		services.NewCaddyRouteService(db, caddyClient),
	)

	createEndpointResponse := controllerRequest(
		t,
		http.MethodPost,
		routes.ResourceEndpointCreate.URL(resource.ID),
		map[string]any{
			"name":     "Primary Redis",
			"role":     "primary",
			"address":  "redis.internal",
			"port":     6379,
			"protocol": "redis",
			"tlsMode":  "disable",
			"settings": json.RawMessage(`{}`),
			"publication": map[string]any{
				"enabled": false,
			},
		},
		echo.PathValues{{Name: "id", Value: resource.ID.String()}},
		controller.CreateEndpoint,
	)
	if createEndpointResponse.Code != http.StatusSeeOther {
		t.Fatalf(
			"create endpoint status = %d, want %d; body: %s",
			createEndpointResponse.Code,
			http.StatusSeeOther,
			createEndpointResponse.Body,
		)
	}
	var endpoints []models.ResourceEndpointEntity
	if err := db.Executor().NewSelect().
		Model(&endpoints).
		Where("resource_id = ?", resource.ID).
		Scan(t.Context()); err != nil {
		t.Fatalf("load created Resource endpoint: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("Resource endpoint count = %d, want 1", len(endpoints))
	}
	endpoint := endpoints[0]
	endpointParams := routes.ResourceEndpointParams{
		ResourceID: resource.ID.String(),
		EndpointID: endpoint.ID.String(),
	}

	updateEndpointResponse := controllerRequest(
		t,
		http.MethodPatch,
		routes.ResourceEndpointUpdate.URL(endpointParams),
		map[string]any{
			"name":     "Renamed Redis",
			"role":     "primary",
			"address":  "renamed-redis.internal",
			"port":     6380,
			"protocol": "redis",
			"tlsMode":  "disable",
			"settings": json.RawMessage(`{}`),
			"publication": map[string]any{
				"enabled": false,
			},
		},
		echo.PathValues{
			{Name: "id", Value: resource.ID.String()},
			{Name: "endpointID", Value: endpoint.ID.String()},
		},
		controller.UpdateEndpoint,
	)
	if updateEndpointResponse.Code != http.StatusSeeOther {
		t.Fatalf(
			"update endpoint status = %d, want %d; body: %s",
			updateEndpointResponse.Code,
			http.StatusSeeOther,
			updateEndpointResponse.Body,
		)
	}
	updatedEndpoint, err := models.ResourceEndpoint.Find(t.Context(), db.Executor(), endpoint.ID)
	if err != nil {
		t.Fatalf("load updated Resource endpoint: %v", err)
	}
	if updatedEndpoint.Name != "Renamed Redis" ||
		updatedEndpoint.Address != "renamed-redis.internal" ||
		updatedEndpoint.Port != 6380 {
		t.Fatalf("updated Resource endpoint = %+v", updatedEndpoint)
	}

	createHealthResponse := controllerRequest(
		t,
		http.MethodPost,
		routes.ResourceHealthCheckCreate.URL(resource.ID),
		map[string]any{
			"name":               "Redis TCP",
			"kind":               "tcp",
			"configuration":      json.RawMessage(`{}`),
			"intervalSeconds":    15,
			"timeoutSeconds":     3,
			"failureThreshold":   3,
			"successThreshold":   1,
			"enabled":            true,
			"resourceEndpointId": endpoint.ID.String(),
		},
		echo.PathValues{{Name: "id", Value: resource.ID.String()}},
		controller.CreateHealthCheck,
	)
	if createHealthResponse.Code != http.StatusSeeOther {
		t.Fatalf(
			"create health check status = %d, want %d; body: %s",
			createHealthResponse.Code,
			http.StatusSeeOther,
			createHealthResponse.Body,
		)
	}
	var healthChecks []models.ResourceHealthCheckEntity
	if err := db.Executor().NewSelect().
		Model(&healthChecks).
		Where("resource_id = ?", resource.ID).
		Scan(t.Context()); err != nil {
		t.Fatalf("load created Resource health check: %v", err)
	}
	if len(healthChecks) != 1 {
		t.Fatalf("Resource health check count = %d, want 1", len(healthChecks))
	}
	healthCheck := healthChecks[0]
	healthParams := routes.ResourceHealthCheckParams{
		ResourceID:    resource.ID.String(),
		HealthCheckID: healthCheck.ID.String(),
	}
	updateHealthResponse := controllerRequest(
		t,
		http.MethodPatch,
		routes.ResourceHealthCheckUpdate.URL(healthParams),
		map[string]any{
			"name":               "Renamed Redis TCP",
			"kind":               "tcp",
			"configuration":      json.RawMessage(`{}`),
			"intervalSeconds":    30,
			"timeoutSeconds":     5,
			"failureThreshold":   4,
			"successThreshold":   2,
			"enabled":            false,
			"resourceEndpointId": endpoint.ID.String(),
		},
		echo.PathValues{
			{Name: "id", Value: resource.ID.String()},
			{Name: "healthCheckID", Value: healthCheck.ID.String()},
		},
		controller.UpdateHealthCheck,
	)
	if updateHealthResponse.Code != http.StatusSeeOther {
		t.Fatalf(
			"update health check status = %d, want %d; body: %s",
			updateHealthResponse.Code,
			http.StatusSeeOther,
			updateHealthResponse.Body,
		)
	}
	var updatedHealth models.ResourceHealthCheckEntity
	if err := db.Executor().NewSelect().
		Model(&updatedHealth).
		Where("id = ?", healthCheck.ID).
		Scan(t.Context()); err != nil {
		t.Fatalf("load updated Resource health check: %v", err)
	}
	if updatedHealth.Name != "Renamed Redis TCP" || updatedHealth.Enabled ||
		updatedHealth.IntervalSeconds != 30 {
		t.Fatalf("updated Resource health check = %+v", updatedHealth)
	}

	destroyHealthResponse := controllerRequest(
		t,
		http.MethodDelete,
		routes.ResourceHealthCheckDestroy.URL(healthParams),
		nil,
		echo.PathValues{
			{Name: "id", Value: resource.ID.String()},
			{Name: "healthCheckID", Value: healthCheck.ID.String()},
		},
		controller.DestroyHealthCheck,
	)
	if destroyHealthResponse.Code != http.StatusSeeOther {
		t.Fatalf(
			"destroy health check status = %d, want %d; body: %s",
			destroyHealthResponse.Code,
			http.StatusSeeOther,
			destroyHealthResponse.Body,
		)
	}
	if err := db.Executor().NewSelect().
		Model(&updatedHealth).
		Where("id = ?", healthCheck.ID).
		Scan(t.Context()); err != nil {
		t.Fatalf("load archived Resource health check: %v", err)
	}
	if !updatedHealth.ArchivedAt.Valid {
		t.Fatal("Resource health check was not archived")
	}

	destroyEndpointPage := requireControllerComponent(t, controllerRequest(
		t,
		http.MethodDelete,
		routes.ResourceEndpointDestroy.URL(endpointParams),
		nil,
		echo.PathValues{
			{Name: "id", Value: resource.ID.String()},
			{Name: "endpointID", Value: endpoint.ID.String()},
		},
		controller.DestroyEndpoint,
	), "Resources/Show")
	var destroyErrors map[string]string
	if err := json.Unmarshal(destroyEndpointPage.Props["errors"], &destroyErrors); err != nil {
		t.Fatalf("decode endpoint destroy validation errors: %v", err)
	}
	if destroyErrors["endpoint"] == "" {
		t.Fatalf("endpoint destroy validation errors = %v", destroyErrors)
	}
	retainedEndpoint, err := models.ResourceEndpoint.Find(t.Context(), db.Executor(), endpoint.ID)
	if err != nil {
		t.Fatalf("load retained Resource endpoint: %v", err)
	}
	if retainedEndpoint.ArchivedAt.Valid {
		t.Fatal("sole primary Resource endpoint was archived despite the topology invariant")
	}
}

func (*fakeResourceContainers) Run(
	context.Context,
	uuid.UUID,
	models.ServerCapability,
	containerclient.RunSpec,
) error {
	return nil
}

func (fake *fakeResourceContainers) Inspect(
	context.Context,
	uuid.UUID,
	models.ServerCapability,
	string,
	string,
) (containerclient.State, error) {
	fake.inspectCalls++
	return containerclient.State{Exists: true, Running: true}, nil
}

func (*fakeResourceContainers) Logs(
	context.Context,
	uuid.UUID,
	models.ServerCapability,
	string,
	string,
	int,
) (string, error) {
	return "", nil
}

func (*fakeResourceContainers) Start(
	context.Context,
	uuid.UUID,
	models.ServerCapability,
	string,
	string,
) error {
	return nil
}

func (fake *fakeResourceContainers) Stop(
	context.Context,
	uuid.UUID,
	models.ServerCapability,
	string,
	string,
) error {
	fake.stopCalls++
	return nil
}

func (*fakeResourceContainers) Restart(
	context.Context,
	uuid.UUID,
	models.ServerCapability,
	string,
	string,
) error {
	return nil
}

func (fake *fakeResourceContainers) Remove(
	context.Context,
	uuid.UUID,
	models.ServerCapability,
	string,
	string,
) error {
	fake.removeCalls++
	return nil
}

func (*fakeResourceContainers) RemoveVolume(
	context.Context,
	uuid.UUID,
	models.ServerCapability,
	string,
) error {
	return nil
}

func TestResourcesControllerDatabaseLifecycle(t *testing.T) {
	db := newControllerTestDB(t, seeds.UI)
	configuration := controllerTestConfig(t)
	containers := &fakeResourceContainers{}
	service := services.NewResourceManagement(db, configuration, nil, containers)
	controller := NewResources(service, nil, nil, nil, nil, nil, services.CaddyRouteService{})

	invalidPage := requireControllerComponent(t, controllerRequest(
		t,
		http.MethodPost,
		routes.ResourceCreate.URL(),
		map[string]any{
			"name":          "Missing Installation",
			"slug":          "missing-installation",
			"resourceType":  "cache",
			"configuration": json.RawMessage(`{"engine":"redis"}`),
		},
		nil,
		controller.Create,
	), "Resources/New")
	var validationErrors map[string]string
	if err := json.Unmarshal(invalidPage.Props["errors"], &validationErrors); err != nil {
		t.Fatalf("decode Resource validation errors: %v", err)
	}
	if validationErrors["installation"] == "" {
		t.Fatalf("Resource validation errors = %v, want installation error", validationErrors)
	}

	servers, err := models.Server.ActiveWorkers(t.Context(), db.Executor())
	if err != nil {
		t.Fatalf("load Resource-capable Servers: %v", err)
	}
	var serverID uuid.UUID
	for _, server := range servers {
		capabilities, parseErr := models.ParseServerCapabilities(server.Capabilities)
		if parseErr == nil && capabilities.Resource {
			serverID = server.ID
			break
		}
	}
	if serverID == uuid.Nil {
		t.Fatal("UI seed did not create a Resource-capable Server")
	}

	createResponse := controllerRequest(
		t,
		http.MethodPost,
		routes.ResourceCreate.URL(),
		map[string]any{
			"name":          "Integration Cache",
			"slug":          "integration-cache",
			"resourceType":  "cache",
			"configuration": json.RawMessage(`{"engine":"redis"}`),
			"installation": map[string]any{
				"imageReference": "redis:8-alpine",
				"containerName":  "integration-cache",
				"restartPolicy":  "unless-stopped",
				"configuration":  json.RawMessage(`{}`),
				"portMappings": []map[string]any{{
					"hostPort": 16379, "containerPort": 6379, "protocol": "tcp",
				}},
				"serverId": serverID.String(),
			},
		},
		nil,
		controller.Create,
	)
	if createResponse.Code != http.StatusSeeOther {
		t.Fatalf(
			"create status = %d, want %d; body: %s",
			createResponse.Code,
			http.StatusSeeOther,
			createResponse.Body,
		)
	}
	location := createResponse.Header().Get("Location")
	resourceID, err := uuid.Parse(location[strings.LastIndex(location, "/")+1:])
	if err != nil {
		t.Fatalf("parse created Resource ID from Location %q: %v", location, err)
	}
	created, err := models.Resource.Find(t.Context(), db.Executor(), resourceID)
	if err != nil {
		t.Fatalf("load created Resource: %v", err)
	}
	if created.Name != "Integration Cache" || created.Engine() != "redis" {
		t.Fatalf("created Resource = %+v", created)
	}

	indexResponse := controllerRequest(
		t,
		http.MethodGet,
		routes.Resources.URL()+"?search=Integration&engine=redis",
		nil,
		nil,
		controller.Index,
	)
	page := requireControllerComponent(t, indexResponse, "Resources/Index")
	var listed []struct {
		ID     uuid.UUID `json:"id"`
		Name   string    `json:"name"`
		Engine string    `json:"engine"`
	}
	if err := json.Unmarshal(page.Props["resources"], &listed); err != nil {
		t.Fatalf("decode Resource index props: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != resourceID ||
		listed[0].Name != "Integration Cache" || listed[0].Engine != "redis" {
		t.Fatalf("filtered Resources = %+v", listed)
	}

	updateResponse := controllerRequest(
		t,
		http.MethodPatch,
		routes.ResourceUpdate.URL(resourceID),
		map[string]any{
			"name":          "Renamed Integration Cache",
			"slug":          "renamed-integration-cache",
			"resourceType":  "cache",
			"configuration": json.RawMessage(`{"engine":"redis"}`),
		},
		echo.PathValues{{Name: "id", Value: resourceID.String()}},
		controller.Update,
	)
	if updateResponse.Code != http.StatusSeeOther {
		t.Fatalf(
			"update status = %d, want %d; body: %s",
			updateResponse.Code,
			http.StatusSeeOther,
			updateResponse.Body,
		)
	}
	updated, err := models.Resource.Find(t.Context(), db.Executor(), resourceID)
	if err != nil {
		t.Fatalf("load updated Resource: %v", err)
	}
	if updated.Name != "Renamed Integration Cache" || updated.Slug != "renamed-integration-cache" {
		t.Fatalf("updated Resource = %+v", updated)
	}

	destroyResponse := controllerRequest(
		t,
		http.MethodDelete,
		routes.ResourceDestroy.URL(resourceID),
		nil,
		echo.PathValues{{Name: "id", Value: resourceID.String()}},
		controller.Destroy,
	)
	if destroyResponse.Code != http.StatusSeeOther {
		t.Fatalf(
			"destroy status = %d, want %d; body: %s",
			destroyResponse.Code,
			http.StatusSeeOther,
			destroyResponse.Body,
		)
	}
	archived, err := models.Resource.Find(t.Context(), db.Executor(), resourceID)
	if err != nil {
		t.Fatalf("load archived Resource: %v", err)
	}
	if !archived.ArchivedAt.Valid {
		t.Fatal("Resource was not archived")
	}
	if containers.inspectCalls != 1 || containers.stopCalls != 1 || containers.removeCalls != 1 {
		t.Fatalf(
			"container calls = inspect:%d stop:%d remove:%d, want 1 each",
			containers.inspectCalls,
			containers.stopCalls,
			containers.removeCalls,
		)
	}
}

func TestResourcesControllerIndexUsesSeedData(t *testing.T) {
	db := newControllerTestDB(t, seeds.UI)
	service := services.NewResourceManagement(
		db,
		controllerTestConfig(t),
		nil,
		&fakeResourceContainers{},
	)
	controller := NewResources(service, nil, nil, nil, nil, nil, services.CaddyRouteService{})

	page := requireControllerComponent(t, controllerRequest(
		t,
		http.MethodGet,
		routes.Resources.URL(),
		nil,
		nil,
		controller.Index,
	), "Resources/Index")
	var resources []json.RawMessage
	if err := json.Unmarshal(page.Props["resources"], &resources); err != nil {
		t.Fatalf("decode seeded Resource props: %v", err)
	}
	if len(resources) < 3 {
		t.Fatalf("seeded Resource count = %d, want at least 3", len(resources))
	}
}
