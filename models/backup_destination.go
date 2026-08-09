package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type BackupDestinationEntity struct {
	bun.BaseModel  `bun:"table:backup_destinations,alias:backup_destinations"`
	ID             uuid.UUID      `bun:"id,pk,type:uuid"`
	CreatedAt      time.Time      `bun:"created_at"`
	UpdatedAt      time.Time      `bun:"updated_at"`
	Name           string         `bun:"name"`
	Provider       string         `bun:"provider"`
	Endpoint       sql.NullString `bun:"endpoint"`
	Region         sql.NullString `bun:"region"`
	Bucket         string         `bun:"bucket"`
	Prefix         sql.NullString `bun:"prefix"`
	ForcePathStyle bool           `bun:"force_path_style"`
	ArchivedAt     sql.NullTime   `bun:"archived_at"`
	CredentialID   uuid.UUID      `bun:"credential_id,type:uuid"`
}

func backupDestinationActiveCredentialQuery(
	db storage.Executor,
	id uuid.UUID,
) *bun.SelectQuery {
	return db.NewSelect().
		TableExpr("backup_destinations AS destination").
		Join("JOIN credentials AS credential ON credential.id = destination.credential_id").
		Where("destination.id = ?", id).
		Where("destination.archived_at IS NULL").
		Where("credential.archived_at IS NULL").
		Where("credential.verified_at IS NOT NULL").
		Where("credential.provider = 'backup_' || destination.provider")
}

func (backupDestination) ActiveVerifiedCredentialPayload(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) ([]byte, error) {
	var payload []byte
	err := backupDestinationActiveCredentialQuery(db, id).
		ColumnExpr("credential.enc_payload").
		Limit(1).
		Scan(ctx, &payload)
	return payload, err
}

func (backupDestination) IsActiveVerified(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (bool, error) {
	count, err := backupDestinationActiveCredentialQuery(db, id).Count(ctx)
	return count > 0, err
}

type BackupDestinationSummary struct {
	ID             uuid.UUID  `bun:"id"`
	Name           string     `bun:"name"`
	Provider       string     `bun:"provider"`
	Endpoint       string     `bun:"endpoint"`
	Region         string     `bun:"region"`
	Bucket         string     `bun:"bucket"`
	Prefix         string     `bun:"prefix"`
	ForcePathStyle bool       `bun:"force_path_style"`
	VerifiedAt     *time.Time `bun:"verified_at"`
	LastUsedAt     *time.Time `bun:"last_used_at"`
}

func (bd backupDestination) ActiveSummaries(
	ctx context.Context,
	db storage.Executor,
) ([]BackupDestinationSummary, error) {
	items := make([]BackupDestinationSummary, 0)
	err := db.NewSelect().
		TableExpr("backup_destinations AS destination").
		ColumnExpr("destination.id, destination.name, destination.provider, destination.force_path_style").
		ColumnExpr("COALESCE(destination.endpoint, '') AS endpoint, COALESCE(destination.region, '') AS region").
		ColumnExpr("destination.bucket, COALESCE(destination.prefix, '') AS prefix").
		ColumnExpr("credential.verified_at, credential.last_used_at").
		Join("JOIN credentials AS credential ON credential.id = destination.credential_id").
		Where("destination.archived_at IS NULL").
		Where("credential.archived_at IS NULL").
		Where("credential.verified_at IS NOT NULL").
		Where("credential.provider = 'backup_' || destination.provider").
		OrderExpr("destination.name ASC").
		Scan(ctx, &items)
	return items, err
}

func (bd backupDestination) ActiveSummary(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (BackupDestinationSummary, error) {
	var item BackupDestinationSummary
	err := db.NewSelect().
		TableExpr("backup_destinations AS destination").
		ColumnExpr("destination.id, destination.name, destination.provider, destination.force_path_style").
		ColumnExpr("COALESCE(destination.endpoint, '') AS endpoint, COALESCE(destination.region, '') AS region").
		ColumnExpr("destination.bucket, COALESCE(destination.prefix, '') AS prefix").
		ColumnExpr("credential.verified_at, credential.last_used_at").
		Join("JOIN credentials AS credential ON credential.id = destination.credential_id").
		Where("destination.id = ?", id).
		Where("destination.archived_at IS NULL").
		Where("credential.archived_at IS NULL").
		Where("credential.verified_at IS NOT NULL").
		Where("credential.provider = 'backup_' || destination.provider").
		Limit(1).
		Scan(ctx, &item)
	if errors.Is(err, sql.ErrNoRows) {
		return BackupDestinationSummary{}, ErrNotFound
	}
	return item, err
}

func (bd backupDestination) HasActivePolicyReferences(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (bool, error) {
	count, err := db.NewSelect().TableExpr("backup_policies").
		Where("backup_destination_id = ?", id).
		Where("archived_at IS NULL").
		Where("activated_at IS NOT NULL").
		Count(ctx)
	return count > 0, err
}

func (e *BackupDestinationEntity) Validate() error {
	builder := validation.NewBuilder()
	if e.ID == uuid.Nil {
		builder.Add("id", "required", "backup destination ID is required")
	}
	if strings.TrimSpace(e.Name) == "" {
		builder.Add("name", "required", "backup destination name is required")
	}
	if e.Provider != "s3" && e.Provider != "r2" {
		builder.Add("provider", "unsupported", "backup destination provider must be s3 or r2")
	}
	if strings.TrimSpace(e.Bucket) == "" {
		builder.Add("bucket", "required", "backup destination bucket is required")
	}
	if e.Provider == "s3" && (!e.Region.Valid || strings.TrimSpace(e.Region.String) == "") {
		builder.Add("region", "required", "generic S3 destinations require a region")
	}
	if e.Provider == "r2" && (!e.Region.Valid || e.Region.String != "auto") {
		builder.Add("region", "invalid", "Cloudflare R2 region must be auto")
	}
	if e.Provider == "r2" && !e.Endpoint.Valid {
		builder.Add("endpoint", "required", "Cloudflare R2 destinations require an endpoint")
	}
	if e.Endpoint.Valid {
		endpoint, err := url.Parse(strings.TrimSpace(e.Endpoint.String))
		if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" ||
			endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
			builder.Add(
				"endpoint",
				"invalid",
				"backup destination endpoint must be an absolute HTTPS URL",
			)
		}
	}
	if e.Prefix.Valid {
		prefix := strings.Trim(e.Prefix.String, "/")
		if prefix != e.Prefix.String {
			builder.Add("prefix", "invalid", "backup destination prefix must be normalized")
		}
		for segment := range strings.SplitSeq(prefix, "/") {
			if segment == "" || segment == "." || segment == ".." {
				builder.Add("prefix", "invalid", "backup destination prefix is unsafe")
				break
			}
		}
	}
	if e.CredentialID == uuid.Nil {
		builder.Add("credential_id", "required", "backup destination credential is required")
	}
	return builder.Err()
}

func (bd backupDestination) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (BackupDestinationEntity, error) {
	var entity BackupDestinationEntity
	if err := db.NewSelect().
		Model(&entity).
		Where("id = ?", id).
		Scan(ctx); err != nil {
		return BackupDestinationEntity{}, err
	}

	return entity, nil
}

type CreateBackupDestinationData struct {
	Name           string
	Provider       string
	Endpoint       sql.NullString
	Region         sql.NullString
	Bucket         string
	Prefix         sql.NullString
	ForcePathStyle bool
	ArchivedAt     sql.NullTime
	CredentialID   uuid.UUID
}

func (bd backupDestination) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateBackupDestinationData,
) (BackupDestinationEntity, error) {
	entity := BackupDestinationEntity{
		ID:             uuid.New(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Name:           data.Name,
		Provider:       data.Provider,
		Endpoint:       data.Endpoint,
		Region:         data.Region,
		Bucket:         data.Bucket,
		Prefix:         data.Prefix,
		ForcePathStyle: data.ForcePathStyle,
		ArchivedAt:     data.ArchivedAt,
		CredentialID:   data.CredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return BackupDestinationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return BackupDestinationEntity{}, err
	}

	return entity, nil
}

type UpdateBackupDestinationData struct {
	ID             uuid.UUID
	UpdatedAt      time.Time
	Name           string
	Provider       string
	Endpoint       sql.NullString
	Region         sql.NullString
	Bucket         string
	Prefix         sql.NullString
	ForcePathStyle bool
	ArchivedAt     sql.NullTime
	CredentialID   uuid.UUID
}

func (bd backupDestination) Update(
	ctx context.Context,
	db storage.Executor,
	data UpdateBackupDestinationData,
) (BackupDestinationEntity, error) {
	if data.ArchivedAt.Valid {
		referenced, err := bd.HasActivePolicyReferences(ctx, db, data.ID)
		if err != nil {
			return BackupDestinationEntity{}, err
		}
		if referenced {
			return BackupDestinationEntity{}, errors.Join(
				ErrDomainValidation,
				errors.New(
					"active backup policies must be paused or archived before archiving this destination",
				),
			)
		}
	}
	entity := BackupDestinationEntity{
		ID:             data.ID,
		UpdatedAt:      time.Now(),
		Name:           data.Name,
		Provider:       data.Provider,
		Endpoint:       data.Endpoint,
		Region:         data.Region,
		Bucket:         data.Bucket,
		Prefix:         data.Prefix,
		ForcePathStyle: data.ForcePathStyle,
		ArchivedAt:     data.ArchivedAt,
		CredentialID:   data.CredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return BackupDestinationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewUpdate().
		Model(&entity).
		Column("updated_at").
		Column("name").
		Column("provider").
		Column("endpoint").
		Column("region").
		Column("bucket").
		Column("prefix").
		Column("force_path_style").
		Column("archived_at").
		Column("credential_id").
		WherePK().
		Returning("*").
		Scan(ctx); err != nil {
		return BackupDestinationEntity{}, err
	}

	return entity, nil
}

func (bd backupDestination) Destroy(ctx context.Context, db storage.Executor, id uuid.UUID) error {
	_, err := db.NewDelete().
		Model((*BackupDestinationEntity)(nil)).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (bd backupDestination) All(
	ctx context.Context,
	db storage.Executor,
) ([]BackupDestinationEntity, error) {
	var entities []BackupDestinationEntity
	if err := db.NewSelect().
		Model(&entities).
		Scan(ctx); err != nil {
		return nil, err
	}

	return entities, nil
}

type PaginatedBackupDestinations struct {
	BackupDestinations []BackupDestinationEntity
	TotalCount         int64
	Page               int64
	PageSize           int64
	TotalPages         int64
}

func (bd backupDestination) Paginate(
	ctx context.Context,
	db storage.Executor,
	page, pageSize int64,
) (PaginatedBackupDestinations, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	totalCount, err := db.NewSelect().
		Model(&BackupDestinationEntity{}).Count(ctx)
	if err != nil {
		return PaginatedBackupDestinations{}, err
	}

	entities := make([]BackupDestinationEntity, 0, int(pageSize))
	if err := db.NewSelect().
		Model(&entities).
		Limit(int(pageSize)).
		Offset(int(offset)).
		Scan(ctx); err != nil {
		return PaginatedBackupDestinations{}, err
	}

	totalPages := (int64(totalCount) + pageSize - 1) / pageSize

	return PaginatedBackupDestinations{
		BackupDestinations: entities,
		TotalCount:         int64(totalCount),
		Page:               page,
		PageSize:           pageSize,
		TotalPages:         totalPages,
	}, nil
}

func (bd backupDestination) Upsert(
	ctx context.Context,
	db storage.Executor,
	data CreateBackupDestinationData,
) (BackupDestinationEntity, error) {
	entity := BackupDestinationEntity{
		ID:             uuid.New(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Name:           data.Name,
		Provider:       data.Provider,
		Endpoint:       data.Endpoint,
		Region:         data.Region,
		Bucket:         data.Bucket,
		Prefix:         data.Prefix,
		ForcePathStyle: data.ForcePathStyle,
		ArchivedAt:     data.ArchivedAt,
		CredentialID:   data.CredentialID,
	}

	if err := validation.Validate(&entity); err != nil {
		return BackupDestinationEntity{}, errors.Join(ErrDomainValidation, err)
	}

	if err := db.NewInsert().
		Model(&entity).
		On("CONFLICT (id) DO UPDATE").
		Set("name = excluded.name").
		Set("provider = excluded.provider").
		Set("endpoint = excluded.endpoint").
		Set("region = excluded.region").
		Set("bucket = excluded.bucket").
		Set("prefix = excluded.prefix").
		Set("force_path_style = excluded.force_path_style").
		Set("archived_at = excluded.archived_at").
		Set("credential_id = excluded.credential_id").
		Returning("*").
		Scan(ctx); err != nil {
		return BackupDestinationEntity{}, err
	}

	return entity, nil
}
