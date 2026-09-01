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
	"github.com/jackc/pgx/v5/pgconn"
)

const catalogColumnLimit = 5000

type CatalogColumn struct {
	Name       string `json:"name"`
	DataType   string `json:"dataType"`
	Nullable   bool   `json:"nullable"`
	HasDefault bool   `json:"hasDefault"`
}

type CatalogRelation struct {
	Schema  string          `json:"schema"`
	Name    string          `json:"name"`
	Kind    string          `json:"kind"`
	Columns []CatalogColumn `json:"columns"`
}

type Catalog struct {
	Relations []CatalogRelation `json:"relations"`
	Truncated bool              `json:"truncated"`
}

type QueryColumn struct {
	Name     string `json:"name"`
	DataType string `json:"dataType"`
}

type QueryResult struct {
	Columns     []QueryColumn `json:"columns"`
	Rows        [][]any       `json:"rows"`
	CommandTag  string        `json:"commandTag"`
	RowCount    int           `json:"rowCount"`
	Truncated   bool          `json:"truncated"`
	ExecutionMS int64         `json:"executionMs"`
}

type QueryError struct {
	Code     string `json:"code,omitempty"`
	Message  string `json:"message"`
	Detail   string `json:"detail,omitempty"`
	Hint     string `json:"hint,omitempty"`
	Position int32  `json:"position,omitempty"`
}

func (e *QueryError) Error() string { return e.Message }

func (Client) Catalog(
	ctx context.Context,
	connection Connection,
	database string,
) (Catalog, error) {
	postgres, err := connectDatabase(ctx, connection, database)
	if err != nil {
		return Catalog{}, err
	}
	defer postgres.Close(context.WithoutCancel(ctx))

	tx, err := beginReadOnly(ctx, postgres)
	if err != nil {
		return Catalog{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))

	rows, err := tx.Query(ctx, `
		SELECT namespace.nspname,
		       relation.relname,
		       CASE relation.relkind
		         WHEN 'r' THEN 'table'
		         WHEN 'p' THEN 'partitioned table'
		         WHEN 'v' THEN 'view'
		         WHEN 'm' THEN 'materialized view'
		         WHEN 'f' THEN 'foreign table'
		         ELSE 'relation'
		       END,
		       attribute.attname,
		       pg_catalog.format_type(attribute.atttypid, attribute.atttypmod),
		       NOT attribute.attnotnull,
		       default_value.adbin IS NOT NULL
		FROM pg_catalog.pg_class AS relation
		JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = relation.relnamespace
		JOIN pg_catalog.pg_attribute AS attribute ON attribute.attrelid = relation.oid
		LEFT JOIN pg_catalog.pg_attrdef AS default_value
		  ON default_value.adrelid = relation.oid AND default_value.adnum = attribute.attnum
		WHERE relation.relkind IN ('r', 'p', 'v', 'm', 'f')
		  AND attribute.attnum > 0
		  AND NOT attribute.attisdropped
		  AND namespace.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND namespace.nspname NOT LIKE 'pg_toast%'
		ORDER BY namespace.nspname, relation.relname, attribute.attnum
		LIMIT $1`, catalogColumnLimit+1)
	if err != nil {
		return Catalog{}, fmt.Errorf("inspect PostgreSQL catalog: %w", err)
	}
	defer rows.Close()

	catalog := Catalog{Relations: make([]CatalogRelation, 0)}
	for rows.Next() {
		if len(catalog.Relations) > 0 && catalogColumnCount(catalog.Relations) >= catalogColumnLimit {
			catalog.Truncated = true
			break
		}
		var schema, relation, kind string
		var column CatalogColumn
		if err := rows.Scan(
			&schema, &relation, &kind, &column.Name, &column.DataType,
			&column.Nullable, &column.HasDefault,
		); err != nil {
			return Catalog{}, fmt.Errorf("read PostgreSQL catalog: %w", err)
		}
		last := len(catalog.Relations) - 1
		if last < 0 || catalog.Relations[last].Schema != schema ||
			catalog.Relations[last].Name != relation {
			catalog.Relations = append(catalog.Relations, CatalogRelation{
				Schema: schema, Name: relation, Kind: kind, Columns: make([]CatalogColumn, 0),
			})
			last++
		}
		catalog.Relations[last].Columns = append(catalog.Relations[last].Columns, column)
	}
	if err := rows.Err(); err != nil {
		return Catalog{}, fmt.Errorf("read PostgreSQL catalog: %w", err)
	}
	return catalog, nil
}

func (Client) ExecuteReadOnly(
	ctx context.Context,
	connection Connection,
	database, statement string,
	maxRows, maxBytes int,
) (QueryResult, error) {
	if maxRows <= 0 || maxBytes <= 0 {
		return QueryResult{}, errors.New("PostgreSQL query limits must be positive")
	}
	postgres, err := connectDatabase(ctx, connection, database)
	if err != nil {
		return QueryResult{}, err
	}
	defer postgres.Close(context.WithoutCancel(ctx))

	tx, err := beginReadOnly(ctx, postgres)
	if err != nil {
		return QueryResult{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))

	queryContext, cancel := context.WithCancel(ctx)
	defer cancel()
	startedAt := time.Now()
	rows, err := tx.Query(
		queryContext,
		statement,
		pgx.QueryResultFormats{pgx.TextFormatCode},
	)
	if err != nil {
		return QueryResult{}, normalizeQueryError(err)
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	result := QueryResult{
		Columns: make([]QueryColumn, 0, len(fields)),
		Rows:    make([][]any, 0, min(maxRows, 64)),
	}
	for _, field := range fields {
		dataType := strconv.FormatUint(uint64(field.DataTypeOID), 10)
		if registered, ok := postgres.TypeMap().TypeForOID(field.DataTypeOID); ok {
			dataType = registered.Name
		}
		result.Columns = append(result.Columns, QueryColumn{
			Name: field.Name, DataType: dataType,
		})
	}

	usedBytes := 0
	for rows.Next() {
		if len(result.Rows) >= maxRows {
			result.Truncated = true
			cancel()
			break
		}
		raw := rows.RawValues()
		row := make([]any, len(raw))
		for index, value := range raw {
			if value == nil {
				row[index] = nil
				continue
			}
			remaining := maxBytes - usedBytes
			if remaining <= 0 {
				result.Truncated = true
				cancel()
				break
			}
			if len(value) > remaining {
				row[index] = string(value[:remaining]) + "…"
				usedBytes = maxBytes
				result.Truncated = true
				cancel()
				break
			}
			row[index] = string(value)
			usedBytes += len(value)
		}
		result.Rows = append(result.Rows, row)
		if result.Truncated {
			break
		}
	}
	if !result.Truncated {
		if err := rows.Err(); err != nil {
			return QueryResult{}, normalizeQueryError(err)
		}
	}
	result.CommandTag = rows.CommandTag().String()
	result.RowCount = len(result.Rows)
	result.ExecutionMS = time.Since(startedAt).Milliseconds()
	return result, nil
}

func connectDatabase(
	ctx context.Context,
	connection Connection,
	database string,
) (*pgx.Conn, error) {
	tlsMode := strings.TrimSpace(connection.TLSMode)
	if tlsMode == "" {
		tlsMode = "disable"
	}
	databaseURL := &url.URL{
		Scheme: "postgres",
		User: url.UserPassword(
			strings.TrimSpace(connection.Username),
			connection.Password,
		),
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
		return nil, fmt.Errorf("connect to PostgreSQL database %q: %w", database, err)
	}
	return postgres, nil
}

func beginReadOnly(ctx context.Context, postgres *pgx.Conn) (pgx.Tx, error) {
	tx, err := postgres.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin PostgreSQL read-only transaction: %w", err)
	}
	for _, statement := range []string{
		"SET LOCAL statement_timeout = '15s'",
		"SET LOCAL lock_timeout = '3s'",
		"SET LOCAL idle_in_transaction_session_timeout = '20s'",
	} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			return nil, fmt.Errorf("configure PostgreSQL read-only transaction: %w", err)
		}
	}
	var ready int
	if err := tx.QueryRow(ctx, "SELECT 1").Scan(&ready); err != nil {
		_ = tx.Rollback(context.WithoutCancel(ctx))
		return nil, fmt.Errorf("lock PostgreSQL read-only transaction mode: %w", err)
	}
	if ready != 1 {
		_ = tx.Rollback(context.WithoutCancel(ctx))
		return nil, errors.New("lock PostgreSQL read-only transaction mode: unexpected result")
	}
	return tx, nil
}

func normalizeQueryError(err error) error {
	if postgresError, ok := errors.AsType[*pgconn.PgError](err); ok {
		return &QueryError{
			Code: postgresError.Code, Message: postgresError.Message,
			Detail: postgresError.Detail, Hint: postgresError.Hint,
			Position: postgresError.Position,
		}
	}
	return err
}

func catalogColumnCount(relations []CatalogRelation) int {
	count := 0
	for _, relation := range relations {
		count += len(relation.Columns)
	}
	return count
}
