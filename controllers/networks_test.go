package controllers

import (
	"net/http"
	"testing"

	"deploycrate-ce/database/seeds"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func TestNetworksControllerIndexUsesSeedData(t *testing.T) {
	db := newControllerTestDB(t, seeds.UI)
	controller := NewNetworks(db, nil)

	page := requireControllerComponent(t, controllerRequest(
		t,
		http.MethodGet,
		routes.Networks.URL(),
		nil,
		nil,
		controller.Index,
	), "Networks/Index")
	if len(page.Props["network"]) == 0 || string(page.Props["network"]) == "null" {
		t.Fatal("seeded system network was not rendered")
	}
}

func TestNetworksControllerDestroyMissingDeviceIsIdempotent(t *testing.T) {
	db := newControllerTestDB(t, seeds.UI)
	controller := NewNetworks(db, services.NewResourcePrivateAccess(db, nil))
	deviceID := uuid.New()

	response := controllerRequest(
		t,
		http.MethodDelete,
		routes.NetworkWireGuardDeviceDestroy.URL(deviceID),
		nil,
		echo.PathValues{{Name: "id", Value: deviceID.String()}},
		controller.DestroyWireGuardDevice,
	)
	if response.Code != http.StatusSeeOther {
		t.Fatalf(
			"destroy status = %d, want %d; body: %s",
			response.Code,
			http.StatusSeeOther,
			response.Body,
		)
	}
}
