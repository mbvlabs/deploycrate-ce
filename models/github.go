package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	GitHubCredentialProvider          = "github_app"
	GitHubSetupManifestRegistration   = "manifest_registration"
	GitHubSetupInstallation           = "installation"
	GitHubDeliveryReceived            = "received"
	GitHubDeliveryProcessing          = "processing"
	GitHubDeliveryProcessed           = "processed"
	GitHubDeliveryIgnored             = "ignored"
	GitHubDeliveryFailed              = "failed"
	GitHubRepositorySelectionAll      = "all"
	GitHubRepositorySelectionSelected = "selected"
)

type (
	githubAppSetupAttempt   struct{}
	githubApp               struct{}
	githubInstallation      struct{}
	githubRepository        struct{}
	githubEnvironmentSource struct{}
	githubWebhookDelivery   struct{}
)

var (
	GitHubAppSetupAttempt   githubAppSetupAttempt
	GitHubApp               githubApp
	GitHubInstallation      githubInstallation
	GitHubRepository        githubRepository
	GitHubEnvironmentSource githubEnvironmentSource
	GitHubWebhookDelivery   githubWebhookDelivery
)

func (e *GitHubAppSetupAttemptEntity) Validate() error {
	b := validation.NewBuilder()
	if e.ID == uuid.Nil || e.InstanceID == uuid.Nil || e.UserID == uuid.Nil {
		b.Add("id", "required", "setup attempt identity is required")
	}
	b.OneOf("purpose", e.Purpose, GitHubSetupManifestRegistration, GitHubSetupInstallation)
	if len(e.StateDigest) != 32 || strings.TrimSpace(e.StatePrefix) == "" {
		b.Add("state", "invalid", "setup state is invalid")
	}
	if e.ExpiresAt.IsZero() || !e.ExpiresAt.After(e.CreatedAt) {
		b.Add("expires_at", "invalid", "setup attempt expiry must be after creation")
	}
	if e.CompletedAt.Valid && e.CompletedAt.Time.Before(e.CreatedAt) {
		b.Add("completed_at", "invalid", "completion cannot predate creation")
	}
	return b.Err()
}

type CreateGitHubAppSetupAttemptData struct {
	InstanceID  uuid.UUID
	UserID      uuid.UUID
	Purpose     string
	StatePrefix string
	StateDigest []byte
	OwnerType   sql.NullString
	OwnerLogin  sql.NullString
	ExpiresAt   time.Time
}

func (githubAppSetupAttempt) Create(ctx context.Context, db storage.Executor, data CreateGitHubAppSetupAttemptData) (GitHubAppSetupAttemptEntity, error) {
	now := time.Now().UTC()
	entity := GitHubAppSetupAttemptEntity{ID: uuid.New(), CreatedAt: now, UpdatedAt: now, InstanceID: data.InstanceID, UserID: data.UserID, Purpose: data.Purpose, StatePrefix: data.StatePrefix, StateDigest: data.StateDigest, OwnerType: data.OwnerType, OwnerLogin: data.OwnerLogin, ExpiresAt: data.ExpiresAt.UTC()}
	if err := validation.Validate(&entity); err != nil {
		return entity, errors.Join(ErrDomainValidation, err)
	}
	_, err := db.NewInsert().Model(&entity).Exec(ctx)
	return entity, err
}

func (githubAppSetupAttempt) LockUsable(ctx context.Context, db storage.Executor, digest []byte, purpose string, instanceID, userID uuid.UUID) (GitHubAppSetupAttemptEntity, error) {
	var entity GitHubAppSetupAttemptEntity
	err := db.NewSelect().Model(&entity).
		Where("state_digest = ?", digest).
		Where("purpose = ?", purpose).
		Where("instance_id = ?", instanceID).
		Where("user_id = ?", userID).
		Where("completed_at IS NULL").
		Where("expires_at > ?", time.Now().UTC()).
		For("UPDATE").Scan(ctx)
	return entity, err
}

func (githubAppSetupAttempt) MarkCompleted(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	now := time.Now().UTC()
	_, err := db.NewUpdate().TableExpr("github_app_setup_attempts").Set("completed_at = ?", now).Set("updated_at = ?", now).Set("error = NULL").Where("id = ? AND completed_at IS NULL", id).Exec(ctx)
	return err
}

func (githubAppSetupAttempt) MarkFailed(ctx context.Context, db storage.Executor, id uuid.UUID, operationErr error) error {
	now := time.Now().UTC()
	message := "setup failed"
	if operationErr != nil {
		message = operationErr.Error()
	}
	_, err := db.NewUpdate().TableExpr("github_app_setup_attempts").Set("error = ?", message).Set("updated_at = ?", now).Where("id = ? AND completed_at IS NULL", id).Exec(ctx)
	return err
}

func (e *GitHubAppEntity) Validate() error {
	b := validation.NewBuilder()
	if e.ID == uuid.Nil || e.CredentialID == uuid.Nil || e.InstanceID == uuid.Nil || e.ExternalID <= 0 || e.OwnerID <= 0 {
		b.Add("id", "required", "GitHub App identity is required")
	}
	for field, value := range map[string]string{"client_id": e.ClientID, "slug": e.Slug, "name": e.Name, "owner_login": e.OwnerLogin, "html_url": e.HTMLURL} {
		if strings.TrimSpace(value) == "" {
			b.Add(field, "required", "field is required")
		}
	}
	b.OneOf("owner_type", e.OwnerType, "User", "Organization")
	if !validJSONObjects(e.Permissions) || !validJSONStringArray(e.Events) {
		b.Add("permissions", "invalid", "GitHub App permissions and events are invalid")
	}
	return b.Err()
}

type CreateGitHubAppData struct {
	CredentialID, InstanceID uuid.UUID
	ExternalID, OwnerID      int64
	ClientID, Slug, Name     string
	OwnerLogin, OwnerType    string
	HTMLURL                  string
	Permissions, Events      json.RawMessage
	VerifiedAt               sql.NullTime
}

func (githubApp) Create(ctx context.Context, db storage.Executor, data CreateGitHubAppData) (GitHubAppEntity, error) {
	now := time.Now().UTC()
	entity := GitHubAppEntity{ID: uuid.New(), CreatedAt: now, UpdatedAt: now, CredentialID: data.CredentialID, InstanceID: data.InstanceID, ExternalID: data.ExternalID, ClientID: data.ClientID, Slug: data.Slug, Name: data.Name, OwnerID: data.OwnerID, OwnerLogin: data.OwnerLogin, OwnerType: data.OwnerType, HTMLURL: data.HTMLURL, Permissions: data.Permissions, Events: data.Events, VerifiedAt: data.VerifiedAt}
	if err := validation.Validate(&entity); err != nil {
		return entity, errors.Join(ErrDomainValidation, err)
	}
	_, err := db.NewInsert().Model(&entity).Exec(ctx)
	return entity, err
}

func (githubApp) ActiveByInstance(ctx context.Context, db storage.Executor, instanceID uuid.UUID) (GitHubAppEntity, error) {
	var entity GitHubAppEntity
	err := db.NewSelect().Model(&entity).Where("instance_id = ?", instanceID).Where("archived_at IS NULL").Scan(ctx)
	return entity, err
}

func (githubApp) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (GitHubAppEntity, error) {
	var entity GitHubAppEntity
	err := db.NewSelect().Model(&entity).Where("id = ?", id).Scan(ctx)
	return entity, err
}

func (githubApp) Archive(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	now := time.Now().UTC()
	_, err := db.NewUpdate().TableExpr("github_apps").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("id = ? AND archived_at IS NULL", id).Exec(ctx)
	return err
}

func (githubApp) ActiveDependencyCount(ctx context.Context, db storage.Executor, id uuid.UUID) (int, error) {
	installationCount, err := db.NewSelect().TableExpr("github_installations").Where("github_app_id = ?", id).Where("archived_at IS NULL").Count(ctx)
	if err != nil {
		return 0, err
	}
	sourceCount, err := db.NewSelect().TableExpr("github_environment_sources AS binding").
		Join("JOIN github_repositories AS repository ON repository.id = binding.github_repository_id").
		Join("JOIN github_installations AS installation ON installation.id = repository.github_installation_id").
		Join("JOIN environment_sources AS source ON source.id = binding.environment_source_id").
		Join("JOIN environments AS environment ON environment.id = source.environment_id").
		Where("installation.github_app_id = ?", id).Where("source.archived_at IS NULL").Count(ctx)
	if err != nil {
		return 0, err
	}
	return installationCount + sourceCount, nil
}

func (e *GitHubInstallationEntity) Validate() error {
	b := validation.NewBuilder()
	if e.ID == uuid.Nil || e.GitHubAppID == uuid.Nil || e.ExternalID <= 0 || e.AccountID <= 0 || strings.TrimSpace(e.AccountLogin) == "" {
		b.Add("id", "required", "GitHub installation identity is required")
	}
	b.OneOf("account_type", e.AccountType, "User", "Organization")
	b.OneOf("repository_selection", e.RepositorySelection, GitHubRepositorySelectionAll, GitHubRepositorySelectionSelected)
	if !validJSONObjects(e.Permissions) || !validJSONStringArray(e.Events) {
		b.Add("permissions", "invalid", "GitHub installation permissions and events are invalid")
	}
	return b.Err()
}

type UpsertGitHubInstallationData struct {
	GitHubAppID               uuid.UUID
	ExternalID, AccountID     int64
	AccountLogin, AccountType string
	RepositorySelection       string
	Permissions, Events       json.RawMessage
	SuspendedAt               sql.NullTime
	LastSyncedAt              sql.NullTime
}

func (githubInstallation) Upsert(ctx context.Context, db storage.Executor, data UpsertGitHubInstallationData) (GitHubInstallationEntity, error) {
	now := time.Now().UTC()
	entity := GitHubInstallationEntity{ID: uuid.New(), CreatedAt: now, UpdatedAt: now, GitHubAppID: data.GitHubAppID, ExternalID: data.ExternalID, AccountID: data.AccountID, AccountLogin: data.AccountLogin, AccountType: data.AccountType, RepositorySelection: data.RepositorySelection, Permissions: data.Permissions, Events: data.Events, SuspendedAt: data.SuspendedAt, LastSyncedAt: data.LastSyncedAt}
	if err := validation.Validate(&entity); err != nil {
		return entity, errors.Join(ErrDomainValidation, err)
	}
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "github-installation:"+strconv.FormatInt(data.ExternalID, 10)); err != nil {
		return entity, err
	}
	var existing GitHubInstallationEntity
	err := db.NewSelect().Model(&existing).Where("external_id = ?", data.ExternalID).For("UPDATE").Scan(ctx)
	if err == nil {
		entity.ID = existing.ID
		entity.CreatedAt = existing.CreatedAt
		err = db.NewUpdate().Model(&entity).
			Column("updated_at", "archived_at", "github_app_id", "external_id", "account_id", "account_login", "account_type", "repository_selection", "permissions", "events", "suspended_at", "last_synced_at").
			WherePK().Returning("*").Scan(ctx)
		return entity, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return entity, err
	}
	err = db.NewInsert().Model(&entity).Returning("*").Scan(ctx)
	return entity, err
}

func (githubInstallation) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (GitHubInstallationEntity, error) {
	var entity GitHubInstallationEntity
	err := db.NewSelect().Model(&entity).Where("id = ?", id).Scan(ctx)
	return entity, err
}

func (githubInstallation) FindByExternal(ctx context.Context, db storage.Executor, externalID int64) (GitHubInstallationEntity, error) {
	var entity GitHubInstallationEntity
	err := db.NewSelect().Model(&entity).Where("external_id = ?", externalID).Scan(ctx)
	return entity, err
}

func (githubInstallation) ListForApp(ctx context.Context, db storage.Executor, appID uuid.UUID) ([]GitHubInstallationEntity, error) {
	var entities []GitHubInstallationEntity
	err := db.NewSelect().Model(&entities).Where("github_app_id = ?", appID).OrderExpr("account_login ASC").Scan(ctx)
	return entities, err
}

func (githubInstallation) Archive(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	now := time.Now().UTC()
	_, err := db.NewUpdate().TableExpr("github_installations").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("id = ? AND archived_at IS NULL", id).Exec(ctx)
	return err
}

func (githubInstallation) ActiveSourceCount(ctx context.Context, db storage.Executor, id uuid.UUID) (int, error) {
	return db.NewSelect().TableExpr("github_environment_sources AS binding").Join("JOIN github_repositories AS repository ON repository.id = binding.github_repository_id").Join("JOIN environment_sources AS source ON source.id = binding.environment_source_id").Where("repository.github_installation_id = ?", id).Where("source.archived_at IS NULL").Count(ctx)
}

type UpsertGitHubRepositoryData struct {
	GitHubInstallationID uuid.UUID
	ExternalID           int64
	NodeID, OwnerLogin   string
	Name, FullName       string
	DefaultBranch        string
	Visibility, HTMLURL  string
	LastSyncedAt         time.Time
}

func (githubRepository) Upsert(ctx context.Context, db storage.Executor, data UpsertGitHubRepositoryData) (GitHubRepositoryEntity, error) {
	now := time.Now().UTC()
	entity := GitHubRepositoryEntity{ID: uuid.New(), CreatedAt: now, UpdatedAt: now, GitHubInstallationID: data.GitHubInstallationID, ExternalID: data.ExternalID, NodeID: data.NodeID, OwnerLogin: data.OwnerLogin, Name: data.Name, FullName: data.FullName, DefaultBranch: data.DefaultBranch, Visibility: data.Visibility, HTMLURL: data.HTMLURL, LastSyncedAt: data.LastSyncedAt.UTC()}
	if entity.ExternalID <= 0 || entity.GitHubInstallationID == uuid.Nil || strings.TrimSpace(entity.FullName) == "" {
		return entity, errors.Join(ErrDomainValidation, errors.New("GitHub repository identity is required"))
	}
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "github-repository:"+strconv.FormatInt(data.ExternalID, 10)); err != nil {
		return entity, err
	}
	var existing GitHubRepositoryEntity
	err := db.NewSelect().Model(&existing).Where("external_id = ?", data.ExternalID).For("UPDATE").Scan(ctx)
	if err == nil {
		entity.ID = existing.ID
		entity.CreatedAt = existing.CreatedAt
		err = db.NewUpdate().Model(&entity).
			Column("updated_at", "removed_at", "github_installation_id", "external_id", "node_id", "owner_login", "name", "full_name", "default_branch", "visibility", "html_url", "last_synced_at").
			WherePK().Returning("*").Scan(ctx)
		return entity, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return entity, err
	}
	err = db.NewInsert().Model(&entity).Returning("*").Scan(ctx)
	return entity, err
}

func (githubRepository) MarkMissingRemoved(ctx context.Context, db storage.Executor, installationID uuid.UUID, externalIDs []int64, at time.Time) error {
	query := db.NewUpdate().TableExpr("github_repositories").Set("removed_at = ?", at).Set("updated_at = ?", at).Where("github_installation_id = ?", installationID).Where("removed_at IS NULL")
	if len(externalIDs) > 0 {
		query = query.Where("external_id NOT IN (?)", bun.In(externalIDs))
	}
	_, err := query.Exec(ctx)
	return err
}

func (githubRepository) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (GitHubRepositoryEntity, error) {
	var entity GitHubRepositoryEntity
	err := db.NewSelect().Model(&entity).Where("id = ?", id).Scan(ctx)
	return entity, err
}

func (githubRepository) FindByExternal(ctx context.Context, db storage.Executor, externalID int64) (GitHubRepositoryEntity, error) {
	var entity GitHubRepositoryEntity
	err := db.NewSelect().Model(&entity).Where("external_id = ?", externalID).Scan(ctx)
	return entity, err
}

func (githubRepository) ListActive(ctx context.Context, db storage.Executor, installationID uuid.UUID) ([]GitHubRepositoryEntity, error) {
	var entities []GitHubRepositoryEntity
	err := db.NewSelect().Model(&entities).Where("github_installation_id = ?", installationID).Where("removed_at IS NULL").OrderExpr("full_name ASC").Scan(ctx)
	return entities, err
}

func (githubEnvironmentSource) Create(ctx context.Context, db storage.Executor, sourceID, repositoryID uuid.UUID) (GitHubEnvironmentSourceEntity, error) {
	now := time.Now().UTC()
	entity := GitHubEnvironmentSourceEntity{EnvironmentSourceID: sourceID, GitHubRepositoryID: repositoryID, CreatedAt: now, UpdatedAt: now}
	_, err := db.NewInsert().Model(&entity).Exec(ctx)
	return entity, err
}

type GitHubMatchingSource struct {
	EnvironmentSourceID  uuid.UUID       `bun:"environment_source_id"`
	EnvironmentID        uuid.UUID       `bun:"environment_id"`
	Reference            string          `bun:"reference"`
	AutoBuild            bool            `bun:"auto_build"`
	RepositoryFullName   string          `bun:"repository_full_name"`
	ContextPath          string          `bun:"context_path"`
	BuilderReference     sql.NullString  `bun:"builder_reference"`
	BuildpackSettings    json.RawMessage `bun:"buildpack_settings,type:jsonb"`
	ImageRepository      string          `bun:"image_repository"`
	RegistryResourceID   uuid.UUID       `bun:"registry_resource_id"`
	RegistryCredentialID uuid.UUID       `bun:"registry_credential_id"`
	RegistryEndpoint     string          `bun:"registry_endpoint"`
	BuildServerID        uuid.UUID       `bun:"build_server_id"`
}

func (githubEnvironmentSource) MatchingActive(ctx context.Context, db storage.Executor, installationExternalID, repositoryExternalID int64, reference string) ([]GitHubMatchingSource, error) {
	var entities []GitHubMatchingSource
	err := db.NewSelect().TableExpr("github_environment_sources AS binding").
		ColumnExpr("source.id AS environment_source_id").ColumnExpr("source.environment_id").ColumnExpr("source.reference").ColumnExpr("source.auto_build").ColumnExpr("repository.full_name AS repository_full_name").
		ColumnExpr("buildpack.context_path, buildpack.builder_reference, buildpack.settings AS buildpack_settings, buildpack.image_repository, buildpack.server_id AS build_server_id").
		ColumnExpr("registry_resource.id AS registry_resource_id, registry_credential.id AS registry_credential_id").
		ColumnExpr("CASE WHEN registry_resource.system_managed THEN COALESCE(NULLIF(registry.configuration ->> 'route_host', ''), registry_endpoint.address) WHEN registry_endpoint.port IN (80, 443) THEN registry_endpoint.address ELSE registry_endpoint.address || ':' || registry_endpoint.port::text END AS registry_endpoint").
		Join("JOIN environment_sources AS source ON source.id = binding.environment_source_id").
		Join("JOIN environments AS environment ON environment.id = source.environment_id").
		Join("JOIN github_repositories AS repository ON repository.id = binding.github_repository_id").
		Join("JOIN github_installations AS installation ON installation.id = repository.github_installation_id").
		Join("JOIN github_apps AS app ON app.id = installation.github_app_id").
		Join("JOIN buildpack_configurations AS buildpack ON buildpack.environment_source_id = source.id").
		Join("JOIN registry_resources AS registry ON registry.resource_id = buildpack.registry_resource_id").
		Join("JOIN resources AS registry_resource ON registry_resource.id = registry.resource_id AND registry_resource.archived_at IS NULL").
		Join("JOIN resource_endpoints AS registry_endpoint ON registry_endpoint.resource_id = registry.resource_id AND registry_endpoint.role = 'primary' AND registry_endpoint.archived_at IS NULL").
		Join("JOIN resource_credentials AS registry_credential ON registry_credential.resource_id = registry.resource_id AND registry_credential.archived_at IS NULL").
		Where("installation.external_id = ?", installationExternalID).Where("repository.external_id = ?", repositoryExternalID).
		Where("source.archived_at IS NULL").Where("environment.archived_at IS NULL").
		Where(`EXISTS (
			SELECT 1 FROM changes AS setup_change
			JOIN change_state_revisions AS setup_result ON setup_result.change_id = setup_change.id AND setup_result.role = 'result'
			JOIN environment_state_revisions AS setup_revision ON setup_revision.id = setup_result.environment_state_revision_id AND setup_revision.environment_id = environment.id
			WHERE setup_change.environment_id = environment.id AND setup_change.kind = 'environment_setup'
			AND setup_change.committed_at IS NOT NULL AND setup_change.cancelled_at IS NULL
		)`).Where("source.auto_build = TRUE").Where("source.reference = ?", reference).
		Where("repository.removed_at IS NULL").Where("installation.archived_at IS NULL").Where("installation.suspended_at IS NULL").Where("app.archived_at IS NULL").Scan(ctx, &entities)
	return entities, err
}

type CreateGitHubWebhookDeliveryData struct {
	DeliveryID, Event                            string
	Action                                       sql.NullString
	InstallationExternalID, RepositoryExternalID sql.NullInt64
	BodyDigest                                   []byte
	Payload                                      json.RawMessage
	ReceivedAt                                   time.Time
}

func (githubWebhookDelivery) CreateOrFind(ctx context.Context, db storage.Executor, data CreateGitHubWebhookDeliveryData) (GitHubWebhookDeliveryEntity, bool, error) {
	var existing GitHubWebhookDeliveryEntity
	err := db.NewSelect().Model(&existing).Where("delivery_id = ?", data.DeliveryID).Scan(ctx)
	if err == nil {
		return existing, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return existing, false, err
	}
	now := time.Now().UTC()
	entity := GitHubWebhookDeliveryEntity{ID: uuid.New(), CreatedAt: now, UpdatedAt: now, DeliveryID: data.DeliveryID, Event: data.Event, Action: data.Action, InstallationExternalID: data.InstallationExternalID, RepositoryExternalID: data.RepositoryExternalID, BodyDigest: data.BodyDigest, Payload: data.Payload, ReceivedAt: data.ReceivedAt, Status: GitHubDeliveryReceived}
	_, err = db.NewInsert().Model(&entity).Exec(ctx)
	return entity, false, err
}

func (githubWebhookDelivery) Lock(ctx context.Context, db storage.Executor, id uuid.UUID) (GitHubWebhookDeliveryEntity, error) {
	var entity GitHubWebhookDeliveryEntity
	err := db.NewSelect().Model(&entity).Where("id = ?", id).For("UPDATE").Scan(ctx)
	return entity, err
}

func (githubWebhookDelivery) Mark(ctx context.Context, db storage.Executor, id uuid.UUID, status string, operationErr error) error {
	now := time.Now().UTC()
	query := db.NewUpdate().TableExpr("github_webhook_deliveries").Set("status = ?", status).Set("updated_at = ?", now)
	if status == GitHubDeliveryProcessed || status == GitHubDeliveryIgnored {
		query = query.Set("processed_at = ?", now).Set("error = NULL")
	} else if operationErr != nil {
		query = query.Set("error = ?", operationErr.Error())
	}
	_, err := query.Where("id = ?", id).Exec(ctx)
	return err
}

func validJSONObjects(value json.RawMessage) bool {
	var decoded map[string]string
	return json.Unmarshal(value, &decoded) == nil
}

func validJSONStringArray(value json.RawMessage) bool {
	var decoded []string
	return json.Unmarshal(value, &decoded) == nil
}
