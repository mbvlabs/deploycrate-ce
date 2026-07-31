package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrGitHubWebhookSignature = errors.New("GitHub webhook signature is invalid")

type GitHubWebhook struct {
	db         storage.Pool
	queue      storage.InsertQueue
	connection *GitHubConnection
}

func NewGitHubWebhook(db storage.Pool, queue storage.InsertQueue, connection *GitHubConnection) *GitHubWebhook {
	return &GitHubWebhook{db: db, queue: queue, connection: connection}
}

type githubWebhookPayload struct {
	Action       string `json:"action"`
	Ref          string `json:"ref"`
	After        string `json:"after"`
	Deleted      bool   `json:"deleted"`
	Installation *struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Repository *struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func (service *GitHubWebhook) Process(ctx context.Context, deliveryID, event, signature string, body []byte) error {
	deliveryID = strings.TrimSpace(deliveryID)
	event = strings.TrimSpace(event)
	if deliveryID == "" || event == "" || signature == "" {
		return errors.New("required GitHub webhook headers are missing")
	}
	if len(deliveryID) > 255 || len(event) > 100 {
		return errors.New("GitHub webhook headers exceed the allowed size")
	}
	instanceID, err := service.connection.instanceID()
	if err != nil {
		return err
	}
	app, err := models.GitHubApp.ActiveByInstance(ctx, service.db.Executor(), instanceID)
	if err != nil {
		return ErrGitHubNotConfigured
	}
	secret, err := service.connection.webhookSecret(ctx, app)
	if err != nil {
		return err
	}
	if !validGitHubSignature(body, signature, secret) {
		return ErrGitHubWebhookSignature
	}

	var payload githubWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return errors.New("GitHub webhook payload is invalid")
	}
	installationExternalID := sql.NullInt64{}
	if payload.Installation != nil && payload.Installation.ID > 0 {
		installationExternalID = sql.NullInt64{Int64: payload.Installation.ID, Valid: true}
	}
	repositoryExternalID := sql.NullInt64{}
	if payload.Repository != nil && payload.Repository.ID > 0 {
		repositoryExternalID = sql.NullInt64{Int64: payload.Repository.ID, Valid: true}
	}
	sanitized, _ := json.Marshal(map[string]any{
		"schema_version":  1,
		"action":          payload.Action,
		"installation_id": nullableInt64(installationExternalID),
		"repository_id":   nullableInt64(repositoryExternalID),
		"repository":      repositoryFullName(payload),
		"ref":             payload.Ref,
		"after":           payload.After,
		"deleted":         payload.Deleted,
	})
	digest := sha256.Sum256(body)
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "github-delivery:"+deliveryID); err != nil {
		return err
	}
	delivery, duplicate, err := models.GitHubWebhookDelivery.CreateOrFind(ctx, tx, models.CreateGitHubWebhookDeliveryData{DeliveryID: deliveryID, Event: event, Action: sql.NullString{String: payload.Action, Valid: payload.Action != ""}, InstallationExternalID: installationExternalID, RepositoryExternalID: repositoryExternalID, BodyDigest: digest[:], Payload: sanitized, ReceivedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if duplicate && (delivery.Status == models.GitHubDeliveryProcessed || delivery.Status == models.GitHubDeliveryIgnored || delivery.Status == models.GitHubDeliveryProcessing) {
		return nil
	}

	var processErr error
	switch event {
	case "push":
		processErr = service.processPush(ctx, delivery, payload, sanitized)
	case "installation":
		processErr = service.processInstallationLifecycle(ctx, delivery, payload)
	case "installation_repositories":
		if !installationExternalID.Valid {
			processErr = service.markIgnored(ctx, delivery.ID)
		} else {
			_, processErr = service.connection.synchronizeExternal(ctx, installationExternalID.Int64)
			if processErr == nil {
				processErr = models.GitHubWebhookDelivery.Mark(ctx, service.db.Executor(), delivery.ID, models.GitHubDeliveryProcessed, nil)
			}
		}
	default:
		processErr = service.markIgnored(ctx, delivery.ID)
	}
	if processErr != nil {
		_ = models.GitHubWebhookDelivery.Mark(ctx, service.db.Executor(), delivery.ID, models.GitHubDeliveryFailed, processErr)
	}
	return processErr
}

func (service *GitHubWebhook) processInstallationLifecycle(ctx context.Context, delivery models.GitHubWebhookDeliveryEntity, payload githubWebhookPayload) error {
	if payload.Installation == nil || payload.Installation.ID <= 0 {
		return service.markIgnored(ctx, delivery.ID)
	}
	now := time.Now().UTC()
	switch payload.Action {
	case "created", "unsuspended", "new_permissions_accepted":
		if _, err := service.connection.synchronizeExternal(ctx, payload.Installation.ID); err != nil {
			return err
		}
	case "deleted":
		if _, err := service.db.Executor().NewUpdate().TableExpr("github_installations").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("external_id = ?", payload.Installation.ID).Exec(ctx); err != nil {
			return err
		}
	case "suspended":
		if _, err := service.db.Executor().NewUpdate().TableExpr("github_installations").Set("suspended_at = ?", now).Set("updated_at = ?", now).Where("external_id = ? AND archived_at IS NULL", payload.Installation.ID).Exec(ctx); err != nil {
			return err
		}
	default:
		return service.markIgnored(ctx, delivery.ID)
	}
	return models.GitHubWebhookDelivery.Mark(ctx, service.db.Executor(), delivery.ID, models.GitHubDeliveryProcessed, nil)
}

func (service *GitHubWebhook) processPush(ctx context.Context, delivery models.GitHubWebhookDeliveryEntity, payload githubWebhookPayload, sanitized json.RawMessage) error {
	if payload.Installation == nil || payload.Repository == nil || payload.Installation.ID <= 0 || payload.Repository.ID <= 0 || payload.Deleted || strings.Trim(payload.After, "0") == "" {
		return service.markIgnored(ctx, delivery.ID)
	}
	reference := normalizeGitReference(payload.Ref)
	if reference == "" || strings.TrimSpace(payload.After) == "" {
		return service.markIgnored(ctx, delivery.ID)
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	locked, err := models.GitHubWebhookDelivery.Lock(ctx, tx, delivery.ID)
	if err != nil {
		return err
	}
	if locked.Status == models.GitHubDeliveryProcessed || locked.Status == models.GitHubDeliveryIgnored {
		return tx.Commit()
	}
	if err := models.GitHubWebhookDelivery.Mark(ctx, tx, delivery.ID, models.GitHubDeliveryProcessing, nil); err != nil {
		return err
	}
	matches, err := models.GitHubEnvironmentSource.MatchingActive(ctx, tx, payload.Installation.ID, payload.Repository.ID, reference)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		if err := models.GitHubWebhookDelivery.Mark(ctx, tx, delivery.ID, models.GitHubDeliveryIgnored, nil); err != nil {
			return err
		}
		return tx.Commit()
	}

	now := time.Now().UTC()
	for _, source := range matches {
		stateRevision, err := models.EnvironmentStateRevision.LatestCommitted(ctx, tx, source.EnvironmentID)
		if err != nil {
			return fmt.Errorf("load current Environment state: %w", err)
		}
		state, err := models.ParseEnvironmentDesiredState(stateRevision.State)
		if err != nil {
			return fmt.Errorf("parse current Environment state: %w", err)
		}
		event, err := models.SourceEvent.Create(ctx, tx, models.CreateSourceEventData{ExternalID: delivery.DeliveryID, Kind: "github_push", SourceRevision: sql.NullString{String: payload.After, Valid: true}, Payload: sanitized, ReceivedAt: delivery.ReceivedAt, ProcessedAt: sql.NullTime{Time: now, Valid: true}, EnvironmentSourceID: source.EnvironmentSourceID})
		if err != nil {
			return fmt.Errorf("create source event: %w", err)
		}
		sequence, err := models.Change.NextSequence(ctx, tx, source.EnvironmentID)
		if err != nil {
			return err
		}
		change, err := models.Change.Create(ctx, tx, models.CreateChangeData{Sequence: sequence, Kind: "build", TriggerType: "webhook", ActorType: "system", CauseSystem: sql.NullString{String: "github", Valid: true}, CauseReference: sql.NullString{String: delivery.DeliveryID, Valid: true}, CorrelationID: delivery.ID, Summary: "Build " + source.RepositoryFullName + " at " + shortRevision(payload.After), Status: "committed", RequestedAt: now, CommittedAt: sql.NullTime{Time: now, Valid: true}, EnvironmentID: source.EnvironmentID})
		if err != nil {
			return fmt.Errorf("create build change: %w", err)
		}
		requestedValue, _ := json.Marshal(map[string]any{"source_event_id": event.ID, "revision": payload.After, "reference": reference})
		if _, err := models.ChangeItem.Create(ctx, tx, models.CreateChangeItemData{Action: "build", SubjectType: "environment_source", SubjectID: source.EnvironmentSourceID, RequestedValue: requestedValue, ChangeID: change.ID}); err != nil {
			return fmt.Errorf("create build change item: %w", err)
		}
		buildConfiguration, err := marshalBuildSnapshot(buildSnapshot{
			SchemaVersion: 1, SourceEventID: event.ID, EnvironmentStateRevisionID: stateRevision.ID,
			Repository: source.RepositoryFullName, Reference: source.Reference, SourceRevision: payload.After,
			ContextPath: source.ContextPath, BuilderReference: nullableStringPointer(source.BuilderReference),
			ImageRepository: source.ImageRepository, RegistryResourceID: source.RegistryResourceID,
			RegistryCredentialID: source.RegistryCredentialID,
			RegistryEndpoint:     source.RegistryEndpoint, Settings: source.BuildpackSettings,
			BPGOTargets: state.Runtime.BPGOTargets,
		})
		if err != nil {
			return fmt.Errorf("create Build configuration snapshot: %w", err)
		}
		build, err := models.Build.Create(ctx, tx, models.CreateBuildData{
			SourceRevision: payload.After, BuildMethod: "buildpacks", BuildConfiguration: buildConfiguration,
			Status: "pending", CurrentStep: sql.NullString{String: "queued", Valid: true},
			EnvironmentID: source.EnvironmentID, EnvironmentSourceID: source.EnvironmentSourceID, ChangeID: change.ID,
		})
		if err != nil {
			return fmt.Errorf("create pending build: %w", err)
		}
		if _, err := service.queue.InsertTx(ctx, tx.Tx, jobs.BuildSourceArgs{BuildID: build.ID}, jobs.BuildSourceInsertOpts(build.ID)); err != nil {
			return fmt.Errorf("insert build source job: %w", err)
		}
	}
	if err := models.GitHubWebhookDelivery.Mark(ctx, tx, delivery.ID, models.GitHubDeliveryProcessed, nil); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *GitHubWebhook) markIgnored(ctx context.Context, deliveryID uuid.UUID) error {
	return models.GitHubWebhookDelivery.Mark(ctx, service.db.Executor(), deliveryID, models.GitHubDeliveryIgnored, nil)
}

func validGitHubSignature(body []byte, header, secret string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) || len(header) != len(prefix)+sha256.Size*2 {
		return false
	}
	received, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(received, mac.Sum(nil))
}

func nullableInt64(value sql.NullInt64) any {
	if value.Valid {
		return value.Int64
	}
	return nil
}

func repositoryFullName(payload githubWebhookPayload) any {
	if payload.Repository != nil {
		return payload.Repository.FullName
	}
	return nil
}

func shortRevision(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}
