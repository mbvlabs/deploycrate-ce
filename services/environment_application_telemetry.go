package services

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	clickhouseclient "deploycrate-ce/clients/clickhouse"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

const openTelemetryResourceEngine = "opentelemetry"

var ErrEnvironmentOpenTelemetryUnavailable = errors.New(
	"OpenTelemetry is not attached to this Environment",
)

type EnvironmentApplicationTelemetry struct {
	resource *ClickHouseResource
	db       storage.Pool
}

func NewEnvironmentApplicationTelemetry(
	resource *ClickHouseResource,
	db storage.Pool,
) *EnvironmentApplicationTelemetry {
	return &EnvironmentApplicationTelemetry{resource: resource, db: db}
}

func (service *EnvironmentApplicationTelemetry) Enabled(
	ctx context.Context,
	applicationID uuid.UUID,
	environmentID uuid.UUID,
) (bool, error) {
	if _, err := models.Environment.FindForApplication(
		ctx,
		service.db.Executor(),
		applicationID,
		environmentID,
	); err != nil {
		return false, err
	}
	return models.EnvironmentResource.HasActiveEngineForEnvironment(
		ctx,
		service.db.Executor(),
		environmentID,
		openTelemetryResourceEngine,
	)
}

func (service *EnvironmentApplicationTelemetry) Snapshot(
	ctx context.Context,
	applicationID uuid.UUID,
	environmentID uuid.UUID,
	telemetryRange TelemetryRange,
) (clickhouseclient.ApplicationTelemetry, error) {
	if err := service.ensureEnabled(ctx, applicationID, environmentID); err != nil {
		return clickhouseclient.ApplicationTelemetry{}, err
	}
	client, err := service.resource.Client(ctx)
	if err != nil {
		return clickhouseclient.ApplicationTelemetry{}, err
	}
	return client.EnvironmentApplicationTelemetry(
		ctx,
		environmentID.String(),
		time.Now().UTC().Add(-telemetryRange.Duration()),
		telemetryRange.Bucket(),
	)
}

func (service *EnvironmentApplicationTelemetry) SlowQueries(
	ctx context.Context,
	applicationID uuid.UUID,
	environmentID uuid.UUID,
	telemetryRange TelemetryRange,
) ([]clickhouseclient.QueryTelemetry, error) {
	if err := service.ensureEnabled(ctx, applicationID, environmentID); err != nil {
		return nil, err
	}
	client, err := service.resource.Client(ctx)
	if err != nil {
		return nil, err
	}
	return client.EnvironmentSlowQueries(
		ctx,
		environmentID.String(),
		time.Now().UTC().Add(-telemetryRange.Duration()),
	)
}

func (service *EnvironmentApplicationTelemetry) Logs(
	ctx context.Context,
	applicationID uuid.UUID,
	environmentID uuid.UUID,
	after string,
	telemetryRange TelemetryRange,
	search string,
) (SystemLogSnapshot, error) {
	if err := service.ensureEnabled(ctx, applicationID, environmentID); err != nil {
		return SystemLogSnapshot{}, err
	}
	cursor, err := decodeSystemLogCursor(after)
	if err != nil {
		return SystemLogSnapshot{}, err
	}
	search = strings.TrimSpace(search)
	if utf8.RuneCountInString(search) > maxSystemLogSearch {
		return SystemLogSnapshot{}, ErrInvalidSystemLogSearch
	}
	client, err := service.resource.Client(ctx)
	if err != nil {
		return SystemLogSnapshot{}, err
	}
	limit := uint64(systemLogBatchSize)
	if cursor != nil {
		limit++
	}
	page, err := client.EnvironmentTelemetryLogs(
		ctx,
		environmentID.String(),
		time.Now().UTC().Add(-telemetryRange.Duration()),
		search,
		cursor,
		limit,
	)
	if err != nil {
		return SystemLogSnapshot{}, err
	}
	return systemLogSnapshot(page, after)
}

func (service *EnvironmentApplicationTelemetry) Trace(
	ctx context.Context,
	applicationID uuid.UUID,
	environmentID uuid.UUID,
	traceID string,
) ([]clickhouseclient.TraceSpan, error) {
	if err := service.ensureEnabled(ctx, applicationID, environmentID); err != nil {
		return nil, err
	}
	traceID = strings.ToLower(strings.TrimSpace(traceID))
	if len(traceID) != 32 || strings.Trim(traceID, "0123456789abcdef") != "" {
		return nil, ErrInvalidTraceID
	}
	client, err := service.resource.Client(ctx)
	if err != nil {
		return nil, err
	}
	return client.EnvironmentTrace(ctx, environmentID.String(), traceID)
}

func (service *EnvironmentApplicationTelemetry) ensureEnabled(
	ctx context.Context,
	applicationID uuid.UUID,
	environmentID uuid.UUID,
) error {
	enabled, err := service.Enabled(ctx, applicationID, environmentID)
	if err != nil {
		return err
	}
	if !enabled {
		return ErrEnvironmentOpenTelemetryUnavailable
	}
	return nil
}
