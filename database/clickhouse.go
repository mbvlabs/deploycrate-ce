package database

import (
	"context"
	"crypto/tls"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"strings"

	"deploycrate-ce/config"

	clickhousedriver "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/pressly/goose/v3"
)

//go:embed clickhouse/migrations/*
var ClickHouseMigrations embed.FS

type ClickHouse struct {
	conn *sql.DB
}

func NewClickHouse(ctx context.Context, cfg config.Config) (*ClickHouse, error) {
	options := &clickhousedriver.Options{
		Addr: []string{net.JoinHostPort(cfg.ClickHouse.Host, cfg.ClickHouse.Port)},
		Auth: clickhousedriver.Auth{
			Database: cfg.ClickHouse.Database,
			Username: cfg.ClickHouse.User,
			Password: cfg.ClickHouse.Password,
		},
	}
	switch strings.ToLower(strings.TrimSpace(cfg.ClickHouse.Protocol)) {
	case "http":
		options.Protocol = clickhousedriver.HTTP
	case "https":
		options.Protocol = clickhousedriver.HTTP
		options.TLS = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.ClickHouse.Host}
	case "clickhouse", "native", "tcp":
		options.Protocol = clickhousedriver.Native
	default:
		return nil, fmt.Errorf(
			"database: unsupported ClickHouse protocol %q",
			cfg.ClickHouse.Protocol,
		)
	}

	db := clickhousedriver.OpenDB(options)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("database: ping ClickHouse: %w", err)
	}
	return &ClickHouse{conn: db}, nil
}

func (c *ClickHouse) Conn() *sql.DB {
	return c.conn
}

func (c *ClickHouse) Close() error {
	return c.conn.Close()
}

func ApplyClickHouseMigrations(ctx context.Context, db *sql.DB) error {
	migrations, err := fs.Sub(ClickHouseMigrations, "clickhouse/migrations")
	if err != nil {
		return fmt.Errorf("database: open ClickHouse migrations: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectClickHouse, db, migrations)
	if err != nil {
		return fmt.Errorf("database: create ClickHouse migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("database: apply ClickHouse migrations: %w", err)
	}
	return nil
}
