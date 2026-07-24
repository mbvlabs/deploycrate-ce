package models_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"deploycrate-ce/database"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/models/factories"

	"github.com/google/uuid"
)

func TestSystemUpdateCheckpointPersistence(t *testing.T) {
	cluster, err := storage.NewTestCluster(context.Background())
	if err != nil {
		t.Fatalf("start Postgres test cluster: %v", err)
	}
	t.Cleanup(func() {
		if err := cluster.Close(context.Background()); err != nil {
			t.Errorf("close Postgres test cluster: %v", err)
		}
	})

	t.Run("saves checkpoint as JSON", func(t *testing.T) {
		db := cluster.NewTestDB(t, database.Migrations, "migrations")
		deployment := createDeploymentFixture(t, db.Executor())
		checkpoint := models.SystemUpdateCheckpoint{
			ServiceTemplate:    "deploycrate-ce@.service",
			ActiveSlot:         "blue",
			TargetSlot:         "green",
			Phase:              "target_started",
			PreviousSlotTarget: "/opt/deploycrate-ce/releases/v1.0.0/deploycrate-ce",
			TargetStarted:      true,
		}

		if err := models.Deployment.SaveSystemUpdateCheckpoint(
			t.Context(),
			db.Executor(),
			deployment.ID,
			checkpoint,
		); err != nil {
			t.Fatalf("save system update checkpoint: %v", err)
		}

		persisted, err := models.Deployment.Find(t.Context(), db.Executor(), deployment.ID)
		if err != nil {
			t.Fatalf("find deployment: %v", err)
		}
		assertCheckpoint(t, persisted.RuntimeConfiguration, checkpoint)
	})

	t.Run("finishes deployment with checkpoint as JSON", func(t *testing.T) {
		db := cluster.NewTestDB(t, database.Migrations, "migrations")
		deployment := createDeploymentFixture(t, db.Executor())
		checkpoint := models.SystemUpdateCheckpoint{
			ServiceTemplate:    "deploycrate-ce@.service",
			ActiveSlot:         "blue",
			TargetSlot:         "green",
			Phase:              "rolled_back",
			PreviousSlotTarget: "/opt/deploycrate-ce/releases/v1.0.0/deploycrate-ce",
			TargetStarted:      true,
			TrafficSwitched:    true,
		}
		failure := sql.NullString{String: "public health check failed", Valid: true}

		if err := models.Deployment.FinishSystemUpdate(
			t.Context(),
			db.Executor(),
			deployment.ID,
			"failed",
			"failed",
			failure,
			checkpoint,
			time.Now().UTC(),
		); err != nil {
			t.Fatalf("finish system update: %v", err)
		}

		persisted, err := models.Deployment.Find(t.Context(), db.Executor(), deployment.ID)
		if err != nil {
			t.Fatalf("find deployment: %v", err)
		}
		if persisted.Status != "failed" {
			t.Errorf("status = %q, want failed", persisted.Status)
		}
		if persisted.CurrentStep.String != "failed" || !persisted.CurrentStep.Valid {
			t.Errorf("current step = %+v, want failed", persisted.CurrentStep)
		}
		if persisted.Error != failure {
			t.Errorf("error = %+v, want %+v", persisted.Error, failure)
		}
		if !persisted.FinishedAt.Valid {
			t.Error("finished at is not set")
		}
		assertCheckpoint(t, persisted.RuntimeConfiguration, checkpoint)
	})
}

func createDeploymentFixture(t *testing.T, exec storage.Executor) models.DeploymentEntity {
	t.Helper()

	application, err := factories.CreateApplication(t.Context(), exec)
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	environment, err := factories.CreateEnvironment(t.Context(), exec, application.ID)
	if err != nil {
		t.Fatalf("create environment: %v", err)
	}
	server, err := factories.CreateServer(
		t.Context(),
		exec,
		factories.WithServersCapabilities(json.RawMessage(`{}`)),
	)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	target, err := factories.CreateEnvironmentTarget(t.Context(), exec, environment.ID, server.ID)
	if err != nil {
		t.Fatalf("create environment target: %v", err)
	}
	change, err := factories.CreateChange(
		t.Context(),
		exec,
		nil,
		uuid.New(),
		environment.ID,
		nil,
		factories.WithChangesCorrectionContext(json.RawMessage(`{}`)),
		factories.WithChangesRequestedAt(time.Now().UTC()),
	)
	if err != nil {
		t.Fatalf("create change: %v", err)
	}
	release, err := factories.CreateRelease(t.Context(), exec, environment.ID, nil, nil, change.ID)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}
	deployment, err := factories.CreateDeployment(
		t.Context(),
		exec,
		change.ID,
		release.ID,
		target.ID,
		factories.WithDeploymentsStrategy(json.RawMessage(`{"type":"blue_green"}`)),
		factories.WithDeploymentsRuntimeConfiguration(json.RawMessage(`{"phase":"queued"}`)),
		factories.WithDeploymentsStatus("in_progress"),
	)
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	return deployment
}

func assertCheckpoint(t *testing.T, content json.RawMessage, want models.SystemUpdateCheckpoint) {
	t.Helper()

	var got models.SystemUpdateCheckpoint
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatalf("decode persisted checkpoint: %v", err)
	}
	if got != want {
		t.Errorf("checkpoint = %+v, want %+v", got, want)
	}
}
