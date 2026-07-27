package models

import (
	"context"
	"errors"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

func ensureActiveUnique(
	ctx context.Context,
	db storage.Executor,
	lockKey string,
	entityID uuid.UUID,
	query *bun.SelectQuery,
	field string,
	message string,
) error {
	switch db.(type) {
	case bun.Tx, *bun.Tx:
	default:
		return errors.New("active uniqueness checks require a transaction")
	}
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", lockKey); err != nil {
		return err
	}
	count, err := query.Where("archived_at IS NULL").Where("id <> ?", entityID).Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.Join(ErrDomainValidation, validation.ValidationErrors{{Field: field, Code: "taken", Message: message}})
	}
	return nil
}
