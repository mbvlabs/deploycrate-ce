package controllers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"deploycrate-ce/models"
	"deploycrate-ce/models/factories"
	"deploycrate-ce/queue"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func TestObjectStorageControllerIndexAndShowUseDatabase(t *testing.T) {
	db := newControllerTestDB(t, nil)
	configuration := controllerTestConfig(t)
	credential, err := factories.CreateCredential(
		t.Context(),
		db.Executor(),
		factories.WithCredentialsName("Integration S3"),
		factories.WithCredentialsProvider("backup_s3"),
		factories.WithCredentialsMetadata(json.RawMessage(`{}`)),
		factories.WithCredentialsEncPayload([]byte("encrypted-test-credentials")),
		factories.WithCredentialsVerifiedAt(sql.NullTime{Time: time.Now(), Valid: true}),
		factories.WithCredentialsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		t.Fatalf("create Object Storage credential fixture: %v", err)
	}
	destination, err := factories.CreateBackupDestination(
		t.Context(),
		db.Executor(),
		credential.ID,
		factories.WithBackupDestinationsName("Integration Backups"),
		factories.WithBackupDestinationsProvider("s3"),
		factories.WithBackupDestinationsRegion(sql.NullString{String: "eu-west-1", Valid: true}),
		factories.WithBackupDestinationsBucket("integration-backups"),
		factories.WithBackupDestinationsPrefix(sql.NullString{String: "deploycrate", Valid: true}),
		factories.WithBackupDestinationsArchivedAt(sql.NullTime{}),
	)
	if err != nil {
		t.Fatalf("create Object Storage destination fixture: %v", err)
	}
	identity := services.NewIdentity(db, queue.InsertOnly{}, configuration)
	controller := NewObjectStorage(
		services.NewDatabaseBackups(db, nil, configuration, identity),
	)

	page := requireControllerComponent(t, controllerRequest(
		t,
		http.MethodGet,
		routes.ObjectStorage.URL(),
		nil,
		nil,
		controller.Index,
	), "Connections/ObjectStorage")
	var destinations []map[string]any
	if err := json.Unmarshal(page.Props["destinations"], &destinations); err != nil {
		t.Fatalf("decode Object Storage props: %v", err)
	}
	if len(destinations) != 1 || destinations[0]["name"] != "Integration Backups" {
		t.Fatalf("Object Storage destinations = %+v", destinations)
	}
	requireControllerComponent(t, controllerRequest(
		t,
		http.MethodGet,
		routes.ObjectStorageShow.URL(destination.ID),
		nil,
		echo.PathValues{{Name: "id", Value: destination.ID.String()}},
		controller.Show,
	), "Connections/ObjectStorage/Show")
}

func TestObjectStorageControllerCreateValidationDoesNotWriteDatabase(t *testing.T) {
	db := newControllerTestDB(t, nil)
	configuration := controllerTestConfig(t)
	controller := NewObjectStorage(services.NewDatabaseBackups(
		db,
		nil,
		configuration,
		services.NewIdentity(db, queue.InsertOnly{}, configuration),
	))

	page := requireControllerComponent(t, controllerRequest(
		t,
		http.MethodPost,
		routes.ObjectStorageCreate.URL(),
		map[string]string{"name": "", "provider": "s3", "bucket": ""},
		nil,
		controller.Create,
	), "Connections/ObjectStorage")
	var validationErrors map[string]string
	if err := json.Unmarshal(page.Props["errors"], &validationErrors); err != nil {
		t.Fatalf("decode Object Storage validation errors: %v", err)
	}
	if validationErrors["name"] == "" || validationErrors["bucket"] == "" ||
		validationErrors["accessKeyId"] == "" || validationErrors["secretAccessKey"] == "" {
		t.Fatalf("Object Storage validation errors = %v", validationErrors)
	}
	destinationCount, err := db.Executor().NewSelect().
		Model((*models.BackupDestinationEntity)(nil)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count destinations after invalid create: %v", err)
	}
	credentialCount, err := db.Executor().NewSelect().
		Model((*models.CredentialEntity)(nil)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count credentials after invalid create: %v", err)
	}
	if destinationCount != 0 || credentialCount != 0 {
		t.Fatalf(
			"rows after invalid create = destinations:%d credentials:%d, want 0 each",
			destinationCount,
			credentialCount,
		)
	}
}

func TestObjectStorageControllerRecoveryRejectsMissingPasswordBeforeDatabaseLookup(t *testing.T) {
	db := newControllerTestDB(t, nil)
	configuration := controllerTestConfig(t)
	controller := NewObjectStorage(services.NewDatabaseBackups(
		db,
		nil,
		configuration,
		services.NewIdentity(db, queue.InsertOnly{}, configuration),
	))
	id := uuid.New()

	response := controllerRequest(
		t,
		http.MethodPost,
		routes.ObjectStorageRecovery.URL(id),
		map[string]string{"password": ""},
		echo.PathValues{{Name: "id", Value: id.String()}},
		controller.Recovery,
	)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf(
			"recovery status = %d, want %d; body: %s",
			response.Code,
			http.StatusUnprocessableEntity,
			response.Body,
		)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", response.Header().Get("Cache-Control"))
	}
}
