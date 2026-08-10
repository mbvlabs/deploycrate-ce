package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	caddyclients "deploycrate-ce/clients/caddy"
	"deploycrate-ce/database/seeds"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type fakeCaddyClient struct {
	configuredRoutes []string
	appliedRoutes    []caddyclients.Route
	deletedRoutes    []string
}

func (fake *fakeCaddyClient) ApplyRoute(_ context.Context, route caddyclients.Route) error {
	fake.appliedRoutes = append(fake.appliedRoutes, route)
	return nil
}
func (fake *fakeCaddyClient) DeleteRoute(_ context.Context, externalID string) error {
	fake.deletedRoutes = append(fake.deletedRoutes, externalID)
	return nil
}
func (fake *fakeCaddyClient) RouteConfig(
	_ context.Context,
	externalID string,
) (json.RawMessage, error) {
	fake.configuredRoutes = append(fake.configuredRoutes, externalID)
	return json.RawMessage(`{"apps":{"http":{}}}`), nil
}

func TestCaddyRoutesControllerUpdateAndDestroyReconcileDatabaseRoute(t *testing.T) {
	db := newControllerTestDB(t, seeds.UI)
	client := &fakeCaddyClient{}
	service := services.NewCaddyRouteService(db, client)
	controller := NewCaddyRoutes(service)
	snapshot, err := service.ManagementSnapshot(t.Context())
	if err != nil {
		t.Fatalf("load Caddy management snapshot: %v", err)
	}
	if len(snapshot.Routes) == 0 {
		t.Fatal("UI seed did not create a managed Caddy route")
	}
	route := snapshot.Routes[0]
	routeID, err := uuid.Parse(route.ID)
	if err != nil {
		t.Fatalf("parse Caddy route ID %q: %v", route.ID, err)
	}
	domainID, err := uuid.Parse(route.EnvironmentDomainID)
	if err != nil {
		t.Fatalf("parse Caddy domain ID %q: %v", route.EnvironmentDomainID, err)
	}
	targetID, err := uuid.Parse(route.EnvironmentTargetID)
	if err != nil {
		t.Fatalf("parse Caddy target ID %q: %v", route.EnvironmentTargetID, err)
	}
	releaseID, err := uuid.Parse(route.ReleaseID)
	if err != nil {
		t.Fatalf("parse Caddy release ID %q: %v", route.ReleaseID, err)
	}
	backends := make([]services.ManagedCaddyRouteBackendInput, 0, len(route.Backends))
	for _, backend := range route.Backends {
		instanceID, parseErr := uuid.Parse(backend.InstanceID)
		if parseErr != nil {
			t.Fatalf("parse Caddy backend instance ID %q: %v", backend.InstanceID, parseErr)
		}
		backends = append(backends, services.ManagedCaddyRouteBackendInput{
			InstanceID: instanceID,
			Weight:     backend.Weight,
		})
	}
	input := services.ManagedCaddyRouteInput{
		ExternalID:          route.ExternalID,
		EnvironmentDomainID: domainID,
		EnvironmentTargetID: targetID,
		ReleaseID:           releaseID,
		Backends:            backends,
	}

	updateResponse := controllerRequest(
		t,
		http.MethodPatch,
		routes.CaddyRouteUpdate.URL(routeID),
		input,
		echo.PathValues{{Name: "id", Value: routeID.String()}},
		controller.Update,
	)
	if updateResponse.Code != http.StatusSeeOther {
		t.Fatalf("update status = %d, want %d; body: %s", updateResponse.Code, http.StatusSeeOther, updateResponse.Body)
	}
	if len(client.appliedRoutes) != 1 {
		t.Fatalf("Caddy apply calls = %d, want 1", len(client.appliedRoutes))
	}

	destroyResponse := controllerRequest(
		t,
		http.MethodDelete,
		routes.CaddyRouteDestroy.URL(routeID),
		nil,
		echo.PathValues{{Name: "id", Value: routeID.String()}},
		controller.Destroy,
	)
	if destroyResponse.Code != http.StatusSeeOther {
		t.Fatalf("destroy status = %d, want %d; body: %s", destroyResponse.Code, http.StatusSeeOther, destroyResponse.Body)
	}
	if len(client.deletedRoutes) != 1 || client.deletedRoutes[0] != route.ExternalID {
		t.Fatalf("Caddy delete calls = %v, want [%s]", client.deletedRoutes, route.ExternalID)
	}
	after, err := service.ManagementSnapshot(t.Context())
	if err != nil {
		t.Fatalf("reload Caddy management snapshot: %v", err)
	}
	for _, remaining := range after.Routes {
		if remaining.ID == route.ID {
			t.Fatalf("destroyed Caddy route still appears in management snapshot: %+v", remaining)
		}
	}
}
func (*fakeCaddyClient) VerifyRoute(context.Context, string) error          { return nil }
func (*fakeCaddyClient) VerifyPublic(context.Context, string, string) error { return nil }

func TestCaddyRoutesControllerIndexAndShowUseDatabase(t *testing.T) {
	db := newControllerTestDB(t, seeds.UI)
	client := &fakeCaddyClient{}
	controller := NewCaddyRoutes(services.NewCaddyRouteService(db, client))

	page := requireControllerComponent(t, controllerRequest(
		t,
		http.MethodGet,
		routes.CaddyRoutes.URL(),
		nil,
		nil,
		controller.Index,
	), "System/CaddyRoutes/Index")
	var caddyRoutes []services.ManagedCaddyRoute
	if err := json.Unmarshal(page.Props["routes"], &caddyRoutes); err != nil {
		t.Fatalf("decode seeded Caddy routes: %v", err)
	}
	if len(caddyRoutes) == 0 {
		t.Fatal("UI seed did not render a managed Caddy route")
	}
	externalID := caddyRoutes[0].ExternalID
	requireControllerComponent(t, controllerRequest(
		t,
		http.MethodGet,
		routes.CaddyRouteShow.URL(externalID),
		nil,
		echo.PathValues{{Name: "id", Value: externalID}},
		controller.Show,
	), "System/CaddyRoutes/Show")
	if len(client.configuredRoutes) != 1 || client.configuredRoutes[0] != externalID {
		t.Fatalf("Caddy configuration calls = %v, want [%s]", client.configuredRoutes, externalID)
	}
}
