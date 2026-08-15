package services

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type SourceDeploymentResult struct {
	Build              *models.BuildEntity      `json:"build,omitempty"`
	Release            *models.ReleaseEntity    `json:"release,omitempty"`
	Deployment         *models.DeploymentEntity `json:"deployment,omitempty"`
	DeploymentDeferred bool                     `json:"deploymentDeferred"`
}

type ReleaseDeploymentResult struct {
	Deployment         models.DeploymentEntity
	DeploymentDeferred bool
}

type PromotionResult struct {
	SourceDeployment models.DeploymentEntity
	SourceRelease    models.ReleaseEntity
	Release          models.ReleaseEntity
	Deployment       models.DeploymentEntity
	Deferred         bool
}

type resolvedImageArtifact struct {
	Version              string
	Reference            string
	Digest               []byte
	RegistryResourceID   uuid.UUID
	RegistryCredentialID uuid.UUID
	RegistryEndpoint     string
}

func (service *EnvironmentSetup) RotateAPIToken(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) (string, error) {
	random, err := models.GenerateSecureToken()
	if err != nil {
		return "", err
	}
	token := "dcenv_" + strings.ToLower(random)
	digest := []byte(models.HashForStorage(token, service.config.App.SessionEncryptionKey))
	prefixLength := min(len(token), 13)
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil || environment.ApplicationID != applicationID || environment.ArchivedAt.Valid {
		return "", errors.New("Environment is unavailable")
	}
	if _, err := models.Environment.Update(ctx, tx, models.UpdateEnvironmentData{
		ID:   environment.ID,
		Name: environment.Name,
		Slug: environment.Slug,
		Kind: environment.Kind,
		APITokenPrefix: sql.NullString{
			String: token[:prefixLength],
			Valid:  true,
		},
		APITokenDigest: digest,
		ArchivedAt:     environment.ArchivedAt,
		ApplicationID:  environment.ApplicationID,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

func (service *EnvironmentSetup) AuthenticateAPIToken(
	ctx context.Context,
	environmentID uuid.UUID,
	token string,
) (models.EnvironmentEntity, error) {
	token = strings.TrimSpace(token)
	environment, err := models.Environment.Find(ctx, service.db.Executor(), environmentID)
	if err != nil || environment.ArchivedAt.Valid || !environment.APITokenPrefix.Valid ||
		len(environment.APITokenDigest) == 0 {
		return models.EnvironmentEntity{}, errors.New("invalid Environment API token")
	}
	digest := []byte(models.HashForStorage(token, service.config.App.SessionEncryptionKey))
	if !hmac.Equal(environment.APITokenDigest, digest) {
		return models.EnvironmentEntity{}, errors.New("invalid Environment API token")
	}
	return environment, nil
}

func (service *EnvironmentSetup) RequestSourceDeployment(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
	actorID *uuid.UUID,
	triggerType, reference string,
) (SourceDeploymentResult, error) {
	source, _, _, err := service.loadSource(ctx, applicationID, environmentID)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	if source.Kind != "image" && strings.TrimSpace(reference) != "" {
		return SourceDeploymentResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Buildpacks deployments do not accept an image reference override"),
		)
	}
	if source.Kind != "image" && actorID == nil {
		return SourceDeploymentResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Buildpacks API deployments are not supported"),
		)
	}
	deployability, err := models.Environment.Deployability(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	blocking := make([]string, 0, len(deployability.Missing))
	for _, missing := range deployability.Missing {
		if missing != "managed_dns" {
			blocking = append(blocking, missing)
		}
	}
	if len(blocking) > 0 {
		return SourceDeploymentResult{}, errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf("Environment is not deployable: %s", strings.Join(blocking, ", ")),
		)
	}
	deferred, err := service.dns.PrepareDeployment(
		ctx,
		environmentID,
		actorID,
		triggerType,
		reference,
	)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	if deferred {
		return SourceDeploymentResult{DeploymentDeferred: true}, nil
	}
	return service.QueueSourceDeployment(
		ctx,
		applicationID,
		environmentID,
		actorID,
		triggerType,
		reference,
	)
}

func (service *EnvironmentSetup) QueueSourceDeployment(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
	actorID *uuid.UUID,
	triggerType, reference string,
) (SourceDeploymentResult, error) {
	source, _, _, err := service.loadSource(ctx, applicationID, environmentID)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	if source.Kind != "image" {
		if strings.TrimSpace(reference) != "" {
			return SourceDeploymentResult{}, errors.Join(
				models.ErrDomainValidation,
				errors.New("Buildpacks deployments do not accept an image reference override"),
			)
		}
		if actorID == nil {
			return SourceDeploymentResult{}, errors.Join(
				models.ErrDomainValidation,
				errors.New("Buildpacks API deployments are not supported"),
			)
		}
		build, err := service.QueueManualDeploy(ctx, applicationID, environmentID, *actorID)
		if err != nil {
			return SourceDeploymentResult{}, err
		}
		return SourceDeploymentResult{Build: &build}, nil
	}
	deployability, err := models.Environment.Deployability(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	if !deployability.Deployable {
		return SourceDeploymentResult{}, errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf(
				"Environment is not deployable: %s",
				strings.Join(deployability.Missing, ", "),
			),
		)
	}
	artifact, err := service.resolveImageArtifact(ctx, source, reference)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	defer tx.Rollback()
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil || environment.ApplicationID != applicationID || environment.ArchivedAt.Valid {
		return SourceDeploymentResult{}, errors.New("Environment is unavailable")
	}
	revision, err := models.EnvironmentStateRevision.LatestCommitted(ctx, tx, environmentID)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	release, deployment, err := service.queueImageDeploymentTx(
		ctx,
		tx,
		source,
		revision,
		artifact,
		triggerType,
		actorID,
		"Deploy image "+artifact.Version,
	)
	if err != nil {
		return SourceDeploymentResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return SourceDeploymentResult{}, err
	}
	return SourceDeploymentResult{Release: &release, Deployment: &deployment}, nil
}

func (service *EnvironmentSetup) resolveImageArtifact(
	ctx context.Context,
	source environmentSetupSource,
	override string,
) (resolvedImageArtifact, error) {
	version := strings.TrimSpace(override)
	if version == "" {
		version = strings.TrimSpace(source.Reference)
	}
	if version == "" || strings.ContainsAny(version, " /\t\r\n") {
		return resolvedImageArtifact{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("image tag or digest is invalid"),
		)
	}
	credentials, err := service.builds.RegistryCredentials(
		ctx,
		source.RegistryID,
		source.RegistryCredentialID,
		source.RegistryEndpoint,
	)
	if err != nil {
		return resolvedImageArtifact{}, fmt.Errorf("load image registry credentials: %w", err)
	}
	separator := ":"
	if strings.HasPrefix(strings.ToLower(version), "sha256:") {
		separator = "@"
	}
	mutableReference := strings.TrimSuffix(
		source.RegistryEndpoint,
		"/",
	) + "/" + strings.Trim(
		source.ImageRepository,
		"/",
	) + separator + version
	immutableReference, err := service.registry.ResolveRemoteDigest(
		ctx,
		credentials,
		mutableReference,
	)
	if err != nil {
		return resolvedImageArtifact{}, fmt.Errorf("resolve image reference: %w", err)
	}
	digestIndex := strings.LastIndex(immutableReference, "@sha256:")
	if digestIndex < 0 {
		return resolvedImageArtifact{}, errors.New("resolved image reference is not immutable")
	}
	digestText := immutableReference[digestIndex+len("@sha256:"):]
	digest, err := hex.DecodeString(digestText)
	if err != nil || len(digest) != 32 {
		return resolvedImageArtifact{}, errors.New("resolved image digest is invalid")
	}
	return resolvedImageArtifact{
		Version: version, Reference: immutableReference, Digest: digest,
		RegistryResourceID: source.RegistryID, RegistryCredentialID: source.RegistryCredentialID,
		RegistryEndpoint: source.RegistryEndpoint,
	}, nil
}

func (service *EnvironmentSetup) queueImageDeploymentTx(
	ctx context.Context,
	tx bun.Tx,
	source environmentSetupSource,
	revision models.EnvironmentStateRevisionEntity,
	artifact resolvedImageArtifact,
	triggerType string,
	actorID *uuid.UUID,
	summary string,
) (models.ReleaseEntity, models.DeploymentEntity, error) {
	targets, err := models.EnvironmentTarget.ActiveForEnvironmentAll(ctx, tx, source.EnvironmentID)
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	if len(targets) == 0 {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment has no runtime Server targets"),
		)
	}
	active, err := models.Deployment.ActiveCountForEnvironment(ctx, tx, source.EnvironmentID)
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	if active > 0 {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment already has an active Deployment"),
		)
	}
	now := time.Now().UTC()
	sequence, err := models.Change.NextSequence(ctx, tx, source.EnvironmentID)
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	actorType := "system"
	if actorID != nil {
		actorType = "user"
	}
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence:    sequence,
		Kind:        "deploy",
		TriggerType: triggerType,
		ActorType:   actorType,
		ActorID:     actorID,
		CauseSystem: sql.NullString{
			String: "registry_image",
			Valid:  true,
		},
		CauseReference:    sql.NullString{String: artifact.Reference, Valid: true},
		CorrelationID:     uuid.New(),
		CorrectionContext: json.RawMessage(`{}`),
		Summary:           summary,
		Status:            "committed",
		RequestedAt:       now,
		CommittedAt:       sql.NullTime{Time: now, Valid: true},
		EnvironmentID:     source.EnvironmentID,
	})
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	release, err := models.Release.Create(ctx, tx, models.CreateReleaseData{
		Version: sql.NullString{
			String: artifact.Version,
			Valid:  true,
		},
		ArtifactReference:    artifact.Reference,
		ArtifactDigest:       artifact.Digest,
		EnvironmentID:        source.EnvironmentID,
		EnvironmentSourceID:  &source.EnvironmentSourceID,
		CreatedByChangeID:    change.ID,
		RegistryResourceID:   &artifact.RegistryResourceID,
		RegistryCredentialID: &artifact.RegistryCredentialID,
		RegistryEndpoint:     sql.NullString{String: artifact.RegistryEndpoint, Valid: true},
	})
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	if _, err := models.ChangeRelease.Create(
		ctx,
		tx,
		models.CreateChangeReleaseData{ChangeID: change.ID, ReleaseID: release.ID},
	); err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	if _, err := models.ChangeStateRevision.Create(
		ctx,
		tx,
		models.CreateChangeStateRevisionData{
			Role:                       "result",
			ChangeID:                   change.ID,
			EnvironmentStateRevisionID: revision.ID,
		},
	); err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	result, err := service.releases.OrchestrateTx(ctx, tx, release, change, revision)
	if err != nil {
		return models.ReleaseEntity{}, models.DeploymentEntity{}, err
	}
	return release, result.Deployment, nil
}

func (service *EnvironmentSetup) QueueManualDeploy(
	ctx context.Context,
	applicationID, environmentID, userID uuid.UUID,
) (models.BuildEntity, error) {
	deployability, err := models.Environment.Deployability(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return models.BuildEntity{}, err
	}
	if !deployability.Deployable {
		return models.BuildEntity{}, errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf(
				"Environment is not deployable: %s",
				strings.Join(deployability.Missing, ", "),
			),
		)
	}
	source, repository, installation, err := service.loadSource(ctx, applicationID, environmentID)
	if err != nil {
		return models.BuildEntity{}, err
	}
	revisionSHA, err := service.github.ResolveRevision(
		ctx,
		installation,
		repository,
		source.Reference,
	)
	if err != nil {
		return models.BuildEntity{}, err
	}
	stateRevision, err := models.EnvironmentStateRevision.LatestCommitted(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return models.BuildEntity{}, err
	}
	state, err := models.ParseEnvironmentDesiredState(stateRevision.State)
	if err != nil {
		return models.BuildEntity{}, err
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.BuildEntity{}, err
	}
	defer tx.Rollback()
	if _, err := models.Environment.Lock(ctx, tx, environmentID); err != nil {
		return models.BuildEntity{}, err
	}
	now := time.Now().UTC()
	correlationID := uuid.New()
	payload, _ := json.Marshal(
		map[string]any{
			"schema_version":                1,
			"reference":                     source.Reference,
			"revision":                      revisionSHA,
			"repository":                    source.Repository,
			"environment_state_revision_id": stateRevision.ID,
		},
	)
	event, err := models.SourceEvent.Create(
		ctx,
		tx,
		models.CreateSourceEventData{
			ExternalID:          "manual:" + correlationID.String(),
			Kind:                "manual_deploy",
			SourceRevision:      sql.NullString{String: revisionSHA, Valid: true},
			Payload:             payload,
			ReceivedAt:          now,
			ProcessedAt:         sql.NullTime{Time: now, Valid: true},
			EnvironmentSourceID: source.EnvironmentSourceID,
		},
	)
	if err != nil {
		return models.BuildEntity{}, err
	}
	sequence, err := models.Change.NextSequence(ctx, tx, environmentID)
	if err != nil {
		return models.BuildEntity{}, err
	}
	change, err := models.Change.Create(
		ctx,
		tx,
		models.CreateChangeData{
			Sequence:      sequence,
			Kind:          "build",
			TriggerType:   "user",
			ActorType:     "user",
			ActorID:       &userID,
			CorrelationID: correlationID,
			Summary:       "Deploy current GitHub revision",
			Status:        "committed",
			RequestedAt:   now,
			CommittedAt:   sql.NullTime{Time: now, Valid: true},
			EnvironmentID: environmentID,
		},
	)
	if err != nil {
		return models.BuildEntity{}, err
	}
	buildConfiguration, err := marshalBuildSnapshot(buildSnapshot{
		SchemaVersion:              2,
		SourceEventID:              event.ID,
		EnvironmentStateRevisionID: stateRevision.ID,
		Repository:                 source.Repository,
		Reference:                  source.Reference,
		SourceRevision:             revisionSHA,
		ContextPath:                source.ContextPath,
		BuilderReference:           nullableStringPointer(source.BuilderReference),
		ImageRepository:            source.ImageRepository,
		RegistryResourceID:         source.RegistryID,
		RegistryCredentialID:       source.RegistryCredentialID,
		RegistryEndpoint:           source.RegistryEndpoint,
		Settings:                   source.BuildpackSettings,
		BPGOTargets:                models.FlattenGoProcessTargets(state.Runtime.BPGOTargets),
		ServerID:                   source.BuildServerID,
	})
	if err != nil {
		return models.BuildEntity{}, fmt.Errorf("create Build configuration snapshot: %w", err)
	}
	build, err := models.Build.Create(ctx, tx, models.CreateBuildData{
		SourceRevision:      revisionSHA,
		BuildMethod:         "buildpacks",
		BuildConfiguration:  buildConfiguration,
		Status:              "pending",
		CurrentStep:         sql.NullString{String: "queued", Valid: true},
		EnvironmentID:       environmentID,
		EnvironmentSourceID: source.EnvironmentSourceID,
		ChangeID:            change.ID,
	})
	if err != nil {
		return models.BuildEntity{}, err
	}
	if _, err := service.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.BuildSourceArgs{BuildID: build.ID},
		jobs.BuildSourceInsertOpts(build.ID),
	); err != nil {
		return models.BuildEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.BuildEntity{}, err
	}
	return build, nil
}
