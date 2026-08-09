package postgresql

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

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

func (Client) WaitForReady(
	ctx context.Context,
	connection Connection,
	timeout time.Duration,
) error {
	if timeout <= 0 {
		return errors.New("PostgreSQL readiness timeout must be positive")
	}
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		postgres, err := connectAdministrator(deadline, connection, "postgres")
		if err == nil {
			var result int
			err = postgres.QueryRow(deadline, "SELECT 1").Scan(&result)
			_ = postgres.Close(context.WithoutCancel(deadline))
			if err == nil && result == 1 {
				return nil
			}
			if err == nil {
				err = errors.New("PostgreSQL returned an unexpected readiness result")
			}
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.Done():
			return fmt.Errorf("wait for PostgreSQL readiness: %w", lastErr)
		case <-ticker.C:
		}
	}
}

func (Client) CreateDatabase(
	ctx context.Context,
	connection Connection,
	database, encoding, collation string,
) (bool, error) {
	if err := validateDatabaseName(database); err != nil {
		return false, err
	}
	encoding = strings.TrimSpace(encoding)
	collation = strings.TrimSpace(collation)
	if len([]byte(encoding)) > 63 || strings.ContainsRune(encoding, '\x00') {
		return false, errors.New(
			"PostgreSQL database encoding must be at most 63 bytes and contain no null bytes",
		)
	}
	if len([]byte(collation)) > 255 || strings.ContainsRune(collation, '\x00') {
		return false, errors.New(
			"PostgreSQL database collation must be at most 255 bytes and contain no null bytes",
		)
	}
	postgres, err := connectAdministrator(ctx, connection, "postgres")
	if err != nil {
		return false, err
	}
	defer postgres.Close(context.WithoutCancel(ctx))
	var exists bool
	if err := postgres.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_catalog.pg_database WHERE datname = $1)", database).
		Scan(&exists); err != nil {
		return false, fmt.Errorf("inspect PostgreSQL database %q: %w", database, err)
	}
	if exists {
		return false, nil
	}
	options := make([]string, 0, 3)
	for _, option := range []struct {
		name  string
		value string
	}{{name: "ENCODING", value: encoding}, {name: "LOCALE", value: collation}} {
		if option.value == "" {
			continue
		}
		var quoted string
		if err := postgres.QueryRow(ctx, "SELECT pg_catalog.quote_literal($1)", option.value).
			Scan(&quoted); err != nil {
			return false, fmt.Errorf(
				"quote PostgreSQL database %s: %w",
				strings.ToLower(option.name),
				err,
			)
		}
		options = append(options, option.name+" "+quoted)
	}
	if collation != "" {
		options = append(options, "TEMPLATE template0")
	}
	statement := "CREATE DATABASE " + pgx.Identifier{database}.Sanitize()
	if len(options) > 0 {
		statement += " WITH " + strings.Join(options, " ")
	}
	if _, err := postgres.Exec(ctx, statement); err != nil {
		return false, fmt.Errorf("create PostgreSQL database %q: %w", database, err)
	}
	return true, nil
}

func (Client) DropDatabase(ctx context.Context, connection Connection, database string) error {
	if err := validateDatabaseName(database); err != nil {
		return err
	}
	postgres, err := connectAdministrator(ctx, connection, "postgres")
	if err != nil {
		return err
	}
	defer postgres.Close(context.WithoutCancel(ctx))
	if _, err := postgres.Exec(
		ctx,
		"DROP DATABASE IF EXISTS "+pgx.Identifier{database}.Sanitize()+" WITH (FORCE)",
	); err != nil {
		return fmt.Errorf("drop PostgreSQL database %q: %w", database, err)
	}
	return nil
}

func (Client) DropLoginRole(
	ctx context.Context,
	connection Connection,
	username string,
) error {
	return dropLoginRole(ctx, connection, username)
}

func validateDatabaseName(database string) error {
	database = strings.TrimSpace(database)
	if database == "" || len([]byte(database)) > 63 || strings.ContainsRune(database, '\x00') {
		return errors.New(
			"PostgreSQL database name must be present, at most 63 bytes, and contain no null bytes",
		)
	}
	return nil
}

func connectAdministrator(
	ctx context.Context,
	connection Connection,
	database string,
) (*pgx.Conn, error) {
	configuration, err := pgx.ParseConfig("sslmode=disable")
	if err != nil {
		return nil, fmt.Errorf("prepare PostgreSQL administrator connection: %w", err)
	}
	configuration.Host = strings.TrimSpace(connection.Host)
	configuration.Port = uint16(connection.Port)
	configuration.Database = database
	configuration.User = strings.TrimSpace(connection.Username)
	configuration.Password = connection.Password
	configuration.TLSConfig = nil
	postgres, err := pgx.ConnectConfig(ctx, configuration)
	if err != nil {
		return nil, fmt.Errorf(
			"connect to PostgreSQL with the Resource administrator credential: %w",
			err,
		)
	}
	return postgres, nil
}

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

func (Client) ReconcileLoginRoleAcrossDatabases(
	ctx context.Context,
	connection Connection,
	databases []string,
	username, password, previousPassword string,
) error {
	for index := range databases {
		databases[index] = strings.TrimSpace(databases[index])
		if err := validateDatabaseName(databases[index]); err != nil {
			return err
		}
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("PostgreSQL role username is required")
	}
	if len([]byte(username)) > 63 || strings.ContainsRune(username, '\x00') {
		return errors.New(
			"PostgreSQL role username must be at most 63 bytes and cannot contain null bytes",
		)
	}
	if password == "" {
		return errors.New("PostgreSQL role password is required")
	}

	for _, database := range databases {
		postgres, err := connectAdministrator(ctx, connection, database)
		if err != nil {
			return fmt.Errorf("preflight PostgreSQL database %q: %w", database, err)
		}
		if err := postgres.Ping(ctx); err != nil {
			_ = postgres.Close(context.WithoutCancel(ctx))
			return fmt.Errorf("preflight PostgreSQL database %q: %w", database, err)
		}
		_ = postgres.Close(context.WithoutCancel(ctx))
	}

	postgres, err := connectAdministrator(ctx, connection, "postgres")
	if err != nil {
		return err
	}
	defer postgres.Close(context.WithoutCancel(ctx))
	lockKey := "deploycrate-role:" + username
	if _, err := postgres.Exec(
		ctx,
		"SELECT pg_catalog.pg_advisory_lock(pg_catalog.hashtext($1))",
		lockKey,
	); err != nil {
		return fmt.Errorf("lock PostgreSQL role reconciliation: %w", err)
	}
	defer postgres.Exec(
		context.WithoutCancel(ctx),
		"SELECT pg_catalog.pg_advisory_unlock(pg_catalog.hashtext($1))",
		lockKey,
	)

	tx, err := postgres.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin PostgreSQL role reconciliation: %w", err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx))

	var exists bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", username).
		Scan(&exists); err != nil {
		return fmt.Errorf("inspect PostgreSQL role: %w", err)
	}
	if exists && previousPassword == "" {
		return fmt.Errorf(
			"PostgreSQL login role %q already exists outside this credential reconciliation",
			username,
		)
	}
	identifier := pgx.Identifier{username}.Sanitize()
	if !exists {
		if _, err := tx.Exec(ctx, "CREATE ROLE "+identifier+" LOGIN"); err != nil {
			return fmt.Errorf("create PostgreSQL login role %q: %w", username, err)
		}
	}
	var quotedPassword string
	if err := tx.QueryRow(ctx, "SELECT pg_catalog.quote_literal($1)", password).
		Scan(&quotedPassword); err != nil {
		return fmt.Errorf("quote PostgreSQL role password: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		"ALTER ROLE "+identifier+" WITH LOGIN PASSWORD "+quotedPassword,
	); err != nil {
		return fmt.Errorf("rotate PostgreSQL login role %q password: %w", username, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit PostgreSQL role reconciliation: %w", err)
	}
	appliedDatabases := make([]string, 0, len(databases))
	for _, database := range databases {
		if err := (Client{}).GrantLoginRoleDatabase(
			ctx,
			connection,
			database,
			username,
		); err != nil {
			compensationErrors := make([]error, 0, len(appliedDatabases)+1)
			if exists {
				compensationErrors = append(
					compensationErrors,
					setLoginRolePassword(
						context.WithoutCancel(ctx),
						connection,
						username,
						previousPassword,
					),
				)
			} else {
				for index := len(appliedDatabases) - 1; index >= 0; index-- {
					compensationErrors = append(
						compensationErrors,
						(Client{}).RevokeLoginRoleDatabase(
							context.WithoutCancel(ctx),
							connection,
							appliedDatabases[index],
							username,
						),
					)
				}
				compensationErrors = append(
					compensationErrors,
					dropLoginRole(context.WithoutCancel(ctx), connection, username),
				)
			}
			return errors.Join(err, errors.Join(compensationErrors...))
		}
		appliedDatabases = append(appliedDatabases, database)
	}
	return nil
}

func (Client) RevokeLoginRoleDatabase(
	ctx context.Context,
	connection Connection,
	database, username string,
) error {
	if err := validateDatabaseName(database); err != nil {
		return err
	}
	username = strings.TrimSpace(username)
	if username == "" || len([]byte(username)) > 63 || strings.ContainsRune(username, '\x00') {
		return errors.New(
			"PostgreSQL role username must be present, at most 63 bytes, and contain no null bytes",
		)
	}
	postgres, err := connectAdministrator(ctx, connection, database)
	if err != nil {
		return err
	}
	defer postgres.Close(context.WithoutCancel(ctx))
	tx, err := postgres.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin PostgreSQL database grant compensation: %w", err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	identifier := pgx.Identifier{username}.Sanitize()
	databaseIdentifier := pgx.Identifier{database}.Sanitize()
	administratorIdentifier := pgx.Identifier{strings.TrimSpace(connection.Username)}.Sanitize()
	statements := []string{
		"ALTER DEFAULT PRIVILEGES FOR ROLE " + administratorIdentifier + " IN SCHEMA public REVOKE ALL PRIVILEGES ON ROUTINES FROM " + identifier,
		"ALTER DEFAULT PRIVILEGES FOR ROLE " + administratorIdentifier + " IN SCHEMA public REVOKE ALL PRIVILEGES ON SEQUENCES FROM " + identifier,
		"ALTER DEFAULT PRIVILEGES FOR ROLE " + administratorIdentifier + " IN SCHEMA public REVOKE ALL PRIVILEGES ON TABLES FROM " + identifier,
		"REVOKE ALL PRIVILEGES ON ALL ROUTINES IN SCHEMA public FROM " + identifier,
		"REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM " + identifier,
		"REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM " + identifier,
		"REVOKE ALL PRIVILEGES ON SCHEMA public FROM " + identifier,
		"REVOKE ALL PRIVILEGES ON DATABASE " + databaseIdentifier + " FROM " + identifier,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf(
				"revoke PostgreSQL database %q privileges from role %q: %w",
				database,
				username,
				err,
			)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit PostgreSQL database %q grant compensation: %w", database, err)
	}
	return nil
}

func (Client) RotateAdministratorPassword(
	ctx context.Context,
	connection Connection,
	newPassword string,
) error {
	username := strings.TrimSpace(connection.Username)
	if username == "" || len([]byte(username)) > 63 || strings.ContainsRune(username, '\x00') {
		return errors.New(
			"PostgreSQL administrator username must be present, at most 63 bytes, and contain no null bytes",
		)
	}
	if connection.Password == "" || newPassword == "" {
		return errors.New("PostgreSQL administrator passwords are required")
	}
	if err := setLoginRolePassword(ctx, connection, username, newPassword); err != nil {
		return fmt.Errorf("rotate PostgreSQL administrator password: %w", err)
	}
	return nil
}

func (Client) GrantLoginRoleDatabase(
	ctx context.Context,
	connection Connection,
	database, username string,
) error {
	if err := validateDatabaseName(database); err != nil {
		return err
	}
	username = strings.TrimSpace(username)
	if username == "" || len([]byte(username)) > 63 || strings.ContainsRune(username, '\x00') {
		return errors.New(
			"PostgreSQL role username must be present, at most 63 bytes, and contain no null bytes",
		)
	}
	postgres, err := connectAdministrator(ctx, connection, database)
	if err != nil {
		return err
	}
	defer postgres.Close(context.WithoutCancel(ctx))
	tx, err := postgres.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin PostgreSQL database grant reconciliation: %w", err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	identifier := pgx.Identifier{username}.Sanitize()
	databaseIdentifier := pgx.Identifier{database}.Sanitize()
	administratorIdentifier := pgx.Identifier{strings.TrimSpace(connection.Username)}.Sanitize()
	statements := []string{
		"REVOKE CONNECT ON DATABASE " + databaseIdentifier + " FROM PUBLIC",
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
			return fmt.Errorf(
				"grant PostgreSQL database %q privileges to role %q: %w",
				database,
				username,
				err,
			)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit PostgreSQL database %q grants: %w", database, err)
	}
	return nil
}

func setLoginRolePassword(
	ctx context.Context,
	connection Connection,
	username, password string,
) error {
	postgres, err := connectAdministrator(ctx, connection, "postgres")
	if err != nil {
		return err
	}
	defer postgres.Close(context.WithoutCancel(ctx))
	var quotedPassword string
	if err := postgres.QueryRow(ctx, "SELECT pg_catalog.quote_literal($1)", password).
		Scan(&quotedPassword); err != nil {
		return fmt.Errorf("quote PostgreSQL role compensation password: %w", err)
	}
	if _, err := postgres.Exec(
		ctx,
		"ALTER ROLE "+pgx.Identifier{username}.Sanitize()+" WITH LOGIN PASSWORD "+quotedPassword,
	); err != nil {
		return fmt.Errorf("restore PostgreSQL login role %q password: %w", username, err)
	}
	return nil
}

func dropLoginRole(ctx context.Context, connection Connection, username string) error {
	postgres, err := connectAdministrator(ctx, connection, "postgres")
	if err != nil {
		return err
	}
	defer postgres.Close(context.WithoutCancel(ctx))
	if _, err := postgres.Exec(
		ctx,
		"DROP ROLE IF EXISTS "+pgx.Identifier{username}.Sanitize(),
	); err != nil {
		return fmt.Errorf(
			"remove PostgreSQL login role %q after failed reconciliation: %w",
			username,
			err,
		)
	}
	return nil
}
