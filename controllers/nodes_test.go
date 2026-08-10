package controllers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"deploycrate-ce/database/seeds"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type failingControllerInsertQueue struct {
	storage.InsertQueue
	err error
}

func (queue failingControllerInsertQueue) InsertTx(
	context.Context,
	*sql.Tx,
	river.JobArgs,
	*river.InsertOpts,
) (*rivertype.JobInsertResult, error) {
	return nil, queue.err
}

func createControllerNodeEnrollment(
	t *testing.T,
	db storage.Pool,
	data models.CreateNodeEnrollmentData,
) models.NodeEnrollmentEntity {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin Node enrollment fixture transaction: %v", err)
	}
	defer tx.Rollback()
	enrollment, err := models.NodeEnrollment.Create(t.Context(), tx, data)
	if err != nil {
		t.Fatalf("create Node enrollment fixture: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit Node enrollment fixture: %v", err)
	}
	return enrollment
}

func TestNodesControllerIndexAndShowUseDatabase(t *testing.T) {
	db := newControllerTestDB(t, seeds.UI)
	configuration := controllerTestConfig(t)
	workers, err := models.Server.ActiveWorkers(t.Context(), db.Executor())
	if err != nil {
		t.Fatalf("load seeded worker Servers: %v", err)
	}
	var firstEnrollmentID uuid.UUID
	for index, worker := range workers {
		enrollment := createControllerNodeEnrollment(
			t,
			db,
			models.CreateNodeEnrollmentData{
				HostFingerprint:  "SHA256:integration-fingerprint",
				AllocatedAddress: fmt.Sprintf("10.99.1.%d", index+2),
				InstallerVersion: "integration",
				ServerID:         worker.ID,
			},
		)
		if firstEnrollmentID == uuid.Nil {
			firstEnrollmentID = enrollment.ID
		}
	}
	service := services.NewNodeEnrollment(
		db,
		queue.InsertOnly{},
		configuration,
		services.CurrentVersion("integration"),
		services.NewSSHCAService(configuration),
		nil,
	)
	controller := NewNodes(service, services.NewServerManagement(db, nil))

	page := requireControllerComponent(t, controllerRequest(
		t,
		http.MethodGet,
		routes.Nodes.URL(),
		nil,
		nil,
		controller.Index,
	), "Nodes/Index")
	var nodes []json.RawMessage
	if err := json.Unmarshal(page.Props["nodes"], &nodes); err != nil {
		t.Fatalf("decode Node props: %v", err)
	}
	if len(nodes) != len(workers) {
		t.Fatalf("Node count = %d, want %d", len(nodes), len(workers))
	}
	requireControllerComponent(t, controllerRequest(
		t,
		http.MethodGet,
		routes.NodeShow.URL(firstEnrollmentID),
		nil,
		echo.PathValues{{Name: "id", Value: firstEnrollmentID.String()}},
		controller.Show,
	), "Nodes/Show")
}

func TestNodesControllerCreateValidationDoesNotWriteDatabase(t *testing.T) {
	db := newControllerTestDB(t, nil)
	configuration := controllerTestConfig(t)
	controller := NewNodes(
		services.NewNodeEnrollment(
			db,
			queue.InsertOnly{},
			configuration,
			services.CurrentVersion("integration"),
			services.NewSSHCAService(configuration),
			nil,
		),
		services.NewServerManagement(db, nil),
	)

	page := requireControllerComponent(t, controllerRequest(
		t,
		http.MethodPost,
		routes.NodeCreate.URL(),
		map[string]any{"name": "", "address": "bad address", "port": 0},
		nil,
		controller.Create,
	), "Nodes/New")
	var validationErrors map[string]string
	if err := json.Unmarshal(page.Props["errors"], &validationErrors); err != nil {
		t.Fatalf("decode Node validation errors: %v", err)
	}
	if validationErrors["name"] == "" || validationErrors["address"] == "" {
		t.Fatalf("Node validation errors = %v", validationErrors)
	}
	serverCount, err := db.Executor().NewSelect().Model((*models.ServerEntity)(nil)).Count(t.Context())
	if err != nil {
		t.Fatalf("count Servers after invalid create: %v", err)
	}
	if serverCount != 0 {
		t.Fatalf("Server count after invalid create = %d, want 0", serverCount)
	}
}

func TestNodesControllerConfirmRollsBackWhenQueueInsertFails(t *testing.T) {
	db := newControllerTestDB(t, seeds.UI)
	configuration := controllerTestConfig(t)
	workers, err := models.Server.ActiveWorkers(t.Context(), db.Executor())
	if err != nil || len(workers) == 0 {
		t.Fatalf("load seeded worker Server: workers=%d err=%v", len(workers), err)
	}
	enrollment := createControllerNodeEnrollment(
		t,
		db,
		models.CreateNodeEnrollmentData{
			HostFingerprint:  "SHA256:rollback-fingerprint",
			AllocatedAddress: "10.99.2.2",
			InstallerVersion: "integration",
			ServerID:         workers[0].ID,
		},
	)
	queueFailure := errors.New("integration queue failure")
	controller := NewNodes(
		services.NewNodeEnrollment(
			db,
			failingControllerInsertQueue{err: queueFailure},
			configuration,
			services.CurrentVersion("integration"),
			services.NewSSHCAService(configuration),
			nil,
		),
		services.NewServerManagement(db, nil),
	)

	response := controllerRequest(
		t,
		http.MethodPost,
		routes.NodeConfirm.URL(enrollment.ID),
		map[string]string{"fingerprint": enrollment.HostFingerprint},
		echo.PathValues{{Name: "id", Value: enrollment.ID.String()}},
		controller.Confirm,
	)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("confirm status = %d, want %d; body: %s", response.Code, http.StatusSeeOther, response.Body)
	}
	after, err := models.NodeEnrollment.Find(t.Context(), db.Executor(), enrollment.ID)
	if err != nil {
		t.Fatalf("reload Node enrollment: %v", err)
	}
	if after.State != models.NodeEnrollmentAwaitingConfirmation || after.JobID.Valid {
		t.Fatalf("Node enrollment after failed queue insert = %+v", after)
	}
	credential, err := models.ServerSSHCredential.FindForServer(
		t.Context(),
		db.Executor(),
		workers[0].ID,
	)
	if err != nil {
		t.Fatalf("reload SSH credential: %v", err)
	}
	if credential.HostKeyConfirmedAt.Valid {
		t.Fatal("SSH host key confirmation was not rolled back")
	}
}
