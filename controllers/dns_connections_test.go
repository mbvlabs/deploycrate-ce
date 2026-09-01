package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	cloudflareclient "deploycrate-ce/clients/cloudflare"
	"deploycrate-ce/models"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/labstack/echo/v5"
)

type fakeCloudflareDNS struct {
	zones        []cloudflareclient.Zone
	verifiedWith []string
}

func (fake *fakeCloudflareDNS) VerifyAccountToken(
	_ context.Context,
	_ string,
	token string,
) error {
	fake.verifiedWith = append(fake.verifiedWith, token)
	return nil
}

func (fake *fakeCloudflareDNS) ListZones(
	context.Context,
	string,
	string,
) ([]cloudflareclient.Zone, error) {
	return append([]cloudflareclient.Zone(nil), fake.zones...), nil
}

func (*fakeCloudflareDNS) ListAddressRecords(
	context.Context,
	string,
	string,
	string,
) ([]cloudflareclient.DNSRecord, error) {
	return nil, nil
}

func (*fakeCloudflareDNS) CreateARecord(
	context.Context,
	string,
	string,
	cloudflareclient.DNSRecordInput,
) (cloudflareclient.DNSRecord, error) {
	return cloudflareclient.DNSRecord{}, nil
}

func (*fakeCloudflareDNS) UpdateARecord(
	context.Context,
	string,
	string,
	string,
	cloudflareclient.DNSRecordInput,
) (cloudflareclient.DNSRecord, error) {
	return cloudflareclient.DNSRecord{}, nil
}

func (*fakeCloudflareDNS) DeleteRecord(
	context.Context,
	string,
	string,
	string,
) error {
	return nil
}

func TestDNSConnectionsControllerDatabaseLifecycle(t *testing.T) {
	db := newControllerTestDB(t, nil)
	fake := &fakeCloudflareDNS{zones: []cloudflareclient.Zone{
		{ID: "zone-one", Name: "example.com", Status: "active"},
		{ID: "zone-two", Name: "example.net", Status: "active"},
	}}
	service := services.NewDNSConnections(db, fake, controllerTestConfig(t))
	controller := NewDNSConnections(service)

	invalidPage := requireControllerComponent(t, controllerRequest(
		t,
		http.MethodPost,
		routes.DnsConnectionCreate.URL(),
		map[string]any{
			"name":      "Invalid Cloudflare",
			"accountId": "0123456789abcdef0123456789abcdef",
			"token":     "",
		},
		nil,
		controller.Create,
	), "Connections/DNS")
	var validationErrors map[string]string
	if err := json.Unmarshal(invalidPage.Props["errors"], &validationErrors); err != nil {
		t.Fatalf("decode DNS validation errors: %v", err)
	}
	if validationErrors["token"] == "" {
		t.Fatalf("DNS validation errors = %v, want token error", validationErrors)
	}
	if len(fake.verifiedWith) != 0 {
		t.Fatalf("Cloudflare was called for invalid input: %v", fake.verifiedWith)
	}

	createResponse := controllerRequest(
		t,
		http.MethodPost,
		routes.DnsConnectionCreate.URL(),
		map[string]any{
			"name":      "Production Cloudflare",
			"accountId": "0123456789abcdef0123456789abcdef",
			"token":     "initial-token",
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

	connections, err := service.List(t.Context())
	if err != nil {
		t.Fatalf("list DNS connections: %v", err)
	}
	if len(connections) != 1 || connections[0].Name != "Production Cloudflare" ||
		connections[0].ActiveZones != 2 {
		t.Fatalf("created DNS connections = %+v", connections)
	}
	connectionID := connections[0].ID

	indexResponse := controllerRequest(
		t,
		http.MethodGet,
		routes.DnsConnections.URL(),
		nil,
		nil,
		controller.Index,
	)
	page := requireControllerComponent(t, indexResponse, "Connections/DNS")
	var listed []models.DNSConnectionSummary
	if err := json.Unmarshal(page.Props["connections"], &listed); err != nil {
		t.Fatalf("decode DNS index props: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != connectionID {
		t.Fatalf("DNS index connections = %+v", listed)
	}

	showResponse := controllerRequest(
		t,
		http.MethodGet,
		routes.DnsConnectionShow.URL(connectionID),
		nil,
		echo.PathValues{{Name: "id", Value: connectionID.String()}},
		controller.Show,
	)
	showPage := requireControllerComponent(t, showResponse, "Connections/DNS/Show")
	var zones []models.DNSZoneSummary
	if err := json.Unmarshal(showPage.Props["zones"], &zones); err != nil {
		t.Fatalf("decode DNS zone props: %v", err)
	}
	if len(zones) != 2 {
		t.Fatalf("DNS zones = %+v, want 2", zones)
	}

	fake.zones = []cloudflareclient.Zone{
		{ID: "zone-two", Name: "renamed.example.net", Status: "active"},
	}
	syncResponse := controllerRequest(
		t,
		http.MethodPost,
		routes.DnsConnectionSync.URL(connectionID),
		nil,
		echo.PathValues{{Name: "id", Value: connectionID.String()}},
		controller.Sync,
	)
	if syncResponse.Code != http.StatusSeeOther {
		t.Fatalf(
			"sync status = %d, want %d; body: %s",
			syncResponse.Code,
			http.StatusSeeOther,
			syncResponse.Body,
		)
	}
	zones, err = service.Zones(t.Context(), connectionID)
	if err != nil {
		t.Fatalf("load synchronized zones: %v", err)
	}
	if len(zones) != 1 || zones[0].Name != "renamed.example.net" {
		t.Fatalf("synchronized zones = %+v", zones)
	}

	rotateResponse := controllerRequest(
		t,
		http.MethodPatch,
		routes.DnsConnectionTokenUpdate.URL(connectionID),
		map[string]string{"token": "rotated-token"},
		echo.PathValues{{Name: "id", Value: connectionID.String()}},
		controller.RotateToken,
	)
	if rotateResponse.Code != http.StatusSeeOther {
		t.Fatalf(
			"rotate status = %d, want %d; body: %s",
			rotateResponse.Code,
			http.StatusSeeOther,
			rotateResponse.Body,
		)
	}
	if len(fake.verifiedWith) != 3 || fake.verifiedWith[2] != "rotated-token" {
		t.Fatalf("verified tokens = %v", fake.verifiedWith)
	}

	destroyResponse := controllerRequest(
		t,
		http.MethodDelete,
		routes.DnsConnectionDestroy.URL(connectionID),
		nil,
		echo.PathValues{{Name: "id", Value: connectionID.String()}},
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
	connections, err = service.List(t.Context())
	if err != nil {
		t.Fatalf("list DNS connections after archive: %v", err)
	}
	if len(connections) != 0 {
		t.Fatalf("active DNS connections after archive = %+v", connections)
	}
	archived, err := models.DNSConnection.Find(t.Context(), db.Executor(), connectionID)
	if err != nil {
		t.Fatalf("load archived DNS connection: %v", err)
	}
	if !archived.ArchivedAt.Valid {
		t.Fatal("DNS connection was not archived")
	}
}
