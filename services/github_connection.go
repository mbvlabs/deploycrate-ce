package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	githubclient "deploycrate-ce/clients/github"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/server"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/router/routes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

const githubCredentialPurpose = "github-app-credential"

var (
	ErrGitHubAlreadyConfigured = errors.New("a GitHub App is already connected")
	ErrGitHubNotConfigured     = errors.New("GitHub is not connected")
	ErrGitHubSetupState        = errors.New("GitHub setup state is invalid, expired, or already used")
	ErrGitHubDependencies      = errors.New("GitHub connection has active application dependencies")
	ErrGitHubProductionOrigin  = errors.New("GitHub App setup requires a public HTTPS origin in production")
)

type GitHubCredentialMetadata struct {
	SchemaVersion  int    `json:"schema_version"`
	InstanceID     string `json:"instance_id"`
	CredentialKind string `json:"credential_kind"`
}

type GitHubCredentialPayload struct {
	SchemaVersion int    `json:"schema_version"`
	PrivateKeyPEM string `json:"private_key_pem"`
	WebhookSecret string `json:"webhook_secret"`
	ClientSecret  string `json:"client_secret"`
}

type GitHubManifestStart struct {
	Action   string `json:"action"`
	State    string `json:"state"`
	Manifest string `json:"manifest"`
}

type GitHubInstallationSummary struct {
	models.GitHubInstallationEntity
	RepositoryCount int `json:"repositoryCount"`
}

type GitHubConnectionState struct {
	App           *models.GitHubAppEntity     `json:"app"`
	Installations []GitHubInstallationSummary `json:"installations"`
	Degraded      bool                        `json:"degraded"`
	HealthMessage string                      `json:"healthMessage"`
}

type GitHubConnection struct {
	db     storage.Pool
	cfg    config.Config
	client *githubclient.Client
}

func NewGitHubConnection(db storage.Pool, cfg config.Config, client *githubclient.Client) *GitHubConnection {
	return &GitHubConnection{db: db, cfg: cfg, client: client}
}

func (service *GitHubConnection) StartManifest(ctx context.Context, userID uuid.UUID, ownerType, ownerLogin string) (GitHubManifestStart, error) {
	instanceID, err := service.instanceID()
	if err != nil {
		return GitHubManifestStart{}, err
	}
	if err := validateGitHubOrigin(); err != nil {
		return GitHubManifestStart{}, err
	}
	if _, err := models.GitHubApp.ActiveByInstance(ctx, service.db.Executor(), instanceID); err == nil {
		return GitHubManifestStart{}, ErrGitHubAlreadyConfigured
	} else if !errors.Is(err, sql.ErrNoRows) {
		return GitHubManifestStart{}, err
	}

	ownerType = strings.ToLower(strings.TrimSpace(ownerType))
	ownerLogin = strings.TrimSpace(ownerLogin)
	var action string
	var storedType string
	switch ownerType {
	case "personal", "user":
		action = "https://github.com/settings/apps/new"
		storedType = "User"
	case "organization":
		if ownerLogin == "" || strings.ContainsAny(ownerLogin, "/?#") {
			return GitHubManifestStart{}, errors.Join(models.ErrDomainValidation, errors.New("organization login is required"))
		}
		action = "https://github.com/organizations/" + url.PathEscape(ownerLogin) + "/settings/apps/new"
		storedType = "Organization"
	default:
		return GitHubManifestStart{}, errors.Join(models.ErrDomainValidation, errors.New("owner type must be personal or organization"))
	}

	state, digest, err := service.newState()
	if err != nil {
		return GitHubManifestStart{}, err
	}
	ownerLoginValue := sql.NullString{String: ownerLogin, Valid: ownerLogin != ""}
	_, err = models.GitHubAppSetupAttempt.Create(ctx, service.db.Executor(), models.CreateGitHubAppSetupAttemptData{
		InstanceID:  instanceID,
		UserID:      userID,
		Purpose:     models.GitHubSetupManifestRegistration,
		StatePrefix: state[:8],
		StateDigest: digest,
		OwnerType:   sql.NullString{String: storedType, Valid: true},
		OwnerLogin:  ownerLoginValue,
		ExpiresAt:   time.Now().UTC().Add(15 * time.Minute),
	})
	if err != nil {
		return GitHubManifestStart{}, err
	}

	shortID := strings.ReplaceAll(instanceID.String(), "-", "")[:8]
	manifestBytes, err := json.Marshal(map[string]any{
		"name":                "DeployCrate CE " + shortID,
		"url":                 config.BaseURL,
		"redirect_url":        routes.GitHubAppCallback.FullURL(config.BaseURL),
		"setup_url":           routes.GitHubInstallCallback.FullURL(config.BaseURL),
		"setup_on_update":     true,
		"public":              false,
		"hook_attributes":     map[string]any{"url": routes.GitHubWebhook.FullURL(config.BaseURL), "active": true},
		"default_permissions": map[string]string{"contents": "read", "metadata": "read"},
		"default_events":      []string{"push"},
	})
	if err != nil {
		return GitHubManifestStart{}, err
	}
	return GitHubManifestStart{Action: action + "?state=" + url.QueryEscape(state), State: state, Manifest: string(manifestBytes)}, nil
}

func (service *GitHubConnection) CompleteManifest(ctx context.Context, userID uuid.UUID, state, code string) (models.GitHubAppEntity, error) {
	instanceID, err := service.instanceID()
	if err != nil {
		return models.GitHubAppEntity{}, err
	}
	digest, err := service.stateDigest(state)
	if err != nil {
		return models.GitHubAppEntity{}, ErrGitHubSetupState
	}
	preflightAttempt, err := models.GitHubAppSetupAttempt.LockUsable(ctx, service.db.Executor(), digest, models.GitHubSetupManifestRegistration, instanceID, userID)
	if err != nil {
		return models.GitHubAppEntity{}, ErrGitHubSetupState
	}

	registration, err := service.client.ExchangeManifestCode(ctx, code)
	if err != nil {
		return models.GitHubAppEntity{}, err
	}
	auth := githubclient.AppAuthentication{AppID: registration.ID, PrivateKeyPEM: registration.PEM}
	if registration.ID <= 0 || registration.ClientID == "" || registration.ClientSecret == "" ||
		registration.WebhookSecret == "" || registration.Slug == "" || registration.Name == "" ||
		registration.Owner.ID <= 0 || registration.Owner.Login == "" || registration.HTMLURL == "" ||
		registration.Permissions["contents"] != "read" || registration.Permissions["metadata"] != "read" ||
		!containsGitHubEvent(registration.Events, "push") || githubclient.ValidateAppAuthentication(auth) != nil {
		return models.GitHubAppEntity{}, errors.New("GitHub returned an incomplete or incompatible App registration")
	}
	if !matchesGitHubOwner(preflightAttempt, registration.Owner.Type, registration.Owner.Login) {
		return models.GitHubAppEntity{}, errors.New("GitHub created the App for a different owner than the selected account")
	}

	metadata, _ := json.Marshal(GitHubCredentialMetadata{SchemaVersion: 1, InstanceID: instanceID.String(), CredentialKind: models.GitHubCredentialProvider})
	payload, _ := json.Marshal(GitHubCredentialPayload{SchemaVersion: 1, PrivateKeyPEM: registration.PEM, WebhookSecret: registration.WebhookSecret, ClientSecret: registration.ClientSecret})
	encrypted, err := secretcrypto.EncryptForPurpose(payload, service.cfg.App.SessionEncryptionKey, githubCredentialPurpose)
	if err != nil {
		return models.GitHubAppEntity{}, err
	}
	permissions, _ := json.Marshal(registration.Permissions)
	events, _ := json.Marshal(registration.Events)
	registration.PEM, registration.WebhookSecret, registration.ClientSecret = "", "", ""

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.GitHubAppEntity{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "github-app:"+instanceID.String()); err != nil {
		return models.GitHubAppEntity{}, err
	}
	attempt, err := models.GitHubAppSetupAttempt.LockUsable(ctx, tx, digest, models.GitHubSetupManifestRegistration, instanceID, userID)
	if err != nil {
		return models.GitHubAppEntity{}, ErrGitHubSetupState
	}
	if _, err := models.GitHubApp.ActiveByInstance(ctx, tx, instanceID); err == nil {
		return models.GitHubAppEntity{}, ErrGitHubAlreadyConfigured
	} else if !errors.Is(err, sql.ErrNoRows) {
		return models.GitHubAppEntity{}, err
	}
	now := time.Now().UTC()
	credential, err := models.Credential.Create(ctx, tx, models.CreateCredentialData{Name: "GitHub App " + registration.Name, Provider: models.GitHubCredentialProvider, Metadata: metadata, EncPayload: encrypted, VerifiedAt: sql.NullTime{Time: now, Valid: true}})
	if err != nil {
		return models.GitHubAppEntity{}, err
	}
	app, err := models.GitHubApp.Create(ctx, tx, models.CreateGitHubAppData{CredentialID: credential.ID, InstanceID: instanceID, ExternalID: registration.ID, ClientID: registration.ClientID, Slug: registration.Slug, Name: registration.Name, OwnerID: registration.Owner.ID, OwnerLogin: registration.Owner.Login, OwnerType: registration.Owner.Type, HTMLURL: registration.HTMLURL, Permissions: permissions, Events: events, VerifiedAt: sql.NullTime{Time: now, Valid: true}})
	if err != nil {
		return models.GitHubAppEntity{}, err
	}
	if err := models.GitHubAppSetupAttempt.MarkCompleted(ctx, tx, attempt.ID); err != nil {
		return models.GitHubAppEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.GitHubAppEntity{}, fmt.Errorf("persist GitHub App registration: %w", err)
	}
	return app, nil
}

func (service *GitHubConnection) StartInstallation(ctx context.Context, userID uuid.UUID) (string, error) {
	instanceID, err := service.instanceID()
	if err != nil {
		return "", err
	}
	app, err := models.GitHubApp.ActiveByInstance(ctx, service.db.Executor(), instanceID)
	if err != nil {
		return "", ErrGitHubNotConfigured
	}
	state, digest, err := service.newState()
	if err != nil {
		return "", err
	}
	_, err = models.GitHubAppSetupAttempt.Create(ctx, service.db.Executor(), models.CreateGitHubAppSetupAttemptData{InstanceID: instanceID, UserID: userID, Purpose: models.GitHubSetupInstallation, StatePrefix: state[:8], StateDigest: digest, ExpiresAt: time.Now().UTC().Add(15 * time.Minute)})
	if err != nil {
		return "", err
	}
	return "https://github.com/apps/" + url.PathEscape(app.Slug) + "/installations/new?state=" + url.QueryEscape(state), nil
}

func (service *GitHubConnection) CompleteInstallation(ctx context.Context, userID uuid.UUID, state string, externalID int64) (models.GitHubInstallationEntity, error) {
	instanceID, err := service.instanceID()
	if err != nil {
		return models.GitHubInstallationEntity{}, err
	}
	digest, err := service.stateDigest(state)
	if err != nil {
		return models.GitHubInstallationEntity{}, ErrGitHubSetupState
	}
	if _, err := models.GitHubAppSetupAttempt.LockUsable(ctx, service.db.Executor(), digest, models.GitHubSetupInstallation, instanceID, userID); err != nil {
		return models.GitHubInstallationEntity{}, ErrGitHubSetupState
	}
	installation, err := service.synchronizeExternal(ctx, externalID)
	if err != nil {
		return models.GitHubInstallationEntity{}, err
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.GitHubInstallationEntity{}, err
	}
	defer tx.Rollback()
	attempt, err := models.GitHubAppSetupAttempt.LockUsable(ctx, tx, digest, models.GitHubSetupInstallation, instanceID, userID)
	if err != nil {
		return models.GitHubInstallationEntity{}, ErrGitHubSetupState
	}
	if err := models.GitHubAppSetupAttempt.MarkCompleted(ctx, tx, attempt.ID); err != nil {
		return models.GitHubInstallationEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.GitHubInstallationEntity{}, err
	}
	return installation, nil
}

func (service *GitHubConnection) Synchronize(ctx context.Context, id uuid.UUID) (models.GitHubInstallationEntity, error) {
	installation, err := models.GitHubInstallation.Find(ctx, service.db.Executor(), id)
	if err != nil {
		return models.GitHubInstallationEntity{}, err
	}
	if installation.ArchivedAt.Valid {
		return models.GitHubInstallationEntity{}, errors.New("archived GitHub installations cannot be synchronized")
	}
	return service.synchronizeExternal(ctx, installation.ExternalID)
}

func (service *GitHubConnection) synchronizeExternal(ctx context.Context, externalID int64) (models.GitHubInstallationEntity, error) {
	instanceID, err := service.instanceID()
	if err != nil {
		return models.GitHubInstallationEntity{}, err
	}
	app, err := models.GitHubApp.ActiveByInstance(ctx, service.db.Executor(), instanceID)
	if err != nil {
		return models.GitHubInstallationEntity{}, ErrGitHubNotConfigured
	}
	auth, err := service.authentication(ctx, app)
	if err != nil {
		return models.GitHubInstallationEntity{}, err
	}
	providerInstallation, err := service.client.GetInstallation(ctx, auth, externalID)
	if err != nil {
		return models.GitHubInstallationEntity{}, err
	}
	if providerInstallation.AppID != app.ExternalID {
		return models.GitHubInstallationEntity{}, errors.New("GitHub installation does not belong to the active App")
	}
	providerRepositories, err := service.client.ListInstallationRepositories(ctx, auth, externalID)
	if err != nil {
		return models.GitHubInstallationEntity{}, err
	}
	permissions, _ := json.Marshal(providerInstallation.Permissions)
	events, _ := json.Marshal(providerInstallation.Events)
	now := time.Now().UTC()
	suspendedAt := sql.NullTime{}
	if providerInstallation.SuspendedAt != nil {
		suspendedAt = sql.NullTime{Time: providerInstallation.SuspendedAt.UTC(), Valid: true}
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.GitHubInstallationEntity{}, err
	}
	defer tx.Rollback()
	installation, err := models.GitHubInstallation.Upsert(ctx, tx, models.UpsertGitHubInstallationData{GitHubAppID: app.ID, ExternalID: providerInstallation.ID, AccountID: providerInstallation.Account.ID, AccountLogin: providerInstallation.Account.Login, AccountType: providerInstallation.Account.Type, RepositorySelection: providerInstallation.RepositorySelection, Permissions: permissions, Events: events, SuspendedAt: suspendedAt, LastSyncedAt: sql.NullTime{Time: now, Valid: true}})
	if err != nil {
		return models.GitHubInstallationEntity{}, err
	}
	externalIDs := make([]int64, 0, len(providerRepositories))
	for _, repository := range providerRepositories {
		visibility := repository.Visibility
		if visibility == "" && repository.Private {
			visibility = "private"
		} else if visibility == "" {
			visibility = "public"
		}
		externalIDs = append(externalIDs, repository.ID)
		if _, err := models.GitHubRepository.Upsert(ctx, tx, models.UpsertGitHubRepositoryData{GitHubInstallationID: installation.ID, ExternalID: repository.ID, NodeID: repository.NodeID, OwnerLogin: repository.Owner.Login, Name: repository.Name, FullName: repository.FullName, DefaultBranch: repository.DefaultBranch, Visibility: visibility, HTMLURL: repository.HTMLURL, LastSyncedAt: now}); err != nil {
			return models.GitHubInstallationEntity{}, err
		}
	}
	if err := models.GitHubRepository.MarkMissingRemoved(ctx, tx, installation.ID, externalIDs, now); err != nil {
		return models.GitHubInstallationEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.GitHubInstallationEntity{}, err
	}
	return installation, nil
}

func (service *GitHubConnection) Verify(ctx context.Context, id uuid.UUID) error {
	installation, err := models.GitHubInstallation.Find(ctx, service.db.Executor(), id)
	if err != nil {
		return err
	}
	app, err := models.GitHubApp.Find(ctx, service.db.Executor(), installation.GitHubAppID)
	if err != nil {
		return err
	}
	auth, err := service.authentication(ctx, app)
	if err != nil {
		return err
	}
	providerInstallation, err := service.client.GetInstallation(ctx, auth, installation.ExternalID)
	if err != nil {
		return err
	}
	if providerInstallation.AppID != app.ExternalID {
		return errors.New("GitHub installation no longer belongs to the active App")
	}
	return nil
}

func (service *GitHubConnection) RotateCredential(ctx context.Context, privateKeyPEM, webhookSecret, clientSecret string) error {
	instanceID, err := service.instanceID()
	if err != nil {
		return err
	}
	app, err := models.GitHubApp.ActiveByInstance(ctx, service.db.Executor(), instanceID)
	if err != nil {
		return ErrGitHubNotConfigured
	}
	if strings.TrimSpace(webhookSecret) == "" || strings.TrimSpace(clientSecret) == "" {
		return errors.Join(models.ErrDomainValidation, errors.New("GitHub webhook and client secrets are required"))
	}
	if err := githubclient.ValidateAppAuthentication(githubclient.AppAuthentication{AppID: app.ExternalID, PrivateKeyPEM: privateKeyPEM}); err != nil {
		return errors.Join(models.ErrDomainValidation, err)
	}
	payload, err := json.Marshal(GitHubCredentialPayload{SchemaVersion: 1, PrivateKeyPEM: privateKeyPEM, WebhookSecret: webhookSecret, ClientSecret: clientSecret})
	if err != nil {
		return err
	}
	encrypted, err := secretcrypto.EncryptForPurpose(payload, service.cfg.App.SessionEncryptionKey, githubCredentialPurpose)
	if err != nil {
		return err
	}

	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "github-app:"+instanceID.String()); err != nil {
		return err
	}
	credential, err := models.Credential.Find(ctx, tx, app.CredentialID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := models.Credential.Update(ctx, tx, models.UpdateCredentialData{ID: credential.ID, Name: credential.Name, Provider: credential.Provider, Metadata: credential.Metadata, EncPayload: encrypted, VerifiedAt: sql.NullTime{Time: now, Valid: true}, LastUsedAt: credential.LastUsedAt, ArchivedAt: credential.ArchivedAt}); err != nil {
		return err
	}
	if _, err := tx.NewUpdate().TableExpr("github_apps").Set("verified_at = ?", now).Set("updated_at = ?", now).Where("id = ? AND archived_at IS NULL", app.ID).Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *GitHubConnection) ArchiveInstallation(ctx context.Context, id uuid.UUID) error {
	count, err := models.GitHubInstallation.ActiveSourceCount(ctx, service.db.Executor(), id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrGitHubDependencies
	}
	return models.GitHubInstallation.Archive(ctx, service.db.Executor(), id)
}

func (service *GitHubConnection) ArchiveApp(ctx context.Context) error {
	instanceID, err := service.instanceID()
	if err != nil {
		return err
	}
	app, err := models.GitHubApp.ActiveByInstance(ctx, service.db.Executor(), instanceID)
	if err != nil {
		return ErrGitHubNotConfigured
	}
	count, err := models.GitHubApp.ActiveDependencyCount(ctx, service.db.Executor(), app.ID)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrGitHubDependencies
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := models.GitHubApp.Archive(ctx, tx, app.ID); err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := tx.NewUpdate().TableExpr("credentials").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("id = ?", app.CredentialID).Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *GitHubConnection) State(ctx context.Context) (GitHubConnectionState, error) {
	instanceID, err := service.instanceID()
	if err != nil {
		return GitHubConnectionState{}, err
	}
	app, err := models.GitHubApp.ActiveByInstance(ctx, service.db.Executor(), instanceID)
	if errors.Is(err, sql.ErrNoRows) {
		return GitHubConnectionState{Installations: []GitHubInstallationSummary{}}, nil
	}
	if err != nil {
		return GitHubConnectionState{}, err
	}
	installations, err := models.GitHubInstallation.ListForApp(ctx, service.db.Executor(), app.ID)
	if err != nil {
		return GitHubConnectionState{}, err
	}
	summaries := make([]GitHubInstallationSummary, 0, len(installations))
	_, credentialErr := service.authentication(ctx, app)
	degraded := credentialErr != nil
	for _, installation := range installations {
		count, countErr := service.db.Executor().NewSelect().TableExpr("github_repositories").Where("github_installation_id = ?", installation.ID).Where("removed_at IS NULL").Count(ctx)
		if countErr != nil {
			return GitHubConnectionState{}, countErr
		}
		if installation.SuspendedAt.Valid || installation.ArchivedAt.Valid {
			degraded = true
		}
		summaries = append(summaries, GitHubInstallationSummary{GitHubInstallationEntity: installation, RepositoryCount: count})
	}
	state := GitHubConnectionState{App: &app, Installations: summaries, Degraded: degraded}
	if credentialErr != nil {
		state.HealthMessage = "The GitHub App credential is unavailable or invalid"
	} else if degraded {
		state.HealthMessage = "One or more GitHub installations require attention"
	} else {
		state.HealthMessage = "GitHub App credentials and installations are available"
	}
	return state, nil
}

func (service *GitHubConnection) authentication(ctx context.Context, app models.GitHubAppEntity) (githubclient.AppAuthentication, error) {
	credential, err := models.Credential.Find(ctx, service.db.Executor(), app.CredentialID)
	if err != nil {
		return githubclient.AppAuthentication{}, err
	}
	var metadata GitHubCredentialMetadata
	if json.Unmarshal(credential.Metadata, &metadata) != nil || metadata.InstanceID != app.InstanceID.String() || metadata.CredentialKind != models.GitHubCredentialProvider {
		return githubclient.AppAuthentication{}, errors.New("GitHub App credential metadata does not match this instance")
	}
	plaintext, err := secretcrypto.DecryptForPurpose(credential.EncPayload, service.cfg.App.SessionEncryptionKey, githubCredentialPurpose)
	if err != nil {
		return githubclient.AppAuthentication{}, errors.New("GitHub App credential could not be decrypted")
	}
	var payload GitHubCredentialPayload
	if json.Unmarshal(plaintext, &payload) != nil || payload.SchemaVersion != 1 || payload.PrivateKeyPEM == "" {
		return githubclient.AppAuthentication{}, errors.New("GitHub App credential is invalid")
	}
	return githubclient.AppAuthentication{AppID: app.ExternalID, PrivateKeyPEM: payload.PrivateKeyPEM}, nil
}

func (service *GitHubConnection) webhookSecret(ctx context.Context, app models.GitHubAppEntity) (string, error) {
	credential, err := models.Credential.Find(ctx, service.db.Executor(), app.CredentialID)
	if err != nil {
		return "", err
	}
	plaintext, err := secretcrypto.DecryptForPurpose(credential.EncPayload, service.cfg.App.SessionEncryptionKey, githubCredentialPurpose)
	if err != nil {
		return "", errors.New("GitHub App credential could not be decrypted")
	}
	var payload GitHubCredentialPayload
	if json.Unmarshal(plaintext, &payload) != nil || payload.SchemaVersion != 1 || payload.WebhookSecret == "" {
		return "", errors.New("GitHub App webhook credential is invalid")
	}
	return payload.WebhookSecret, nil
}

func (service *GitHubConnection) instanceID() (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(service.cfg.App.InstanceID))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, errors.New("INSTANCE_ID must be a valid UUID")
	}
	return id, nil
}

func (service *GitHubConnection) newState() (string, []byte, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", nil, err
	}
	state := base64.RawURLEncoding.EncodeToString(value)
	digest, err := service.stateDigest(state)
	return state, digest, err
}

func (service *GitHubConnection) stateDigest(state string) ([]byte, error) {
	if len(state) < 32 || len(state) > 256 {
		return nil, ErrGitHubSetupState
	}
	key, err := hex.DecodeString(service.cfg.App.SessionEncryptionKey)
	if err != nil || len(key) != 32 {
		return nil, errors.New("session encryption key is invalid")
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("github-setup-state/v1/"))
	mac.Write([]byte(state))
	return mac.Sum(nil), nil
}

func validateGitHubOrigin() error {
	if config.Env != server.ProdEnvironment {
		return nil
	}
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.Hostname() == "localhost" || parsed.Hostname() == "0.0.0.0" || parsed.Hostname() == "::" {
		return ErrGitHubProductionOrigin
	}
	return nil
}

func containsGitHubEvent(values []string, expected string) bool {
	return slices.Contains(values, expected)
}

func matchesGitHubOwner(attempt models.GitHubAppSetupAttemptEntity, ownerType, ownerLogin string) bool {
	if !attempt.OwnerType.Valid || !strings.EqualFold(attempt.OwnerType.String, strings.TrimSpace(ownerType)) {
		return false
	}
	return !attempt.OwnerLogin.Valid || strings.EqualFold(attempt.OwnerLogin.String, strings.TrimSpace(ownerLogin))
}
