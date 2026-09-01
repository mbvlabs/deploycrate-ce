package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	EnvironmentDNSPending       = "pending"
	EnvironmentDNSReconciling   = "reconciling"
	EnvironmentDNSApplied       = "applied"
	EnvironmentDNSConflict      = "conflict"
	EnvironmentDNSFailed        = "failed"
	EnvironmentDNSRemoving      = "removing"
	EnvironmentDNSRemovalFailed = "removal_failed"
)

var environmentDNSStates = []string{
	EnvironmentDNSPending,
	EnvironmentDNSReconciling,
	EnvironmentDNSApplied,
	EnvironmentDNSConflict,
	EnvironmentDNSFailed,
	EnvironmentDNSRemoving,
	EnvironmentDNSRemovalFailed,
}

type EnvironmentDNSBindingEntity struct {
	bun.BaseModel          `bun:"table:environment_dns_bindings,alias:environment_dns_bindings"`
	ID                     uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt              time.Time      `bun:"created_at"`
	UpdatedAt              time.Time      `bun:"updated_at"`
	State                  string         `bun:"state"`
	Generation             int64          `bun:"generation"`
	AppliedGeneration      int64          `bun:"applied_generation"`
	LastError              sql.NullString `bun:"last_error"`
	AdoptionConfirmedAt    sql.NullTime   `bun:"adoption_confirmed_at"`
	DeployAfterApply       bool           `bun:"deploy_after_apply"`
	DeploymentActorID      *uuid.UUID     `bun:"deployment_actor_id,type:uuid"`
	DeploymentTriggerType  string         `bun:"deployment_trigger_type"`
	DeploymentReference    string         `bun:"deployment_reference"`
	DeploymentDispatchedAt sql.NullTime   `bun:"deployment_dispatched_at"`
	AppliedAt              sql.NullTime   `bun:"applied_at"`
	ArchivedAt             sql.NullTime   `bun:"archived_at"`
	EnvironmentDomainID    uuid.UUID      `bun:"environment_domain_id,type:uuid"`
	DNSZoneID              uuid.UUID      `bun:"dns_zone_id,type:uuid"`
}

type EnvironmentDNSStatusRow struct {
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

type DNSReconciliationScope struct {
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

func (environmentDNSBinding) StatusForEnvironment(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
) (EnvironmentDNSStatusRow, error) {
	var row EnvironmentDNSStatusRow
	err := db.NewSelect().TableExpr("environment_dns_bindings AS binding").
		ColumnExpr("binding.id AS binding_id, zone.id AS zone_id, zone.name AS zone_name, connection.name AS connection_name").
		ColumnExpr("binding.state, binding.generation, binding.applied_generation, binding.last_error, binding.applied_at").
		Join("JOIN environment_domains AS domain ON domain.id = binding.environment_domain_id AND domain.archived_at IS NULL").
		Join("JOIN dns_zones AS zone ON zone.id = binding.dns_zone_id").
		Join("JOIN dns_connections AS connection ON connection.id = zone.dns_connection_id").
		Where("domain.environment_id = ?", environmentID).
		Where("binding.archived_at IS NULL").Limit(1).Scan(ctx, &row)
	return row, err
}

func (environmentDNSBinding) ReconciliationScope(
	ctx context.Context,
	db storage.Executor,
	bindingID uuid.UUID,
) (DNSReconciliationScope, error) {
	var scope DNSReconciliationScope
	err := db.NewSelect().TableExpr("environment_dns_bindings AS binding").
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

func (environmentDNSBinding) ArchiveGeneration(
	ctx context.Context, db storage.Executor, id uuid.UUID, generation int64, at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("environment_dns_bindings").
		Set("updated_at = ?", at).Set("archived_at = ?", at).Set("last_error = NULL").
		Where("id = ?", id).Where("generation = ?", generation).Exec(ctx)
	return err
}

func (environmentDNSBinding) MarkApplied(
	ctx context.Context, db storage.Executor, id uuid.UUID, generation int64, at time.Time,
) (bool, error) {
	result, err := db.NewUpdate().TableExpr("environment_dns_bindings").
		Set("updated_at = ?", at).Set("state = ?", EnvironmentDNSApplied).
		Set("applied_generation = generation").Set("applied_at = ?", at).Set("last_error = NULL").
		Where("id = ?", id).Where("generation = ?", generation).Exec(ctx)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (environmentDNSBinding) MarkDeploymentDispatched(
	ctx context.Context, db storage.Executor, id uuid.UUID, generation int64, at time.Time,
) error {
	_, err := db.NewUpdate().TableExpr("environment_dns_bindings").
		Set("updated_at = ?", at).Set("deployment_dispatched_at = ?", at).
		Where("id = ?", id).Where("generation = ?", generation).
		Where("deployment_dispatched_at IS NULL").Exec(ctx)
	return err
}

func (environmentDNSBinding) MarkReconciling(
	ctx context.Context, db storage.Executor, id uuid.UUID, generation int64, at time.Time,
) (bool, error) {
	result, err := db.NewUpdate().TableExpr("environment_dns_bindings").
		Set("updated_at = ?", at).Set("state = ?", EnvironmentDNSReconciling).
		Set("last_error = NULL").Where("id = ?", id).Where("generation = ?", generation).
		Where("state IN ('pending', 'failed', 'conflict', 'reconciling')").Exec(ctx)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (environmentDNSBinding) MarkState(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	generation int64,
	state, message string,
	at time.Time,
) error {
	query := db.NewUpdate().TableExpr("environment_dns_bindings").
		Set("updated_at = ?", at).Set("state = ?", state).
		Where("id = ?", id).Where("generation = ?", generation)
	if message == "" {
		query = query.Set("last_error = NULL")
	} else {
		query = query.Set("last_error = ?", message)
	}
	_, err := query.Exec(ctx)
	return err
}

func (entity *EnvironmentDNSBindingEntity) Validate() error {
	builder := validation.NewBuilder()
	if !slices.Contains(environmentDNSStates, entity.State) {
		builder.Add("state", "invalid", "DNS binding state is invalid")
	}
	if entity.Generation < 1 || entity.AppliedGeneration < 0 ||
		entity.AppliedGeneration > entity.Generation {
		builder.Add("generation", "invalid", "DNS binding generation is invalid")
	}
	if entity.EnvironmentDomainID == uuid.Nil {
		builder.Add("environmentDomainId", "required", "Environment domain is required")
	}
	if entity.DNSZoneID == uuid.Nil {
		builder.Add("dnsZoneId", "required", "DNS zone is required")
	}
	return builder.Err()
}

type CreateEnvironmentDNSBindingData struct {
	DNSZoneID           uuid.UUID
	EnvironmentDomainID uuid.UUID
	DeployAfterApply    bool
	DeploymentActorID   *uuid.UUID
}

func (environmentDNSBinding) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentDNSBindingData,
) (EnvironmentDNSBindingEntity, error) {
	now := time.Now().UTC()
	entity := EnvironmentDNSBindingEntity{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, State: EnvironmentDNSPending,
		Generation: 1, AppliedGeneration: 0, DNSZoneID: data.DNSZoneID,
		EnvironmentDomainID: data.EnvironmentDomainID, DeployAfterApply: data.DeployAfterApply,
		DeploymentActorID: data.DeploymentActorID, DeploymentTriggerType: "user",
	}
	if err := validation.Validate(&entity); err != nil {
		return EnvironmentDNSBindingEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureActiveUnique(
		ctx,
		db,
		"environment-dns-binding-domain:"+entity.EnvironmentDomainID.String(),
		entity.ID,
		db.NewSelect().
			Model((*EnvironmentDNSBindingEntity)(nil)).
			Where("environment_domain_id = ?", entity.EnvironmentDomainID),
		"environmentDomainId",
		"the Environment domain already has an active DNS binding",
	); err != nil {
		return EnvironmentDNSBindingEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentDNSBindingEntity{}, err
	}
	return entity, nil
}

func (environmentDNSBinding) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentDNSBindingEntity, error) {
	var entity EnvironmentDNSBindingEntity
	if err := db.NewSelect().Model(&entity).Where("id = ?", id).Scan(ctx); err != nil {
		return EnvironmentDNSBindingEntity{}, err
	}
	return entity, nil
}

func (environmentDNSBinding) ActiveForDomain(
	ctx context.Context,
	db storage.Executor,
	domainID uuid.UUID,
) (EnvironmentDNSBindingEntity, error) {
	var entity EnvironmentDNSBindingEntity
	if err := db.NewSelect().Model(&entity).Where("environment_domain_id = ?", domainID).
		Where("archived_at IS NULL").Limit(1).Scan(ctx); err != nil {
		return EnvironmentDNSBindingEntity{}, err
	}
	return entity, nil
}

func (environmentDNSBinding) ActiveForEnvironment(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
) (EnvironmentDNSBindingEntity, error) {
	var entity EnvironmentDNSBindingEntity
	if err := db.NewSelect().Model(&entity).
		Join("JOIN environment_domains AS domain ON domain.id = environment_dns_bindings.environment_domain_id").
		Where("domain.environment_id = ?", environmentID).Where("domain.archived_at IS NULL").
		Where("environment_dns_bindings.archived_at IS NULL").Limit(1).Scan(ctx); err != nil {
		return EnvironmentDNSBindingEntity{}, err
	}
	return entity, nil
}

func (environmentDNSBinding) Reconfigure(
	ctx context.Context,
	db storage.Executor,
	id, zoneID uuid.UUID,
	deployAfterApply bool,
	actorID *uuid.UUID,
) (EnvironmentDNSBindingEntity, error) {
	var entity EnvironmentDNSBindingEntity
	now := time.Now().UTC()
	if err := db.NewUpdate().Model(&entity).
		Set("updated_at = ?", now).Set("dns_zone_id = ?", zoneID).
		Set("state = ?", EnvironmentDNSPending).Set("generation = generation + 1").
		Set("last_error = NULL").Set("adoption_confirmed_at = NULL").Set("applied_at = NULL").
		Set("deploy_after_apply = ?", deployAfterApply).Set("deployment_actor_id = ?", actorID).
		Set("deployment_trigger_type = 'user'").Set("deployment_reference = ''").
		Set("deployment_dispatched_at = NULL").
		Where("id = ?", id).
		Where("archived_at IS NULL").Where("state <> ?", EnvironmentDNSReconciling).
		Returning("*").Scan(ctx); err != nil {
		return EnvironmentDNSBindingEntity{}, err
	}
	return entity, nil
}

func (environmentDNSBinding) MarkRemoving(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	deployAfterApply bool,
	actorID *uuid.UUID,
) (EnvironmentDNSBindingEntity, error) {
	var entity EnvironmentDNSBindingEntity
	now := time.Now().UTC()
	if err := db.NewUpdate().Model(&entity).
		Set("updated_at = ?", now).Set("state = ?", EnvironmentDNSRemoving).
		Set("generation = generation + 1").Set("last_error = NULL").
		Set("deploy_after_apply = ?", deployAfterApply).Set("deployment_actor_id = ?", actorID).
		Set("deployment_trigger_type = 'user'").Set("deployment_reference = ''").
		Set("deployment_dispatched_at = NULL").
		Where("id = ?", id).
		Where("archived_at IS NULL").Where("state <> ?", EnvironmentDNSReconciling).
		Returning("*").Scan(ctx); err != nil {
		return EnvironmentDNSBindingEntity{}, err
	}
	return entity, nil
}

func (environmentDNSBinding) ConfirmAdoption(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentDNSBindingEntity, error) {
	var entity EnvironmentDNSBindingEntity
	now := time.Now().UTC()
	if err := db.NewUpdate().Model(&entity).
		Set("updated_at = ?", now).Set("state = ?", EnvironmentDNSPending).
		Set("generation = generation + 1").
		Set("last_error = NULL").Set("adoption_confirmed_at = ?", now).
		Where("id = ?", id).Where("archived_at IS NULL").Returning("*").Scan(ctx); err != nil {
		return EnvironmentDNSBindingEntity{}, err
	}
	return entity, nil
}

func (environmentDNSBinding) PrepareDeployment(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
	actorID *uuid.UUID,
	triggerType, reference string,
) (EnvironmentDNSBindingEntity, error) {
	var entity EnvironmentDNSBindingEntity
	now := time.Now().UTC()
	if err := db.NewUpdate().Model(&entity).
		Set("updated_at = ?", now).Set("deploy_after_apply = TRUE").
		Set("deployment_actor_id = ?", actorID).Set("deployment_trigger_type = ?", triggerType).
		Set("deployment_reference = ?", reference).Set("deployment_dispatched_at = NULL").
		Where("id = ?", id).Where("archived_at IS NULL").
		Where("state IN (?, ?, ?, ?)", EnvironmentDNSPending, EnvironmentDNSReconciling, EnvironmentDNSApplied, EnvironmentDNSRemoving).
		Returning("*").Scan(ctx); err != nil {
		return EnvironmentDNSBindingEntity{}, err
	}
	return entity, nil
}

func (environmentDNSBinding) Retry(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentDNSBindingEntity, error) {
	var entity EnvironmentDNSBindingEntity
	now := time.Now().UTC()
	if err := db.NewUpdate().Model(&entity).
		Set("updated_at = ?", now).Set("state = ?", EnvironmentDNSPending).
		Set("generation = generation + 1").Set("last_error = NULL").
		Where("id = ?", id).Where("archived_at IS NULL").Returning("*").Scan(ctx); err != nil {
		return EnvironmentDNSBindingEntity{}, err
	}
	return entity, nil
}

func (environmentDNSBinding) Refresh(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentDNSBindingEntity, error) {
	var entity EnvironmentDNSBindingEntity
	now := time.Now().UTC()
	if err := db.NewUpdate().Model(&entity).
		Set("updated_at = ?", now).Set("state = ?", EnvironmentDNSPending).
		Set("generation = generation + 1").Set("last_error = NULL").Set("applied_at = NULL").
		Where("id = ?", id).
		Where("archived_at IS NULL").Where("state <> ?", EnvironmentDNSReconciling).
		Returning("*").Scan(ctx); err != nil {
		return EnvironmentDNSBindingEntity{}, err
	}
	return entity, nil
}

func (environmentDNSBinding) RetryRemoval(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentDNSBindingEntity, error) {
	var entity EnvironmentDNSBindingEntity
	now := time.Now().UTC()
	if err := db.NewUpdate().Model(&entity).
		Set("updated_at = ?", now).Set("state = ?", EnvironmentDNSRemoving).
		Set("generation = generation + 1").Set("last_error = NULL").
		Where("id = ?", id).Where("archived_at IS NULL").Returning("*").Scan(ctx); err != nil {
		return EnvironmentDNSBindingEntity{}, err
	}
	return entity, nil
}
