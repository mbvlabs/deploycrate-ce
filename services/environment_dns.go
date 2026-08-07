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
	"deploycrate-ce/telemetry"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
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

type EnvironmentDNSOption struct {
	ZoneID         uuid.UUID `json:"zoneId"         bun:"zone_id"`
	ZoneName       string    `json:"zoneName"       bun:"zone_name"`
	ConnectionID   uuid.UUID `json:"connectionId"   bun:"connection_id"`
	ConnectionName string    `json:"connectionName" bun:"connection_name"`
}

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

type DNSRecordStatus struct {
	Type    string `json:"type"    bun:"record_type"`
	Name    string `json:"name"    bun:"observed_name"`
	Content string `json:"content" bun:"content"`
}

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

type dnsReconciliationScope struct {
	BindingID           uuid.UUID    `bun:"binding_id"`
	Generation          int64        `bun:"generation"`
	State               string       `bun:"state"`
	AdoptionConfirmed   sql.NullTime `bun:"adoption_confirmed_at"`
	DeployAfterApply    bool         `bun:"deploy_after_apply"`
	DeploymentActorID   *uuid.UUID   `bun:"deployment_actor_id"`
	DeploymentTrigger   string       `bun:"deployment_trigger_type"`
	DeploymentReference string       `bun:"deployment_reference"`
	DeploymentSentAt    sql.NullTime `bun:"deployment_dispatched_at"`
	DomainID            uuid.UUID    `bun:"domain_id"`
	Hostname            string       `bun:"hostname"`
	EnvironmentID       uuid.UUID    `bun:"environment_id"`
	ApplicationID       uuid.UUID    `bun:"application_id"`
	ZoneID              uuid.UUID    `bun:"zone_id"`
	ZoneExternalID      string       `bun:"zone_external_id"`
	ConnectionID        uuid.UUID    `bun:"connection_id"`
	CredentialPayload   []byte       `bun:"credential_payload"`
}

type dnsTrackedRemoval struct {
	RecordID          uuid.UUID `bun:"record_id"`
	ExternalID        string    `bun:"external_id"`
	ObservedName      string    `bun:"observed_name"`
	ZoneID            uuid.UUID `bun:"zone_id"`
	ZoneExternalID    string    `bun:"zone_external_id"`
	CredentialPayload []byte    `bun:"credential_payload"`
}

func NewEnvironmentDNS(
	db storage.Pool,
	queue storage.InsertQueue,
	client CloudflareDNSClient,
	cfg config.Config,
) *EnvironmentDNS {
	return &EnvironmentDNS{db: db, queue: queue, client: client, config: cfg}
}

func (service *EnvironmentDNS) Options(ctx context.Context) ([]EnvironmentDNSOption, error) {
	options := make([]EnvironmentDNSOption, 0)
	err := service.db.Executor().NewSelect().TableExpr("dns_zones AS zone").
		ColumnExpr("zone.id AS zone_id, zone.name AS zone_name, connection.id AS connection_id, connection.name AS connection_name").
		Join("JOIN dns_connections AS connection ON connection.id = zone.dns_connection_id AND connection.archived_at IS NULL").
		Where("zone.archived_at IS NULL").Where("zone.status = 'active'").
		OrderExpr("lower(connection.name), lower(zone.name)").Scan(ctx, &options)
	return options, err
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
	var row struct {
		BindingID         uuid.UUID      `bun:"binding_id"`
		ZoneID            uuid.UUID      `bun:"zone_id"`
		ZoneName          string         `bun:"zone_name"`
		ConnectionName    string         `bun:"connection_name"`
		State             string         `bun:"state"`
		Generation        int64          `bun:"generation"`
		AppliedGeneration int64          `bun:"applied_generation"`
		LastError         sql.NullString `bun:"last_error"`
		AppliedAt         sql.NullTime   `bun:"applied_at"`
	}
	err := service.db.Executor().NewSelect().TableExpr("environment_dns_bindings AS binding").
		ColumnExpr("binding.id AS binding_id, zone.id AS zone_id, zone.name AS zone_name, connection.name AS connection_name").
		ColumnExpr("binding.state, binding.generation, binding.applied_generation, binding.last_error, binding.applied_at").
		Join("JOIN environment_domains AS domain ON domain.id = binding.environment_domain_id AND domain.archived_at IS NULL").
		Join("JOIN dns_zones AS zone ON zone.id = binding.dns_zone_id").
		Join("JOIN dns_connections AS connection ON connection.id = zone.dns_connection_id").
		Where("domain.environment_id = ?", environmentID).
		Where("binding.archived_at IS NULL").Limit(1).Scan(ctx, &row)
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
	queued, err := service.db.Executor().NewSelect().TableExpr("river_job AS job").
		Where("job.kind = 'dns_reconciliation'").
		Where("job.args ->> 'binding_id' = ?", row.BindingID.String()).
		Where("job.args ->> 'generation' = ?", fmt.Sprint(row.Generation)).
		Where("job.state::text IN ('available', 'pending', 'retryable', 'running', 'scheduled')").
		Count(ctx)
	if err != nil {
		return EnvironmentDNSStatus{}, err
	}
	status.ReconciliationQueued = queued > 0
	if err := service.db.Executor().NewSelect().TableExpr("environment_dns_records").
		ColumnExpr("record_type, observed_name, content").
		Where("environment_dns_binding_id = ?", row.BindingID).
		Where("archived_at IS NULL").OrderExpr("content").Scan(ctx, &status.Records); err != nil {
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
		if _, err := tx.NewUpdate().
			TableExpr("environment_dns_bindings").
			Set("updated_at = ?", now).
			Set("archived_at = ?", now).
			Set("last_error = NULL").
			Where("id = ?", bindingID).
			Where("generation = ?", generation).
			Exec(ctx); err != nil {
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
	remote, err := service.client.ListAddressRecords(
		ctx,
		string(token),
		scope.ZoneExternalID,
		scope.Hostname,
	)
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
	trackedIDs := make(map[string]struct{}, len(tracked))
	for _, record := range tracked {
		if record.DNSZoneID == scope.ZoneID &&
			strings.EqualFold(record.ObservedName, scope.Hostname) {
			trackedIDs[record.ExternalID] = struct{}{}
		}
	}
	marker := ownershipMarker(bindingID)
	owned := make([]cloudflareclient.DNSRecord, 0, len(remote))
	unmanaged := make([]cloudflareclient.DNSRecord, 0)
	for _, record := range remote {
		_, tracked := trackedIDs[record.ID]
		if tracked || record.Comment == marker || scope.AdoptionConfirmed.Valid {
			owned = append(owned, record)
		} else {
			unmanaged = append(unmanaged, record)
		}
	}
	if len(unmanaged) > 0 {
		message := fmt.Sprintf(
			"%d unmanaged Cloudflare address record(s) already use %s",
			len(unmanaged),
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
	sort.Slice(owned, func(i, j int) bool { return owned[i].ID < owned[j].ID })
	applied := make([]cloudflareclient.DNSRecord, 0, len(desired))
	for index, address := range desired {
		input := cloudflareclient.DNSRecordInput{
			Type:    "A",
			Name:    scope.Hostname,
			Content: address,
			TTL:     1,
			Proxied: true,
			Comment: marker,
		}
		var record cloudflareclient.DNSRecord
		if index < len(owned) && strings.EqualFold(owned[index].Type, "A") {
			record, err = service.client.UpdateARecord(
				ctx,
				string(token),
				scope.ZoneExternalID,
				owned[index].ID,
				input,
			)
		} else {
			if index < len(owned) {
				err = service.client.DeleteRecord(
					ctx,
					string(token),
					scope.ZoneExternalID,
					owned[index].ID,
				)
			}
			if err == nil {
				record, err = service.client.CreateARecord(
					ctx,
					string(token),
					scope.ZoneExternalID,
					input,
				)
			}
		}
		if err != nil {
			return nil, service.fail(ctx, bindingID, generation, err)
		}
		applied = append(applied, record)
	}
	for index := len(desired); index < len(owned); index++ {
		if err := service.client.DeleteRecord(
			ctx,
			string(token),
			scope.ZoneExternalID,
			owned[index].ID,
		); err != nil {
			return nil, service.fail(ctx, bindingID, generation, err)
		}
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
	externalIDs := make([]string, 0, len(applied))
	for _, record := range applied {
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
	result, err := tx.NewUpdate().TableExpr("environment_dns_bindings").Set("updated_at = ?", now).
		Set("state = ?", models.EnvironmentDNSApplied).Set("applied_generation = generation").
		Set("applied_at = ?", now).Set("last_error = NULL").Where("id = ?", bindingID).
		Where("generation = ?", generation).Exec(ctx)
	if err != nil {
		return nil, err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
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
	_, err := service.db.Executor().NewUpdate().TableExpr("environment_dns_bindings").
		Set("updated_at = ?", time.Now().UTC()).
		Set("deployment_dispatched_at = ?", time.Now().UTC()).
		Where("id = ?", bindingID).Where("generation = ?", generation).
		Where("deployment_dispatched_at IS NULL").Exec(ctx)
	return err
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
	var scope dnsReconciliationScope
	err := service.db.Executor().NewSelect().TableExpr("environment_dns_bindings AS binding").
		ColumnExpr("binding.id AS binding_id, binding.generation, binding.state, binding.adoption_confirmed_at, binding.deploy_after_apply, binding.deployment_actor_id, binding.deployment_trigger_type, binding.deployment_reference, binding.deployment_dispatched_at").
		ColumnExpr("domain.id AS domain_id, domain.hostname, domain.environment_id, environment.application_id").
		ColumnExpr("zone.id AS zone_id, zone.external_id AS zone_external_id, connection.id AS connection_id, credential.enc_payload AS credential_payload").
		Join("JOIN environment_domains AS domain ON domain.id = binding.environment_domain_id").
		Join("JOIN environments AS environment ON environment.id = domain.environment_id").
		Join("JOIN dns_zones AS zone ON zone.id = binding.dns_zone_id").
		Join("JOIN dns_connections AS connection ON connection.id = zone.dns_connection_id AND connection.archived_at IS NULL").
		Join("JOIN credentials AS credential ON credential.id = connection.credential_id AND credential.archived_at IS NULL").
		Where("binding.id = ?", bindingID).Where("binding.archived_at IS NULL").Scan(ctx, &scope)
	return scope, err
}

type dnsServerAddress struct {
	ID   uuid.UUID `bun:"id"`
	Kind string    `bun:"kind"`
	IPv4 string    `bun:"ipv4_address"`
	Addr string    `bun:"address"`
}

func (service *EnvironmentDNS) desiredIPv4(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
) ([]string, error) {
	servers := make([]dnsServerAddress, 0)
	if err := db.NewSelect().TableExpr("environment_targets AS target").
		ColumnExpr("server.id, server.kind, server.ipv4_address, server.address").
		Join("JOIN servers AS server ON server.id = target.server_id AND server.archived_at IS NULL").
		Where("target.environment_id = ?", environmentID).Where("target.detached_at IS NULL").
		OrderExpr("server.ipv4_address").Scan(ctx, &servers); err != nil {
		return nil, err
	}
	unique := make([]string, 0, len(servers))
	seen := make(map[string]struct{}, len(servers))
	for _, entry := range servers {
		value, err := service.resolvePublicIPv4(ctx, db, entry)
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

func (service *EnvironmentDNS) resolvePublicIPv4(
	ctx context.Context,
	db storage.Executor,
	server dnsServerAddress,
) (string, error) {
	for _, candidate := range []string{server.IPv4, server.Addr} {
		if parsed, err := netip.ParseAddr(
			strings.TrimSpace(candidate),
		); err == nil &&
			isPublicIPv4(parsed) {
			return parsed.String(), nil
		}
	}
	if server.Kind != "self_hosted" {
		return "", errors.Join(
			models.ErrDomainValidation,
			fmt.Errorf(
				"Cloudflare-managed DNS requires a public IPv4 address on runtime Server %s; set one in the Server settings",
				server.ID,
			),
		)
	}
	detected, err := detectHostPublicIPv4(ctx)
	if err != nil {
		return "", errors.Join(models.ErrDomainValidation, err)
	}
	if _, err := db.NewUpdate().
		TableExpr("servers").
		Set("ipv4_address = ?", detected).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", server.ID).
		Exec(ctx); err != nil {
		return "", err
	}
	return detected, nil
}

func detectHostPublicIPv4(ctx context.Context) (string, error) {
	var lastErr error
	for _, endpoint := range []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"} {
		connection, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(
			ctx,
			"tcp4",
			endpoint,
		)
		if err != nil {
			lastErr = err
			continue
		}
		local := connection.LocalAddr()
		_ = connection.Close()
		if tcpAddress, ok := local.(*net.TCPAddr); ok {
			if address, ok := netip.AddrFromSlice(tcpAddress.IP); ok && isPublicIPv4(address) {
				return tcpAddress.IP.String(), nil
			}
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api4.ipify.org", nil)
	if err != nil {
		return "", lastErr
	}
	response, err := telemetry.NewHTTPClient(5 * time.Second).Do(request)
	if err != nil {
		return "", errors.Join(lastErr, err)
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 64))
	if readErr != nil {
		return "", errors.Join(lastErr, readErr)
	}
	value := strings.TrimSpace(string(body))
	if parsed, err := netip.ParseAddr(value); err == nil && isPublicIPv4(parsed) {
		return parsed.String(), nil
	}
	return "", errors.Join(lastErr, errors.New("could not detect the host's public IPv4 address"))
}

func isPublicIPv4(address netip.Addr) bool {
	return address.Is4() && address.IsGlobalUnicast() && !address.IsPrivate() &&
		!address.IsLoopback() &&
		!address.IsLinkLocalUnicast()
}

func (service *EnvironmentDNS) markReconciling(
	ctx context.Context,
	bindingID uuid.UUID,
	generation int64,
) (bool, error) {
	result, err := service.db.Executor().NewUpdate().TableExpr("environment_dns_bindings").
		Set("updated_at = ?", time.Now().UTC()).Set("state = ?", models.EnvironmentDNSReconciling).
		Set("last_error = NULL").Where("id = ?", bindingID).Where("generation = ?", generation).
		Where("state IN ('pending', 'failed', 'conflict', 'reconciling')").Exec(ctx)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
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
	query := service.db.Executor().NewUpdate().TableExpr("environment_dns_bindings").
		Set("updated_at = ?", time.Now().UTC()).Set("state = ?", state).
		Where("id = ?", bindingID).Where("generation = ?", generation)
	if message == "" {
		query = query.Set("last_error = NULL")
	} else {
		query = query.Set("last_error = ?", message)
	}
	_, err := query.Exec(ctx)
	return err
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
	records := make([]dnsTrackedRemoval, 0)
	err := service.db.Executor().NewSelect().TableExpr("environment_dns_records AS record").
		ColumnExpr("record.id AS record_id, record.external_id, record.observed_name, zone.id AS zone_id, zone.external_id AS zone_external_id, credential.enc_payload AS credential_payload").
		Join("JOIN dns_zones AS zone ON zone.id = record.dns_zone_id").
		Join("JOIN dns_connections AS connection ON connection.id = zone.dns_connection_id").
		Join("JOIN credentials AS credential ON credential.id = connection.credential_id").
		Where("record.environment_dns_binding_id = ?", bindingID).
		Where("record.archived_at IS NULL").
		OrderExpr("record.id").Scan(ctx, &records)
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
