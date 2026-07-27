package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"

	clickhouseclient "deploycrate-ce/clients/clickhouse"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
)

const systemTelemetryResourceAlias = "telemetry"

type clickHouseResourceSettings struct {
	Database string `json:"database"`
	User     string `json:"user"`
}

type ClickHouseResource struct {
	db       storage.Pool
	password string
}

func NewClickHouseResource(configuration config.Config, db storage.Pool) *ClickHouseResource {
	return &ClickHouseResource{
		db:       db,
		password: configuration.Metrics.ClickHousePassword,
	}
}

func (resource *ClickHouseResource) Client(ctx context.Context) (clickhouseclient.Client, error) {
	connection, err := models.EnvironmentResource.FindConnectionByApplicationAndAlias(
		ctx,
		resource.db.Executor(),
		models.SystemApplicationSlug,
		systemTelemetryResourceAlias,
	)
	if err != nil {
		return clickhouseclient.Client{}, fmt.Errorf("resolve ClickHouse resource: %w", err)
	}
	if connection.ResourceKind != "clickhouse" {
		return clickhouseclient.Client{}, fmt.Errorf(
			"resolve ClickHouse resource: telemetry binding has kind %q",
			connection.ResourceKind,
		)
	}
	if connection.Protocol != "http" && connection.Protocol != "https" {
		return clickhouseclient.Client{}, fmt.Errorf(
			"resolve ClickHouse resource: unsupported protocol %q",
			connection.Protocol,
		)
	}
	if connection.Address == "" || connection.Port <= 0 {
		return clickhouseclient.Client{}, errors.New(
			"resolve ClickHouse resource: endpoint is incomplete",
		)
	}
	if connection.CredentialSource != "app_env" {
		return clickhouseclient.Client{}, fmt.Errorf(
			"resolve ClickHouse resource: unsupported credential source %q",
			connection.CredentialSource,
		)
	}
	settings := clickHouseResourceSettings{}
	if err := json.Unmarshal(connection.Settings, &settings); err != nil {
		return clickhouseclient.Client{}, fmt.Errorf(
			"resolve ClickHouse resource settings: %w",
			err,
		)
	}
	if settings.Database == "" || settings.User == "" || resource.password == "" {
		return clickhouseclient.Client{}, errors.New(
			"resolve ClickHouse resource: database credentials are incomplete",
		)
	}
	baseURL := url.URL{
		Scheme: connection.Protocol,
		Host:   net.JoinHostPort(connection.Address, fmt.Sprint(connection.Port)),
	}
	return clickhouseclient.New(
		baseURL.String(),
		settings.Database,
		settings.User,
		resource.password,
	), nil
}
