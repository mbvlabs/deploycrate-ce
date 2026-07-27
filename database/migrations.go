package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"deploycrate-ce/internal/storage"

	"github.com/pressly/goose/v3/lock"
)

func ApplyMigrations(ctx context.Context, db storage.Pool) (returnErr error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("database: create migration lock: %w", err)
	}
	connection, err := db.Conn().Conn(ctx)
	if err != nil {
		return fmt.Errorf("database: acquire migration connection: %w", err)
	}
	defer connection.Close()
	if err := locker.SessionLock(ctx, connection); err != nil {
		return fmt.Errorf("database: acquire migration lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		if err := locker.SessionUnlock(unlockCtx, connection); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("database: release migration lock: %w", err))
		}
	}()

	if err := storage.RunMigrations(ctx, db.Conn(), Migrations, "migrations"); err != nil {
		return fmt.Errorf("database: apply migrations: %w", err)
	}
	return nil
}
