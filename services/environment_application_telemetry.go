package services

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	clickhouseclient "deploycrate-ce/database/clickhouse"
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
	geo      GeoResolver
}

func NewEnvironmentApplicationTelemetry(
	resource *ClickHouseResource,
	db storage.Pool,
	geo GeoResolver,
) *EnvironmentApplicationTelemetry {
	return &EnvironmentApplicationTelemetry{resource: resource, db: db, geo: geo}
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
	responseClass string,
) (ApplicationTelemetry, error) {
	if err := service.ensureEnabled(ctx, applicationID, environmentID); err != nil {
		return ApplicationTelemetry{}, err
	}
	client, err := service.resource.Queries(ctx)
	if err != nil {
		return ApplicationTelemetry{}, err
	}
	class, err := telemetryResponseClass(responseClass)
	if err != nil {
		return ApplicationTelemetry{}, err
	}
	since := time.Now().UTC().Add(-telemetryRange.Duration())
	snapshot, err := loadApplicationTelemetry(
		ctx,
		client,
		clickhouseclient.EnvironmentTelemetryScope(environmentID.String()),
		since,
		telemetryRange.Bucket(),
		class,
	)
	if err != nil {
		return ApplicationTelemetry{}, err
	}
	domain, err := models.EnvironmentDomain.PrimaryForEnvironment(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.WarnContext(ctx, "request geography domain lookup failed", "error", err)
		}
		return snapshot, nil
	}
	addresses, err := client.RequestAddresses(ctx, domain.Hostname, since)
	if err != nil {
		slog.WarnContext(ctx, "request geography query failed", "error", err)
		return snapshot, nil
	}
	snapshot.Countries = service.resolveCountries(ctx, addresses)
	return snapshot, nil
}

func (service *EnvironmentApplicationTelemetry) resolveCountries(
	ctx context.Context,
	rows []clickhouseclient.RequestAddressResult,
) []CountryTelemetry {
	requestsByAddress := make(map[string]uint64, len(rows))
	addresses := make([]string, 0, len(rows))
	for _, row := range rows {
		address := net.ParseIP(strings.TrimSpace(row.Address))
		if address == nil {
			continue
		}
		normalized := address.String()
		if _, ok := requestsByAddress[normalized]; !ok {
			addresses = append(addresses, normalized)
		}
		requestsByAddress[normalized] += row.Requests
	}
	locations, _ := service.geo.Resolve(ctx, addresses)
	requestsByCountry := make(map[string]uint64)
	for address, requests := range requestsByAddress {
		code := strings.ToUpper(strings.TrimSpace(locations[address].CountryCode))
		if len(code) != 2 {
			continue
		}
		requestsByCountry[code] += requests
	}
	countries := make([]CountryTelemetry, 0, len(requestsByCountry))
	for code, requests := range requestsByCountry {
		countries = append(countries, CountryTelemetry{Code: code, Requests: requests})
	}
	slices.SortFunc(countries, func(left, right CountryTelemetry) int {
		if left.Requests > right.Requests {
			return -1
		}
		if left.Requests < right.Requests {
			return 1
		}
		return strings.Compare(left.Code, right.Code)
	})
	return countries
}

func (service *EnvironmentApplicationTelemetry) SlowQueries(
	ctx context.Context,
	applicationID uuid.UUID,
	environmentID uuid.UUID,
	telemetryRange TelemetryRange,
) ([]QueryTelemetry, error) {
	if err := service.ensureEnabled(ctx, applicationID, environmentID); err != nil {
		return nil, err
	}
	client, err := service.resource.Queries(ctx)
	if err != nil {
		return nil, err
	}
	queries, _, err := client.SlowQueries(
		ctx,
		clickhouseclient.EnvironmentTelemetryScope(environmentID.String()),
		time.Now().UTC().Add(-telemetryRange.Duration()),
		25,
	)
	if err != nil {
		return nil, err
	}
	return queryTelemetry(queries), nil
}

func (service *EnvironmentApplicationTelemetry) Logs(
	ctx context.Context,
	applicationID uuid.UUID,
	environmentID uuid.UUID,
	after string,
	telemetryRange TelemetryRange,
	search string,
	responseClass string,
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
	class, err := telemetryResponseClass(responseClass)
	if err != nil {
		return SystemLogSnapshot{}, err
	}
	client, err := service.resource.Queries(ctx)
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
		class,
		cursor,
		limit,
	)
	if err != nil {
		return SystemLogSnapshot{}, err
	}
	return systemLogSnapshot(page, after)
}

func telemetryResponseClass(value string) (uint8, error) {
	class, ok := map[string]uint8{"": 0, "2xx": 2, "3xx": 3, "4xx": 4, "5xx": 5}[value]
	if !ok {
		return 0, ErrInvalidTelemetryResponseClass
	}
	return class, nil
}

func (service *EnvironmentApplicationTelemetry) Trace(
	ctx context.Context,
	applicationID uuid.UUID,
	environmentID uuid.UUID,
	traceID string,
) ([]TraceSpan, error) {
	if err := service.ensureEnabled(ctx, applicationID, environmentID); err != nil {
		return nil, err
	}
	traceID = strings.ToLower(strings.TrimSpace(traceID))
	if len(traceID) != 32 || strings.Trim(traceID, "0123456789abcdef") != "" {
		return nil, ErrInvalidTraceID
	}
	client, err := service.resource.Queries(ctx)
	if err != nil {
		return nil, err
	}
	spans, err := client.EnvironmentTrace(ctx, environmentID.String(), traceID)
	if err != nil {
		return nil, err
	}
	return traceSpans(spans), nil
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
