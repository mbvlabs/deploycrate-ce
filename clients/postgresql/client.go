package postgresql

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
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

func (Client) Check(ctx context.Context, connection Connection, database, tlsMode string) error {
	database = strings.TrimSpace(database)
	if database == "" {
		database = "postgres"
	}
	tlsMode = strings.TrimSpace(tlsMode)
	if tlsMode == "" {
		tlsMode = "disable"
	}
	databaseURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(strings.TrimSpace(connection.Username), connection.Password),
		Host: net.JoinHostPort(
			strings.TrimSpace(connection.Host),
			strconv.Itoa(int(connection.Port)),
		),
		Path: database,
	}
	query := databaseURL.Query()
	query.Set("sslmode", tlsMode)
	databaseURL.RawQuery = query.Encode()

	postgres, err := pgx.Connect(ctx, databaseURL.String())
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL Resource: %w", err)
	}
	defer postgres.Close(context.WithoutCancel(ctx))

	var result int
	if err := postgres.QueryRow(ctx, "SELECT 1").Scan(&result); err != nil {
		return fmt.Errorf("query PostgreSQL Resource: %w", err)
	}
	if result != 1 {
		return errors.New("PostgreSQL Resource returned an unexpected readiness result")
	}
	return nil
}

func (Client) ReconcileLoginRole(ctx context.Context, connection Connection, database, username, password string) error {
	database = strings.TrimSpace(database)
	if database != "" && (len([]byte(database)) > 63 || strings.ContainsRune(database, '\x00')) {
		return errors.New("PostgreSQL database name must be at most 63 bytes and cannot contain null bytes")
	}
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
	configuration.Database = database
	if configuration.Database == "" {
		configuration.Database = "postgres"
	}
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
	if database != "" {
		databaseIdentifier := pgx.Identifier{database}.Sanitize()
		administratorIdentifier := pgx.Identifier{configuration.User}.Sanitize()
		statements := []string{
			"GRANT ALL PRIVILEGES ON DATABASE " + databaseIdentifier + " TO " + identifier,
			"GRANT ALL PRIVILEGES ON SCHEMA public TO " + identifier,
			"GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO " + identifier,
			"GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO " + identifier,
			"GRANT ALL PRIVILEGES ON ALL ROUTINES IN SCHEMA public TO " + identifier,
			"ALTER DEFAULT PRIVILEGES FOR ROLE " + administratorIdentifier + " IN SCHEMA public GRANT ALL PRIVILEGES ON TABLES TO " + identifier,
			"ALTER DEFAULT PRIVILEGES FOR ROLE " + administratorIdentifier + " IN SCHEMA public GRANT ALL PRIVILEGES ON SEQUENCES TO " + identifier,
			"ALTER DEFAULT PRIVILEGES FOR ROLE " + administratorIdentifier + " IN SCHEMA public GRANT ALL PRIVILEGES ON ROUTINES TO " + identifier,
		}
		for _, statement := range statements {
			if _, err := tx.Exec(ctx, statement); err != nil {
				return fmt.Errorf("grant PostgreSQL database %q privileges to role %q: %w", database, username, err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit PostgreSQL role reconciliation: %w", err)
	}
	return nil
}
