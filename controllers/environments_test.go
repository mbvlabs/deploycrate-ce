package controllers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"deploycrate-ce/database/seeds"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/models/factories"
	"deploycrate-ce/queue"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/labstack/echo/v5"
)

func newControllerEnvironmentSetup(t *testing.T, db storage.Pool) *services.EnvironmentSetup {
	t.Helper()
	configuration := controllerTestConfig(t)
	dns := services.NewEnvironmentDNS(db, queue.InsertOnly{}, nil, configuration)
	return services.NewEnvironmentSetup(
		db,
		queue.InsertOnly{},
		nil,
		nil,
		nil,
		nil,
		dns,
		services.CaddyRouteService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		configuration,
	)
}

func TestEnvironmentsControllerIndexUsesDatabase(t *testing.T) {
	db := newControllerTestDB(t, nil)
	configuration := controllerTestConfig(t)
	application, err := factories.CreateApplication(
		t.Context(),
		db.Executor(),
		factories.WithApplicationsName("Integration Application"),
		factories.WithApplicationsSlug("integration-application"),
		factories.WithApplicationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		t.Fatalf("create Environment Application fixture: %v", err)
	}
	if _, err := factories.CreateEnvironment(
		t.Context(),
		db.Executor(),
		application.ID,
		factories.WithEnvironmentsName("Staging"),
		factories.WithEnvironmentsSlug("staging"),
		factories.WithEnvironmentsKind("staging"),
		factories.WithEnvironmentsArchivedAt(sql.NullTime{}),
	); err != nil {
		t.Fatalf("create Environment fixture: %v", err)
	}
	service := services.NewEnvironmentSetup(
		db,
		queue.InsertOnly{},
		nil,
		nil,
		nil,
		nil,
		nil,
		services.CaddyRouteService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		configuration,
	)
	controller := NewEnvironments(
		service,
		nil,
		services.NewApplicationSetup(db, configuration),
		services.MetricRollupService{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	page := requireControllerComponent(t, controllerRequest(
		t,
		http.MethodGet,
		routes.Environments.URL(),
		nil,
		nil,
		controller.Index,
	), "Environments/Index")
	var environments []services.EnvironmentListItem
	if err := json.Unmarshal(page.Props["environments"], &environments); err != nil {
		t.Fatalf("decode Environment props: %v", err)
	}
	if len(environments) != 1 || environments[0].ApplicationName != "Integration Application" {
		t.Fatalf("Environments = %+v", environments)
	}
}

func TestEnvironmentsControllerDestroyDeletesEnvironmentAndEmptyApplication(t *testing.T) {
	db := newControllerTestDB(t, seeds.UI)
	application, err := factories.CreateApplication(
		t.Context(),
		db.Executor(),
		factories.WithApplicationsName("Disposable Application"),
		factories.WithApplicationsSlug("disposable-application"),
		factories.WithApplicationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		t.Fatalf("create Application fixture: %v", err)
	}
	environment, err := factories.CreateEnvironment(
		t.Context(),
		db.Executor(),
		application.ID,
		factories.WithEnvironmentsName("Disposable Environment"),
		factories.WithEnvironmentsSlug("disposable-environment"),
		factories.WithEnvironmentsKind("production"),
		factories.WithEnvironmentsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		t.Fatalf("create Environment fixture: %v", err)
	}
	controller := NewEnvironments(
		newControllerEnvironmentSetup(t, db),
		nil,
		services.NewApplicationSetup(db, controllerTestConfig(t)),
		services.MetricRollupService{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	params := routes.EnvironmentParams{
		ApplicationID: application.ID.String(),
		EnvironmentID: environment.ID.String(),
	}

	response := controllerRequest(
		t,
		http.MethodDelete,
		routes.EnvironmentDestroy.URL(params),
		nil,
		echo.PathValues{
			{Name: "applicationID", Value: application.ID.String()},
			{Name: "environmentID", Value: environment.ID.String()},
		},
		controller.Destroy,
	)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("destroy status = %d, want %d; body: %s", response.Code, http.StatusSeeOther, response.Body)
	}
	if _, err := models.Environment.Find(t.Context(), db.Executor(), environment.ID); err == nil {
		t.Fatal("Environment still exists after destroy")
	}
	if _, err := models.Application.Find(t.Context(), db.Executor(), application.ID); err == nil {
		t.Fatal("empty Application still exists after Environment destroy")
	}
}
