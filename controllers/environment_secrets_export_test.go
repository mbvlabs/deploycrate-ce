package controllers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"deploycrate-ce/models"
	"deploycrate-ce/models/factories"
	"deploycrate-ce/router/cookies"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func TestEnvironmentSecretsExport(t *testing.T) {
	db := newControllerTestDB(t, nil)
	cfg := controllerTestConfig(t)
	hash, err := models.HashPassword("export-password", factories.TestPepper)
	if err != nil {
		t.Fatal(err)
	}
	user, err := factories.CreateUser(t.Context(), db.Executor(), factories.WithUsersPassword([]byte(hash)))
	if err != nil {
		t.Fatal(err)
	}
	application, err := factories.CreateApplication(t.Context(), db.Executor(), factories.WithApplicationsArchivedAt(sql.NullTime{}))
	if err != nil {
		t.Fatal(err)
	}
	environment, err := factories.CreateEnvironment(t.Context(), db.Executor(), application.ID, factories.WithEnvironmentsArchivedAt(sql.NullTime{}))
	if err != nil {
		t.Fatal(err)
	}
	service := services.NewEnvironmentSecrets(db, cfg, nil)
	prepared, err := service.Prepare(environment.ID, "API_TOKEN", "00123", models.EnvironmentSecretSourceUser, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	secret, err := service.CreatePrepared(t.Context(), tx, prepared)
	if err != nil {
		t.Fatal(err)
	}
	unused, err := service.Prepare(environment.ID, "NOT_IN_REVISION", "not-current", models.EnvironmentSecretSourceUser, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreatePrepared(t.Context(), tx, unused); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	state, err := models.CanonicalEnvironmentDesiredState(models.EnvironmentDesiredState{
		Runtime:   models.EnvironmentRuntimeState{Runtime: "go", RestartPolicy: "unless-stopped"},
		Processes: []models.EnvironmentProcessState{{Name: "web", Kind: "web", Replicas: 1, ContainerPort: 8080, HealthPath: "/"}},
		Domain:    models.EnvironmentDomainState{ID: uuid.New(), Hostname: "export.example.com", Primary: true},
		Secrets:   []models.EnvironmentSecretDescriptor{models.EnvironmentSecretDescriptorFromEntity(secret)},
	})
	if err != nil {
		t.Fatal(err)
	}
	change, err := factories.CreateChange(t.Context(), db.Executor(), &user.ID, uuid.New(), environment.ID, nil,
		factories.WithChangesCommittedAt(sql.NullTime{Time: time.Now(), Valid: true}), factories.WithChangesCancelledAt(sql.NullTime{}),
		factories.WithChangesCorrectionContext(json.RawMessage(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = factories.CreateEnvironmentStateRevision(t.Context(), db.Executor(), environment.ID, change.ID, factories.WithEnvironmentStateRevisionsState(state))
	if err != nil {
		t.Fatal(err)
	}
	controller := Environments{envSecretsSvc: service}
	for _, test := range []struct {
		name, password string
		applicationID  uuid.UUID
		signedIn       bool
		status         int
	}{
		{"correct password", "export-password", application.ID, true, http.StatusOK},
		{"wrong password", "wrong-password", application.ID, true, http.StatusForbidden},
		{"missing password", "", application.ID, true, http.StatusForbidden},
		{"no user", "export-password", application.ID, false, http.StatusForbidden},
		{"different application", "export-password", uuid.New(), true, http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			params := routes.EnvironmentParams{ApplicationID: test.applicationID.String(), EnvironmentID: environment.ID.String()}
			response := controllerRequest(t, http.MethodPost, routes.EnvironmentSecretsExport.URL(params), map[string]string{"password": test.password},
				echo.PathValues{{Name: "applicationID", Value: params.ApplicationID}, {Name: "environmentID", Value: params.EnvironmentID}},
				func(c *echo.Context) error {
					if test.signedIn {
						if err := cookies.CreateAppSession(c, user); err != nil {
							return err
						}
					}
					return controller.ExportSecrets(c)
				})
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, test.status, response.Body)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("export must not be cached")
			}
			if test.status == http.StatusOK {
				var payload struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Text != `API_TOKEN="00123"` {
					t.Fatal("export did not preserve the secret")
				}
			} else if strings.Contains(response.Body.String(), "00123") {
				t.Fatal("failed export revealed a secret")
			}
		})
	}
}
