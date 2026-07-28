package postgresql

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type Connection struct {
	Host     string
	Port     int32
	Username string
	Password string
}

type Client struct{}

func New() Client { return Client{} }

func (Client) ReconcileLoginRole(ctx context.Context, connection Connection, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("PostgreSQL role username is required")
	}
	if len([]byte(username)) > 63 || strings.ContainsRune(username, '\x00') {
		return errors.New("PostgreSQL role username must be at most 63 bytes and cannot contain null bytes")
	}
	if password == "" {
		return errors.New("PostgreSQL role password is required")
	}

	configuration, err := pgx.ParseConfig("sslmode=disable")
	if err != nil {
		return fmt.Errorf("prepare PostgreSQL administrator connection: %w", err)
	}
	configuration.Host = strings.TrimSpace(connection.Host)
	configuration.Port = uint16(connection.Port)
	configuration.Database = "postgres"
	configuration.User = strings.TrimSpace(connection.Username)
	configuration.Password = connection.Password
	configuration.TLSConfig = nil

	postgres, err := pgx.ConnectConfig(ctx, configuration)
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL with the Resource administrator credential: %w", err)
	}
	defer postgres.Close(context.WithoutCancel(ctx))

	tx, err := postgres.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin PostgreSQL role reconciliation: %w", err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx))

	if _, err := tx.Exec(ctx, "SELECT pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtext($1))", "deploycrate-role:"+username); err != nil {
		return fmt.Errorf("lock PostgreSQL role reconciliation: %w", err)
	}
	var exists bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", username).Scan(&exists); err != nil {
		return fmt.Errorf("inspect PostgreSQL role: %w", err)
	}
	identifier := pgx.Identifier{username}.Sanitize()
	if !exists {
		if _, err := tx.Exec(ctx, "CREATE ROLE "+identifier+" LOGIN"); err != nil {
			return fmt.Errorf("create PostgreSQL login role %q: %w", username, err)
		}
	}
	var quotedPassword string
	if err := tx.QueryRow(ctx, "SELECT pg_catalog.quote_literal($1)", password).Scan(&quotedPassword); err != nil {
		return fmt.Errorf("quote PostgreSQL role password: %w", err)
	}
	if _, err := tx.Exec(ctx, "ALTER ROLE "+identifier+" WITH LOGIN PASSWORD "+quotedPassword); err != nil {
		return fmt.Errorf("rotate PostgreSQL login role %q password: %w", username, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit PostgreSQL role reconciliation: %w", err)
	}
	return nil
}
