package services

import (
	"bytes"
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (service *EnvironmentSetup) QueueReleaseDeployment(
	ctx context.Context,
	applicationID, environmentID, releaseID, userID uuid.UUID,
) (ReleaseDeploymentResult, error) {
	deployability, err := models.Environment.Deployability(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	blocking := make([]string, 0, len(deployability.Missing))
	dnsPending := false
	for _, missing := range deployability.Missing {
		if missing == "managed_dns" {
			dnsPending = true
		} else {
			blocking = append(blocking, missing)
		}
	}
	if len(blocking) > 0 {
		return ReleaseDeploymentResult{}, errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf(
				"Environment is not deployable: %s",
				strings.Join(blocking, ", "),
			),
		)
	}
	if dnsPending {
		deferred, err := service.dns.PrepareDeployment(
			ctx,
			environmentID,
			&userID,
			ReleaseRedeployTriggerType,
			releaseID.String(),
		)
		if err != nil {
			return ReleaseDeploymentResult{}, err
		}
		if deferred {
			return ReleaseDeploymentResult{DeploymentDeferred: true}, nil
		}
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	defer tx.Rollback()
	environment, err := models.Environment.Lock(ctx, tx, environmentID)
	if err != nil || environment.ApplicationID != applicationID || environment.ArchivedAt.Valid {
		return ReleaseDeploymentResult{}, errors.New("Environment is unavailable")
	}
	release, err := models.Release.Find(ctx, tx, releaseID)
	if err != nil || release.EnvironmentID != environmentID || release.RegistryResourceID == nil ||
		release.RegistryCredentialID == nil ||
		!release.RegistryEndpoint.Valid {
		return ReleaseDeploymentResult{}, errors.New("Release does not belong to this Environment")
	}
	targets, err := models.EnvironmentTarget.ActiveForEnvironmentAll(ctx, tx, environmentID)
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	if len(targets) == 0 {
		return ReleaseDeploymentResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment has no runtime Server targets"),
		)
	}
	active, err := models.Deployment.ActiveCountForEnvironment(ctx, tx, environmentID)
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	if active > 0 {
		return ReleaseDeploymentResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Environment already has an active Deployment"),
		)
	}
	revision, err := models.EnvironmentStateRevision.LatestCommitted(ctx, tx, environmentID)
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	now := time.Now().UTC()
	sequence, err := models.Change.NextSequence(ctx, tx, environmentID)
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence:    sequence,
		Kind:        "redeploy",
		TriggerType: "user",
		ActorType:   "user",
		ActorID:     &userID,
		CauseSystem: sql.NullString{
			String: "release",
			Valid:  true,
		},
		CauseReference: sql.NullString{String: release.ID.String(), Valid: true},
		CorrelationID:  uuid.New(),
		Summary:        "Redeploy selected Release",
		Status:         "committed",
		RequestedAt:    now,
		CommittedAt:    sql.NullTime{Time: now, Valid: true},
		EnvironmentID:  environmentID,
	})
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	if _, err := models.ChangeRelease.Create(
		ctx,
		tx,
		models.CreateChangeReleaseData{ChangeID: change.ID, ReleaseID: release.ID},
	); err != nil {
		return ReleaseDeploymentResult{}, err
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
		return ReleaseDeploymentResult{}, err
	}
	result, err := service.releases.OrchestrateTx(ctx, tx, release, change, revision)
	if err != nil {
		return ReleaseDeploymentResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReleaseDeploymentResult{}, err
	}
	return ReleaseDeploymentResult{Deployment: result.Deployment}, nil
}

func (service *EnvironmentSetup) PromoteToProduction(
	ctx context.Context,
	applicationID, stagingEnvironmentID, userID uuid.UUID,
) (PromotionResult, error) {
	staging, err := models.Environment.FindForApplication(
		ctx,
		service.db.Executor(),
		applicationID,
		stagingEnvironmentID,
	)
	if err != nil || staging.ArchivedAt.Valid {
		return PromotionResult{}, errors.New("Environment is unavailable")
	}
	if !strings.EqualFold(strings.TrimSpace(staging.Kind), "staging") {
		return PromotionResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Only a staging Environment can be promoted to production"),
		)
	}
	production, err := productionEnvironmentForApplication(
		ctx,
		service.db.Executor(),
		applicationID,
	)
	if err != nil {
		return PromotionResult{}, err
	}
	sourceDeployment, err := models.Deployment.LatestSucceededForEnvironment(
		ctx,
		service.db.Executor(),
		staging.ID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PromotionResult{}, errors.Join(
				models.ErrDomainValidation,
				errors.New("No successful staging deployment is available to promote"),
			)
		}
		return PromotionResult{}, err
	}
	sourceRelease, err := models.Release.Find(
		ctx,
		service.db.Executor(),
		sourceDeployment.ReleaseID,
	)
	if err != nil {
		return PromotionResult{}, err
	}
	if !promotableRelease(sourceRelease, staging.ID) {
		return PromotionResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Latest successful staging deployment has no complete immutable artifact"),
		)
	}
	deployability, err := models.Environment.Deployability(
		ctx,
		service.db.Executor(),
		production.ID,
	)
	if err != nil {
		return PromotionResult{}, err
	}
	blocking := make([]string, 0, len(deployability.Missing))
	dnsPending := false
	for _, missing := range deployability.Missing {
		if missing == "managed_dns" {
			dnsPending = true
		} else {
			blocking = append(blocking, missing)
		}
	}
	if len(blocking) > 0 {
		return PromotionResult{}, errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf(
				"Production environment is not deployable: %s",
				strings.Join(blocking, ", "),
			),
		)
	}
	if dnsPending {
		deferred, err := service.dns.PrepareDeployment(
			ctx,
			production.ID,
			&userID,
			ReleasePromoteTriggerType,
			staging.ID.String(),
		)
		if err != nil {
			return PromotionResult{}, err
		}
		if deferred {
			return PromotionResult{Deferred: true}, nil
		}
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return PromotionResult{}, err
	}
	defer tx.Rollback()
	staging, err = models.Environment.Lock(ctx, tx, staging.ID)
	if err != nil || staging.ApplicationID != applicationID || staging.ArchivedAt.Valid {
		return PromotionResult{}, errors.New("Environment is unavailable")
	}
	if !strings.EqualFold(strings.TrimSpace(staging.Kind), "staging") {
		return PromotionResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Only a staging Environment can be promoted to production"),
		)
	}
	production, err = models.Environment.Lock(ctx, tx, production.ID)
	if err != nil || production.ApplicationID != applicationID || production.ArchivedAt.Valid {
		return PromotionResult{}, errors.New("Production environment is unavailable")
	}
	sourceDeployment, err = models.Deployment.LatestSucceededForEnvironment(
		ctx,
		tx,
		staging.ID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PromotionResult{}, errors.Join(
				models.ErrDomainValidation,
				errors.New("No successful staging deployment is available to promote"),
			)
		}
		return PromotionResult{}, err
	}
	sourceRelease, err = models.Release.Find(ctx, tx, sourceDeployment.ReleaseID)
	if err != nil {
		return PromotionResult{}, err
	}
	if !promotableRelease(sourceRelease, staging.ID) {
		return PromotionResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("Latest successful staging deployment has no complete immutable artifact"),
		)
	}
	revision, err := models.EnvironmentStateRevision.LatestCommitted(ctx, tx, production.ID)
	if err != nil {
		return PromotionResult{}, err
	}
	productionSourceID, err := models.EnvironmentSource.LatestActiveID(ctx, tx, production.ID)
	if err != nil {
		return PromotionResult{}, err
	}
	now := time.Now().UTC()
	sequence, err := models.Change.NextSequence(ctx, tx, production.ID)
	if err != nil {
		return PromotionResult{}, err
	}
	change, err := models.Change.Create(ctx, tx, models.CreateChangeData{
		Sequence:    sequence,
		Kind:        "deploy",
		TriggerType: "user",
		ActorType:   "user",
		ActorID:     &userID,
		CauseSystem: sql.NullString{
			String: "promotion",
			Valid:  true,
		},
		CauseReference:    sql.NullString{String: sourceRelease.ID.String(), Valid: true},
		CorrelationID:     uuid.New(),
		CorrectionContext: json.RawMessage(`{}`),
		Summary:           "Promote staging release to production",
		Status:            "committed",
		RequestedAt:       now,
		CommittedAt:       sql.NullTime{Time: now, Valid: true},
		EnvironmentID:     production.ID,
	})
	if err != nil {
		return PromotionResult{}, err
	}
	release, err := models.Release.Create(ctx, tx, models.CreateReleaseData{
		Version:              sourceRelease.Version,
		SourceRevision:       sourceRelease.SourceRevision,
		ArtifactReference:    sourceRelease.ArtifactReference,
		ArtifactDigest:       sourceRelease.ArtifactDigest,
		EnvironmentID:        production.ID,
		EnvironmentSourceID:  productionSourceID,
		BuildID:              sourceRelease.BuildID,
		CreatedByChangeID:    change.ID,
		RegistryResourceID:   sourceRelease.RegistryResourceID,
		RegistryCredentialID: sourceRelease.RegistryCredentialID,
		RegistryEndpoint:     sourceRelease.RegistryEndpoint,
	})
	if err != nil {
		return PromotionResult{}, err
	}
	if _, err := models.ChangeRelease.Create(
		ctx,
		tx,
		models.CreateChangeReleaseData{ChangeID: change.ID, ReleaseID: release.ID},
	); err != nil {
		return PromotionResult{}, err
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
		return PromotionResult{}, err
	}
	result, err := service.releases.OrchestrateTx(ctx, tx, release, change, revision)
	if err != nil {
		return PromotionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return PromotionResult{}, err
	}
	return PromotionResult{
		SourceDeployment: sourceDeployment,
		SourceRelease:    sourceRelease,
		Release:          release,
		Deployment:       result.Deployment,
	}, nil
}

func promotableRelease(release models.ReleaseEntity, environmentID uuid.UUID) bool {
	return release.EnvironmentID == environmentID &&
		strings.TrimSpace(release.ArtifactReference) != "" &&
		len(release.ArtifactDigest) > 0 &&
		release.RegistryResourceID != nil &&
		release.RegistryCredentialID != nil &&
		release.RegistryEndpoint.Valid
}

func productionEnvironmentForApplication(
	ctx context.Context,
	exec storage.Executor,
	applicationID uuid.UUID,
) (models.EnvironmentEntity, error) {
	environments, err := models.Environment.ProductionForApplication(ctx, exec, applicationID)
	if err != nil {
		return models.EnvironmentEntity{}, err
	}
	if len(environments) == 0 {
		return models.EnvironmentEntity{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("No production Environment is available to promote to"),
		)
	}
	if len(environments) > 1 {
		return models.EnvironmentEntity{}, errors.Join(
			models.ErrDomainValidation,
			errors.New(
				"Multiple production Environments exist; promotion target selection is not supported",
			),
		)
	}
	return environments[0], nil
}

func promotionOverview(
	ctx context.Context,
	exec storage.Executor,
	applicationID, environmentID uuid.UUID,
	kind string,
	setupComplete bool,
) (canPromote bool, targetName string, latestDeploymentID, latestReleaseID *uuid.UUID, err error) {
	if !strings.EqualFold(strings.TrimSpace(kind), "staging") || !setupComplete {
		return false, "", nil, nil, nil
	}
	production, err := productionEnvironmentForApplication(ctx, exec, applicationID)
	if err != nil {
		return false, "", nil, nil, nil
	}
	deployment, err := models.Deployment.LatestSucceededForEnvironment(
		ctx,
		exec,
		environmentID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return false, production.Name, nil, nil, nil
	}
	if err != nil {
		return false, "", nil, nil, err
	}
	sourceRelease, err := models.Release.Find(ctx, exec, deployment.ReleaseID)
	if err != nil {
		return false, "", nil, nil, err
	}
	productionDeployment, err := models.Deployment.LatestSucceededForEnvironment(
		ctx,
		exec,
		production.ID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return true, production.Name, &deployment.ID, &deployment.ReleaseID, nil
	}
	if err != nil {
		return false, "", nil, nil, err
	}
	productionRelease, err := models.Release.Find(ctx, exec, productionDeployment.ReleaseID)
	if err != nil {
		return false, "", nil, nil, err
	}
	return !sameArtifact(
		sourceRelease,
		productionRelease,
	), production.Name, &deployment.ID, &deployment.ReleaseID, nil
}

func sameArtifact(a, b models.ReleaseEntity) bool {
	if len(a.ArtifactDigest) > 0 && len(b.ArtifactDigest) > 0 {
		return bytes.Equal(a.ArtifactDigest, b.ArtifactDigest)
	}
	referenceA := strings.TrimSpace(a.ArtifactReference)
	referenceB := strings.TrimSpace(b.ArtifactReference)
	return referenceA != "" && referenceA == referenceB
}
