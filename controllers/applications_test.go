package controllers

import (
	"database/sql"
	"net/http"
	"testing"

	"deploycrate-ce/models"
	"deploycrate-ce/models/factories"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func TestApplicationsControllerUpdatesDatabase(t *testing.T) {
	db := newControllerTestDB(t, nil)
	application, err := factories.CreateApplication(
		t.Context(),
		db.Executor(),
		factories.WithApplicationsName("Before"),
		factories.WithApplicationsSlug("before"),
		factories.WithApplicationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		t.Fatalf("create Application fixture: %v", err)
	}
	controller := NewApplications(
		services.NewApplicationSetup(db, controllerTestConfig(t)),
		nil,
	)

	response := controllerRequest(
		t,
		http.MethodPatch,
		routes.ApplicationUpdate.URL(application.ID),
		map[string]string{"name": "After", "slug": "after"},
		echo.PathValues{{Name: "id", Value: application.ID.String()}},
		controller.Update,
	)
	if response.Code != http.StatusSeeOther {
		t.Fatalf(
			"update status = %d, want %d; body: %s",
			response.Code,
			http.StatusSeeOther,
			response.Body,
		)
	}
	updated, err := models.Application.Find(t.Context(), db.Executor(), application.ID)
	if err != nil {
		t.Fatalf("load updated Application: %v", err)
	}
	if updated.Name != "After" || updated.Slug != "after" {
		t.Fatalf("updated Application = %+v", updated)
	}
	if updated.ID == uuid.Nil {
		t.Fatal("updated Application has a nil ID")
	}
}

func TestApplicationsControllerDestroyDeletesEmptyApplication(t *testing.T) {
	db := newControllerTestDB(t, nil)
	application, err := factories.CreateApplication(
		t.Context(),
		db.Executor(),
		factories.WithApplicationsName("Disposable"),
		factories.WithApplicationsSlug("disposable"),
		factories.WithApplicationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		t.Fatalf("create Application fixture: %v", err)
	}
	controller := NewApplications(
		services.NewApplicationSetup(db, controllerTestConfig(t)),
		newControllerEnvironmentSetup(t, db),
	)

	response := controllerRequest(
		t,
		http.MethodDelete,
		routes.ApplicationDestroy.URL(application.ID),
		nil,
		echo.PathValues{{Name: "id", Value: application.ID.String()}},
		controller.Destroy,
	)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("destroy status = %d, want %d; body: %s", response.Code, http.StatusSeeOther, response.Body)
	}
	if _, err := models.Application.Find(t.Context(), db.Executor(), application.ID); err == nil {
		t.Fatal("Application still exists after destroy")
	}
}
