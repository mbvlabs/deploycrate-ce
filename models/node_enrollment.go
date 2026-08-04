package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	internalwireguard "deploycrate-ce/internal/wireguard"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	NodeEnrollmentAwaitingConfirmation = "awaiting_confirmation"
	NodeEnrollmentQueued               = "queued"
	NodeEnrollmentInstalling           = "installing"
	NodeEnrollmentVerifying            = "verifying"
	NodeEnrollmentReady                = "ready"
	NodeEnrollmentFailed               = "failed"
)

var nodeEnrollmentStates = []string{
	NodeEnrollmentAwaitingConfirmation,
	NodeEnrollmentQueued,
	NodeEnrollmentInstalling,
	NodeEnrollmentVerifying,
	NodeEnrollmentReady,
	NodeEnrollmentFailed,
}

type NodeEnrollmentEntity struct {
	bun.BaseModel    `bun:"table:node_enrollments,alias:node_enrollments"`
	ID               uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt        time.Time      `bun:"created_at"`
	UpdatedAt        time.Time      `bun:"updated_at"`
	StartedAt        sql.NullTime   `bun:"started_at"`
	CompletedAt      sql.NullTime   `bun:"completed_at"`
	State            string         `bun:"state"`
	CurrentStep      string         `bun:"current_step"`
	Error            sql.NullString `bun:"error"`
	HostFingerprint  string         `bun:"host_fingerprint"`
	AllocatedAddress string         `bun:"allocated_address"`
	InstallerVersion string         `bun:"installer_version"`
	JobID            sql.NullInt64  `bun:"job_id"`
	ServerID         uuid.UUID      `bun:"server_id,type:uuid"`
}

func (entity *NodeEnrollmentEntity) Validate() error {
	entity.CurrentStep = strings.TrimSpace(entity.CurrentStep)
	entity.HostFingerprint = strings.TrimSpace(entity.HostFingerprint)
	entity.AllocatedAddress = strings.TrimSpace(entity.AllocatedAddress)
	entity.InstallerVersion = strings.TrimSpace(entity.InstallerVersion)
	var errs []error
	if entity.ID == uuid.Nil {
		errs = append(errs, errors.New("ID is required"))
	}
	if entity.ServerID == uuid.Nil {
		errs = append(errs, errors.New("Server is required"))
	}
	if !slices.Contains(nodeEnrollmentStates, entity.State) {
		errs = append(errs, fmt.Errorf("state %q is invalid", entity.State))
	}
	if strings.TrimSpace(entity.CurrentStep) == "" {
		errs = append(errs, errors.New("current step is required"))
	}
	if strings.TrimSpace(entity.HostFingerprint) == "" {
		errs = append(errs, errors.New("SSH host fingerprint is required"))
	}
	address, err := netip.ParseAddr(strings.TrimSpace(entity.AllocatedAddress))
	if err != nil || !netip.MustParsePrefix(internalwireguard.NodeCIDR).Contains(address) || address.String() == internalwireguard.ControlPlaneAddress || address == netip.MustParsePrefix(internalwireguard.NodeCIDR).Addr() {
		errs = append(errs, errors.New("allocated address must be a Node WireGuard address"))
	}
	if strings.TrimSpace(entity.InstallerVersion) == "" {
		errs = append(errs, errors.New("installer version is required"))
	}
	return errors.Join(errs...)
}

type CreateNodeEnrollmentData struct {
	HostFingerprint  string
	AllocatedAddress string
	InstallerVersion string
	ServerID         uuid.UUID
}

func (nodeEnrollment) Create(ctx context.Context, db storage.Executor, data CreateNodeEnrollmentData) (NodeEnrollmentEntity, error) {
	now := time.Now().UTC()
	entity := NodeEnrollmentEntity{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now,
		State: NodeEnrollmentAwaitingConfirmation, CurrentStep: "confirm_host_key",
		HostFingerprint: data.HostFingerprint, AllocatedAddress: data.AllocatedAddress,
		InstallerVersion: data.InstallerVersion, ServerID: data.ServerID,
	}
	if err := validation.Validate(&entity); err != nil {
		return NodeEnrollmentEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if err := ensureUnique(
		ctx,
		db,
		"node-enrollment-active-server:"+entity.ServerID.String(),
		db.NewSelect().Model((*NodeEnrollmentEntity)(nil)).
			Where("server_id = ?", entity.ServerID).
			Where("state NOT IN (?, ?)", NodeEnrollmentReady, NodeEnrollmentFailed),
		"serverId",
		"the Server already has an active node enrollment",
	); err != nil {
		return NodeEnrollmentEntity{}, err
	}
	if err := ensureUnique(
		ctx,
		db,
		"node-enrollment-address:"+entity.AllocatedAddress,
		db.NewSelect().Model((*NodeEnrollmentEntity)(nil)).Where("allocated_address = ?", entity.AllocatedAddress),
		"allocatedAddress",
		"the WireGuard address is already allocated to another node enrollment",
	); err != nil {
		return NodeEnrollmentEntity{}, err
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return NodeEnrollmentEntity{}, err
	}
	return entity, nil
}

func (nodeEnrollment) Find(ctx context.Context, db storage.Executor, id uuid.UUID) (NodeEnrollmentEntity, error) {
	var entity NodeEnrollmentEntity
	if err := db.NewSelect().Model(&entity).Where("id = ?", id).Scan(ctx); err != nil {
		return NodeEnrollmentEntity{}, err
	}
	return entity, nil
}

func (nodeEnrollment) LatestForServer(ctx context.Context, db storage.Executor, serverID uuid.UUID) (NodeEnrollmentEntity, error) {
	var entity NodeEnrollmentEntity
	if err := db.NewSelect().Model(&entity).Where("server_id = ?", serverID).
		OrderExpr("created_at DESC").Limit(1).Scan(ctx); err != nil {
		return NodeEnrollmentEntity{}, err
	}
	return entity, nil
}

func (nodeEnrollment) Transition(ctx context.Context, db storage.Executor, id uuid.UUID, state, step string, transitionError error) error {
	now := time.Now().UTC()
	query := db.NewUpdate().Model((*NodeEnrollmentEntity)(nil)).
		Set("updated_at = ?", now).Set("state = ?", state).Set("current_step = ?", step).
		Where("id = ?", id)
	if state == NodeEnrollmentInstalling {
		query = query.Set("started_at = COALESCE(started_at, ?)", now)
	}
	if state == NodeEnrollmentReady || state == NodeEnrollmentFailed {
		query = query.Set("completed_at = ?", now)
	} else {
		query = query.Set("completed_at = NULL")
	}
	if transitionError == nil {
		query = query.Set("error = NULL")
	} else {
		message := strings.TrimSpace(transitionError.Error())
		if len(message) > 1000 {
			message = message[:1000]
		}
		query = query.Set("error = ?", message)
	}
	_, err := query.Exec(ctx)
	return err
}

func (nodeEnrollment) SetJob(ctx context.Context, db storage.Executor, id uuid.UUID, jobID int64) error {
	_, err := db.NewUpdate().Model((*NodeEnrollmentEntity)(nil)).
		Set("updated_at = ?", time.Now().UTC()).Set("job_id = ?", jobID).
		Where("id = ?", id).Exec(ctx)
	return err
}
