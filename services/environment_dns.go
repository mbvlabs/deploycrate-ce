package services

import (
	"context"
	"database/sql"
	cloudflareclient "deploycrate-ce/clients/cloudflare"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue/jobs"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	DNSModeManual     = "manual"
	DNSModeCloudflare = "cloudflare"

	ReleaseRedeployTriggerType = "release_redeploy"
	ReleasePromoteTriggerType  = "release_promote"
)

type EnvironmentDNS struct {
	db     storage.Pool
	queue  storage.InsertQueue
	client CloudflareDNSClient
	config config.Config
}

type EnvironmentDNSInput struct {
	Mode   string
	ZoneID *uuid.UUID
}

type EnvironmentDNSOption = models.EnvironmentDNSOption

type EnvironmentDNSStatus struct {
	Mode                 string            `json:"mode"`
	BindingID            *uuid.UUID        `json:"bindingId,omitempty"`
	ZoneID               *uuid.UUID        `json:"zoneId,omitempty"`
	ZoneName             string            `json:"zoneName"`
	ConnectionName       string            `json:"connectionName"`
	State                string            `json:"state"`
	Generation           int64             `json:"generation"`
	AppliedGeneration    int64             `json:"appliedGeneration"`
	LastError            string            `json:"lastError"`
	ReconciliationQueued bool              `json:"reconciliationQueued"`
	AppliedAt            sql.NullTime      `json:"appliedAt"`
	Records              []DNSRecordStatus `json:"records"`
}

type DNSRecordStatus = models.DNSRecordStatus

type DNSConfigureResult struct {
	Managed            bool
	DeploymentDeferred bool
	Binding            models.EnvironmentDNSBindingEntity
}

type DNSDeploymentIntent struct {
	BindingID     uuid.UUID
	Generation    int64
	ApplicationID uuid.UUID
	EnvironmentID uuid.UUID
	ActorID       *uuid.UUID
	TriggerType   string
	Reference     string
}

type dnsReconciliationScope = models.DNSReconciliationScope
type dnsTrackedRemoval = models.DNSTrackedRemoval

func NewEnvironmentDNS(
	db storage.Pool,
	queue storage.InsertQueue,
	client CloudflareDNSClient,
	cfg config.Config,
) *EnvironmentDNS {
	return &EnvironmentDNS{db: db, queue: queue, client: client, config: cfg}
}

func (service *EnvironmentDNS) Options(ctx context.Context) ([]EnvironmentDNSOption, error) {
	return models.DNSZone.EnvironmentOptions(ctx, service.db.Executor())
}

func (service *EnvironmentDNS) Status(
	ctx context.Context,
	environmentID uuid.UUID,
) (EnvironmentDNSStatus, error) {
	status := EnvironmentDNSStatus{
		Mode:    DNSModeManual,
		State:   "manual",
		Records: []DNSRecordStatus{},
	}
	row, err := models.EnvironmentDNSBinding.StatusForEnvironment(
		ctx, service.db.Executor(), environmentID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return EnvironmentDNSStatus{}, err
	}
	status.Mode = DNSModeCloudflare
	status.BindingID = &row.BindingID
	status.ZoneID = &row.ZoneID
	status.ZoneName = row.ZoneName
	status.ConnectionName = row.ConnectionName
	status.State = row.State
	status.Generation = row.Generation
	status.AppliedGeneration = row.AppliedGeneration
	status.LastError = row.LastError.String
	status.AppliedAt = row.AppliedAt
	queued, err := models.Job.DNSReconciliationQueued(
		ctx, service.db.Executor(), row.BindingID, row.Generation,
	)
	if err != nil {
		return EnvironmentDNSStatus{}, err
	}
	status.ReconciliationQueued = queued
	status.Records, err = models.EnvironmentDNSRecord.StatusForBinding(
		ctx, service.db.Executor(), row.BindingID,
	)
	if err != nil {
		return EnvironmentDNSStatus{}, err
	}
	return status, nil
}

func (service *EnvironmentDNS) ConfigureTx(
	ctx context.Context,
	tx bun.Tx,
	domain models.EnvironmentDomainEntity,
	input EnvironmentDNSInput,
	force bool,
	enqueue bool,
	deployAfterApply bool,
	actorID *uuid.UUID,
) (DNSConfigureResult, error) {
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	if mode == "" {
		mode = DNSModeManual
	}
	existing, existingErr := models.EnvironmentDNSBinding.ActiveForDomain(ctx, tx, domain.ID)
	if mode == DNSModeManual {
		if errors.Is(existingErr, sql.ErrNoRows) {
			return DNSConfigureResult{}, nil
		}
		if existingErr != nil {
			return DNSConfigureResult{}, existingErr
		}
		binding, err := models.EnvironmentDNSBinding.MarkRemoving(
			ctx,
			tx,
			existing.ID,
			deployAfterApply,
			actorID,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return DNSConfigureResult{}, errors.Join(
				models.ErrDomainValidation,
				errors.New("DNS reconciliation is in progress; retry this change shortly"),
			)
		}
		if err != nil {
			return DNSConfigureResult{}, err
		}
		if enqueue {
			if err := service.enqueueTx(ctx, tx, binding); err != nil {
				return DNSConfigureResult{}, err
			}
		}
		return DNSConfigureResult{DeploymentDeferred: deployAfterApply, Binding: binding}, nil
	}
	if mode != DNSModeCloudflare || input.ZoneID == nil || *input.ZoneID == uuid.Nil {
		return DNSConfigureResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("select a Cloudflare DNS zone or use manual DNS"),
		)
	}
	zone, err := models.DNSZone.Find(ctx, tx, *input.ZoneID)
	if err != nil || zone.ArchivedAt.Valid || zone.Status != "active" {
		return DNSConfigureResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("selected Cloudflare DNS zone is unavailable"),
		)
	}
	connection, err := models.DNSConnection.Find(ctx, tx, zone.DNSConnectionID)
	if err != nil || connection.ArchivedAt.Valid {
		return DNSConfigureResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("selected Cloudflare DNS connection is unavailable"),
		)
	}
	if !hostnameBelongsToZone(domain.Hostname, zone.Name) {
		return DNSConfigureResult{}, errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf("hostname must be %s or a subdomain of %s", zone.Name, zone.Name),
		)
	}
	if enqueue {
		if _, err := service.desiredIPv4(ctx, tx, domain.EnvironmentID); err != nil {
			return DNSConfigureResult{}, err
		}
	}
	if errors.Is(existingErr, sql.ErrNoRows) {
		binding, err := models.EnvironmentDNSBinding.Create(
			ctx,
			tx,
			models.CreateEnvironmentDNSBindingData{
				DNSZoneID:           zone.ID,
				EnvironmentDomainID: domain.ID,
				DeployAfterApply:    deployAfterApply,
				DeploymentActorID:   actorID,
			},
		)
		if err != nil {
			return DNSConfigureResult{}, err
		}
		if enqueue {
			if err := service.enqueueTx(ctx, tx, binding); err != nil {
				return DNSConfigureResult{}, err
			}
		}
		return DNSConfigureResult{
			Managed:            true,
			DeploymentDeferred: deployAfterApply,
			Binding:            binding,
		}, nil
	}
	if existingErr != nil {
		return DNSConfigureResult{}, existingErr
	}
	if existing.DNSZoneID == zone.ID && !force {
		deferred := existing.State != models.EnvironmentDNSApplied ||
			existing.AppliedGeneration != existing.Generation
		return DNSConfigureResult{
			Managed:            true,
			DeploymentDeferred: deferred,
			Binding:            existing,
		}, nil
	}
	binding, err := models.EnvironmentDNSBinding.Reconfigure(
		ctx,
		tx,
		existing.ID,
		zone.ID,
		deployAfterApply,
		actorID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return DNSConfigureResult{}, errors.Join(
			models.ErrDomainValidation,
			errors.New("DNS reconciliation is in progress; retry this change shortly"),
		)
	}
	if err != nil {
		return DNSConfigureResult{}, err
	}
	if enqueue {
		if err := service.enqueueTx(ctx, tx, binding); err != nil {
			return DNSConfigureResult{}, err
		}
	}
	return DNSConfigureResult{
		Managed:            true,
		DeploymentDeferred: deployAfterApply,
		Binding:            binding,
	}, nil
}

func (service *EnvironmentDNS) PrepareDeployment(
	ctx context.Context,
	environmentID uuid.UUID,
	actorID *uuid.UUID,
	triggerType, reference string,
) (bool, error) {
	triggerType = strings.TrimSpace(triggerType)
	if triggerType == "" {
		triggerType = "user"
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	binding, err := models.EnvironmentDNSBinding.ActiveForEnvironment(ctx, tx, environmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if binding.State == models.EnvironmentDNSApplied &&
		binding.AppliedGeneration == binding.Generation {
		if !binding.DeployAfterApply || binding.DeploymentDispatchedAt.Valid {
			return false, nil
		}
		binding, err = models.EnvironmentDNSBinding.PrepareDeployment(
			ctx,
			tx,
			binding.ID,
			actorID,
			triggerType,
			strings.TrimSpace(reference),
		)
		if err != nil {
			return false, err
		}
		if err := service.enqueueTx(ctx, tx, binding); err != nil {
			return false, err
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}
	if binding.State != models.EnvironmentDNSPending &&
		binding.State != models.EnvironmentDNSReconciling &&
		binding.State != models.EnvironmentDNSRemoving {
		return false, errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf(
				"managed DNS is %s; resolve the DNS status before deploying",
				strings.ReplaceAll(binding.State, "_", " "),
			),
		)
	}
	if binding.State == models.EnvironmentDNSPending {
		if _, err := service.desiredIPv4(ctx, tx, environmentID); err != nil {
			return false, err
		}
	}
	binding, err = models.EnvironmentDNSBinding.PrepareDeployment(
		ctx,
		tx,
		binding.ID,
		actorID,
		triggerType,
		strings.TrimSpace(reference),
	)
	if errors.Is(err, sql.ErrNoRows) {
		latest, latestErr := models.EnvironmentDNSBinding.ActiveForEnvironment(
			ctx,
			tx,
			environmentID,
		)
		if errors.Is(latestErr, sql.ErrNoRows) {
			return false, nil
		}
		if latestErr == nil && latest.State == models.EnvironmentDNSApplied &&
			latest.AppliedGeneration == latest.Generation {
			return false, nil
		}
	}
	if err != nil {
		return false, err
	}
	if binding.State != models.EnvironmentDNSReconciling {
		if err := service.enqueueTx(ctx, tx, binding); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (service *EnvironmentDNS) ConfirmAdoption(ctx context.Context, environmentID uuid.UUID) error {
	return service.requeue(ctx, environmentID, true)
}

func (service *EnvironmentDNS) Retry(ctx context.Context, environmentID uuid.UUID) error {
	return service.requeue(ctx, environmentID, false)
}

func (service *EnvironmentDNS) Refresh(ctx context.Context, environmentID uuid.UUID) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	binding, err := models.EnvironmentDNSBinding.ActiveForEnvironment(ctx, tx, environmentID)
	if err != nil {
		return err
	}
	if binding.State == models.EnvironmentDNSReconciling {
		return errors.Join(
			models.ErrDomainValidation,
			errors.New("DNS reconciliation is already in progress"),
		)
	}
	if binding.State == models.EnvironmentDNSRemoving ||
		binding.State == models.EnvironmentDNSRemovalFailed {
		return errors.Join(
			models.ErrDomainValidation,
			errors.New("DNS removal is in progress; wait for it to finish"),
		)
	}
	binding, err = models.EnvironmentDNSBinding.Refresh(ctx, tx, binding.ID)
	if err != nil {
		return err
	}
	if err := service.enqueueTx(ctx, tx, binding); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *EnvironmentDNS) requeue(
	ctx context.Context,
	environmentID uuid.UUID,
	adopt bool,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	binding, err := models.EnvironmentDNSBinding.ActiveForEnvironment(ctx, tx, environmentID)
	if err != nil {
		return err
	}
	if adopt {
		if binding.State != models.EnvironmentDNSConflict {
			return errors.Join(
				models.ErrDomainValidation,
				errors.New("DNS binding is not waiting for adoption"),
			)
		}
		binding, err = models.EnvironmentDNSBinding.ConfirmAdoption(ctx, tx, binding.ID)
	} else {
		if binding.State != models.EnvironmentDNSFailed &&
			binding.State != models.EnvironmentDNSConflict &&
			binding.State != models.EnvironmentDNSRemovalFailed {
			return errors.Join(
				models.ErrDomainValidation,
				errors.New("DNS binding is not retryable"),
			)
		}
		if binding.State == models.EnvironmentDNSRemovalFailed {
			binding, err = models.EnvironmentDNSBinding.RetryRemoval(ctx, tx, binding.ID)
		} else {
			binding, err = models.EnvironmentDNSBinding.Retry(ctx, tx, binding.ID)
		}
	}
	if err != nil {
		return err
	}
	if err := service.enqueueTx(ctx, tx, binding); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *EnvironmentDNS) Reconcile(
	ctx context.Context,
	bindingID uuid.UUID,
	generation int64,
) (*DNSDeploymentIntent, error) {
	scope, err := service.scope(ctx, bindingID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if scope.Generation != generation {
		return nil, nil
	}
	if scope.State == models.EnvironmentDNSApplied {
		return deploymentIntent(scope), nil
	}
	if scope.State == models.EnvironmentDNSRemoving ||
		scope.State == models.EnvironmentDNSRemovalFailed {
		if err := service.removeTracked(ctx, bindingID, nil); err != nil {
			return nil, service.failRemoval(ctx, bindingID, generation, err)
		}
		now := time.Now().UTC()
		tx, err := service.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		if err := models.EnvironmentDNSRecord.ArchiveMissing(
			ctx,
			tx,
			bindingID,
			nil,
			now,
		); err != nil {
			return nil, err
		}
		if err := models.EnvironmentDNSBinding.ArchiveGeneration(
			ctx, tx, bindingID, generation, now,
		); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return deploymentIntent(scope), nil
	}
	token, err := secretcrypto.DecryptForPurpose(
		scope.CredentialPayload,
		service.config.App.SessionEncryptionKey,
		cloudflareTokenEncryptionPurpose,
	)
	if err != nil {
		return nil, service.fail(
			ctx,
			bindingID,
			generation,
			errors.New("Cloudflare account-owned API token could not be decrypted"),
		)
	}
	claimed, err := service.markReconciling(ctx, bindingID, generation)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, nil
	}
	desired, err := service.desiredIPv4(ctx, service.db.Executor(), scope.EnvironmentID)
	if err != nil {
		return nil, service.fail(ctx, bindingID, generation, err)
	}
	tracked, err := models.EnvironmentDNSRecord.ActiveForBinding(
		ctx,
		service.db.Executor(),
		bindingID,
	)
	if err != nil {
		return nil, service.fail(ctx, bindingID, generation, err)
	}
	trackedIDs := make([]string, 0, len(tracked))
	for _, record := range tracked {
		if record.DNSZoneID == scope.ZoneID &&
			strings.EqualFold(record.ObservedName, scope.Hostname) {
			trackedIDs = append(trackedIDs, record.ExternalID)
		}
	}
	reconciliation, err := cloudflareclient.ReconcileARecords(
		ctx,
		service.client,
		cloudflareclient.ARecordReconciliationInput{
			Token:            string(token),
			ZoneID:           scope.ZoneExternalID,
			Hostname:         scope.Hostname,
			DesiredIPv4:      desired,
			OwnershipMarker:  ownershipMarker(bindingID),
			TrackedRecordIDs: trackedIDs,
			AdoptUnmanaged:   scope.AdoptionConfirmed.Valid,
		},
	)
	if err != nil {
		return nil, service.fail(ctx, bindingID, generation, err)
	}
	if reconciliation.BlockedByUnmanaged {
		message := fmt.Sprintf(
			"%d unmanaged Cloudflare address record(s) already use %s",
			len(reconciliation.Classification.Unmanaged),
			scope.Hostname,
		)
		if err := service.markState(
			ctx,
			bindingID,
			generation,
			models.EnvironmentDNSConflict,
			errors.New(message),
		); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err := service.removeTracked(ctx, bindingID, func(record dnsTrackedRemoval) bool {
		return record.ZoneID != scope.ZoneID ||
			!strings.EqualFold(record.ObservedName, scope.Hostname)
	}); err != nil {
		return nil, service.fail(ctx, bindingID, generation, err)
	}
	now := time.Now().UTC()
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	externalIDs := make([]string, 0, len(reconciliation.Applied))
	for _, record := range reconciliation.Applied {
		externalIDs = append(externalIDs, record.ID)
		if _, err := models.EnvironmentDNSRecord.Upsert(
			ctx,
			tx,
			models.UpsertEnvironmentDNSRecordData{
				ExternalID: record.ID, Content: record.Content, ObservedName: record.Name,
				EnvironmentDNSBindingID: bindingID, DNSZoneID: scope.ZoneID,
			},
		); err != nil {
			return nil, err
		}
	}
	if err := models.EnvironmentDNSRecord.ArchiveMissing(
		ctx,
		tx,
		bindingID,
		externalIDs,
		now,
	); err != nil {
		return nil, err
	}
	markedApplied, err := models.EnvironmentDNSBinding.MarkApplied(
		ctx, tx, bindingID, generation, now,
	)
	if err != nil {
		return nil, err
	}
	if !markedApplied {
		return nil, tx.Rollback()
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return service.currentDeploymentIntent(ctx, bindingID, generation)
}

func (service *EnvironmentDNS) currentDeploymentIntent(
	ctx context.Context,
	bindingID uuid.UUID,
	generation int64,
) (*DNSDeploymentIntent, error) {
	scope, err := service.scope(ctx, bindingID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if scope.Generation != generation {
		return nil, nil
	}
	return deploymentIntent(scope), nil
}

func deploymentIntent(scope dnsReconciliationScope) *DNSDeploymentIntent {
	if !scope.DeployAfterApply || scope.DeploymentSentAt.Valid {
		return nil
	}
	return &DNSDeploymentIntent{
		BindingID:     scope.BindingID,
		Generation:    scope.Generation,
		ApplicationID: scope.ApplicationID,
		EnvironmentID: scope.EnvironmentID,
		ActorID:       scope.DeploymentActorID,
		TriggerType:   scope.DeploymentTrigger,
		Reference:     scope.DeploymentReference,
	}
}

func (service *EnvironmentDNS) MarkDeploymentDispatched(
	ctx context.Context,
	bindingID uuid.UUID,
	generation int64,
) error {
	return models.EnvironmentDNSBinding.MarkDeploymentDispatched(
		ctx, service.db.Executor(), bindingID, generation, time.Now().UTC(),
	)
}

func (service *EnvironmentDNS) RemoveForEnvironment(
	ctx context.Context,
	environmentID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	binding, err := models.EnvironmentDNSBinding.ActiveForEnvironment(ctx, tx, environmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	binding, err = models.EnvironmentDNSBinding.MarkRemoving(ctx, tx, binding.ID, false, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("DNS reconciliation is in progress; retry deletion shortly")
	}
	if err != nil {
		return err
	}
	if err := service.enqueueTx(ctx, tx, binding); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return service.removeTracked(ctx, binding.ID, nil)
}

func (service *EnvironmentDNS) enqueueTx(
	ctx context.Context,
	tx bun.Tx,
	binding models.EnvironmentDNSBindingEntity,
) error {
	_, err := service.queue.InsertTx(
		ctx,
		tx.Tx,
		jobs.DNSReconciliationArgs{BindingID: binding.ID, Generation: binding.Generation},
		jobs.DNSReconciliationInsertOpts(binding.ID, binding.Generation),
	)
	return err
}

func (service *EnvironmentDNS) scope(
	ctx context.Context,
	bindingID uuid.UUID,
) (dnsReconciliationScope, error) {
	return models.EnvironmentDNSBinding.ReconciliationScope(
		ctx, service.db.Executor(), bindingID,
	)
}

func (service *EnvironmentDNS) desiredIPv4(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
) ([]string, error) {
	servers, err := models.EnvironmentTarget.ActiveDNSServerAddresses(ctx, db, environmentID)
	if err != nil {
		return nil, err
	}
	unique := make([]string, 0, len(servers))
	seen := make(map[string]struct{}, len(servers))
	for _, entry := range servers {
		value, err := resolveDNSServerPublicIPv4(ctx, db, entry)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			unique = append(unique, value)
		}
	}
	if len(unique) == 0 {
		return nil, errors.Join(
			models.ErrDomainValidation,
			errors.New("Cloudflare-managed DNS requires at least one runtime Server IPv4 address"),
		)
	}
	sort.Strings(unique)
	return unique, nil
}

func (service *EnvironmentDNS) markReconciling(
	ctx context.Context,
	bindingID uuid.UUID,
	generation int64,
) (bool, error) {
	return models.EnvironmentDNSBinding.MarkReconciling(
		ctx, service.db.Executor(), bindingID, generation, time.Now().UTC(),
	)
}

func (service *EnvironmentDNS) markState(
	ctx context.Context,
	bindingID uuid.UUID,
	generation int64,
	state string,
	stateErr error,
) error {
	message := ""
	if stateErr != nil {
		message = strings.TrimSpace(stateErr.Error())
		if len(message) > 1000 {
			message = message[:1000]
		}
	}
	return models.EnvironmentDNSBinding.MarkState(
		ctx, service.db.Executor(), bindingID, generation, state, message, time.Now().UTC(),
	)
}

func (service *EnvironmentDNS) fail(
	ctx context.Context,
	bindingID uuid.UUID,
	generation int64,
	cause error,
) error {
	if err := service.markState(
		ctx,
		bindingID,
		generation,
		models.EnvironmentDNSFailed,
		cause,
	); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (service *EnvironmentDNS) failRemoval(
	ctx context.Context,
	bindingID uuid.UUID,
	generation int64,
	cause error,
) error {
	if err := service.markState(
		ctx,
		bindingID,
		generation,
		models.EnvironmentDNSRemovalFailed,
		cause,
	); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (service *EnvironmentDNS) removeTracked(
	ctx context.Context,
	bindingID uuid.UUID,
	remove func(dnsTrackedRemoval) bool,
) error {
	records, err := models.EnvironmentDNSRecord.TrackedRemovals(
		ctx, service.db.Executor(), bindingID,
	)
	if err != nil {
		return err
	}
	for _, record := range records {
		if remove != nil && !remove(record) {
			continue
		}
		token, err := secretcrypto.DecryptForPurpose(
			record.CredentialPayload,
			service.config.App.SessionEncryptionKey,
			cloudflareTokenEncryptionPurpose,
		)
		if err != nil {
			return errors.New("Cloudflare account-owned API token could not be decrypted")
		}
		if err := service.client.DeleteRecord(
			ctx,
			string(token),
			record.ZoneExternalID,
			record.ExternalID,
		); err != nil {
			return err
		}
	}
	return nil
}

func hostnameBelongsToZone(hostname, zone string) bool {
	hostname = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(hostname), "."))
	zone = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(zone), "."))
	return hostname == zone || strings.HasSuffix(hostname, "."+zone)
}

func ownershipMarker(bindingID uuid.UUID) string {
	return "deploycrate:dns-binding:" + bindingID.String()
}
