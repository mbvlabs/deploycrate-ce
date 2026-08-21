package services

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	clickhouseclient "deploycrate-ce/database/clickhouse"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const (
	requestObservationInitialWindow = 10 * time.Minute
	requestObservationOverlap       = 5 * time.Minute
	requestObservationSettleTime    = 10 * time.Second
	requestObservationBatchSize     = 5000
)

type RequestTelemetry struct {
	Routes    []RouteTelemetry   `json:"routes"`
	Countries []CountryTelemetry `json:"countries"`
}

type RequestAnalytics struct {
	resource *ClickHouseResource
	db       storage.Pool
	geo      GeoResolver
	now      func() time.Time
}

func NewRequestAnalytics(
	resource *ClickHouseResource,
	db storage.Pool,
	geo GeoResolver,
) *RequestAnalytics {
	return &RequestAnalytics{resource: resource, db: db, geo: geo, now: time.Now}
}

func (service *RequestAnalytics) Process(ctx context.Context) error {
	queries, err := service.resource.Queries(ctx)
	if err != nil {
		return err
	}
	processedAt := service.now().UTC()
	before := processedAt.Add(-requestObservationSettleTime)
	latest, err := queries.LatestRequestObservation(ctx)
	if err != nil {
		return err
	}
	since := before.Add(-requestObservationInitialWindow)
	if !latest.IsZero() {
		since = latest.Add(-requestObservationOverlap)
	}
	if !since.Before(before) {
		return nil
	}
	requests, err := queries.CaddyRequests(
		ctx,
		since,
		before,
		requestObservationBatchSize,
	)
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		return nil
	}
	identities, err := models.EnvironmentDomain.ActiveRequestObservationIdentities(
		ctx,
		service.db.Executor(),
	)
	if err != nil {
		return fmt.Errorf("load request observation identities: %w", err)
	}
	byDomain := make(map[string]models.RequestObservationIdentity, len(identities))
	for _, identity := range identities {
		byDomain[models.NormalizeHostname(identity.Hostname)] = identity
	}

	addresses := make([]string, 0, len(requests))
	seenAddresses := make(map[string]struct{}, len(requests))
	for _, request := range requests {
		address := normalizedAddress(request.ClientAddress)
		if address == "" {
			continue
		}
		if _, seen := seenAddresses[address]; seen {
			continue
		}
		seenAddresses[address] = struct{}{}
		addresses = append(addresses, address)
	}
	locations, _ := service.geo.Resolve(ctx, addresses)
	observations := make([]clickhouseclient.RequestObservation, 0, len(requests))
	for _, request := range requests {
		domain := normalizedRequestHost(request.Host)
		identity, found := byDomain[domain]
		if !found {
			continue
		}
		path := literalRequestPath(request.URI)
		if path == "" {
			continue
		}
		country := strings.ToUpper(strings.TrimSpace(
			locations[normalizedAddress(request.ClientAddress)].CountryCode,
		))
		if len(country) != 2 {
			country = ""
		}
		observations = append(observations, clickhouseclient.RequestObservation{
			ObservedAt: request.ObservedAt, ProcessedAt: processedAt,
			Fingerprint:   request.Fingerprint,
			ApplicationID: identity.ApplicationID.String(),
			EnvironmentID: identity.EnvironmentID.String(),
			Domain:        domain, Method: strings.ToUpper(strings.TrimSpace(request.Method)),
			Path: path, StatusCode: request.StatusCode, CountryCode: country,
			DurationMS: request.DurationMS,
		})
	}
	return queries.InsertRequestObservations(ctx, observations)
}

func (service *RequestAnalytics) Snapshot(
	ctx context.Context,
	applicationID uuid.UUID,
	environmentID uuid.UUID,
	telemetryRange TelemetryRange,
) (RequestTelemetry, error) {
	result := RequestTelemetry{
		Routes: []RouteTelemetry{}, Countries: []CountryTelemetry{},
	}
	if _, err := models.Environment.FindForApplication(
		ctx,
		service.db.Executor(),
		applicationID,
		environmentID,
	); err != nil {
		return result, err
	}
	queries, err := service.resource.Queries(ctx)
	if err != nil {
		return result, err
	}
	since := service.now().UTC().Add(-telemetryRange.Duration())
	var (
		paths     []clickhouseclient.RequestPathResult
		countries []clickhouseclient.RequestCountryResult
	)
	group, groupContext := errgroup.WithContext(ctx)
	group.Go(func() error {
		var queryErr error
		paths, queryErr = queries.RequestPaths(groupContext, environmentID.String(), since)
		return queryErr
	})
	group.Go(func() error {
		var queryErr error
		countries, queryErr = queries.RequestCountries(
			groupContext,
			environmentID.String(),
			since,
		)
		return queryErr
	})
	if err := group.Wait(); err != nil {
		return result, err
	}
	windowSeconds := telemetryRange.Duration().Seconds()
	for _, path := range paths {
		route := RouteTelemetry{
			Route: path.Path, Method: path.Method, Requests: path.Requests,
			RequestsPerSecond: float64(path.Requests) / windowSeconds,
			P95DurationMS:     path.P95DurationMS,
		}
		if path.Requests > 0 {
			route.ErrorRate = float64(path.Errors) / float64(path.Requests)
		}
		result.Routes = append(result.Routes, route)
	}
	for _, country := range countries {
		result.Countries = append(result.Countries, CountryTelemetry{
			Code: country.Code, Requests: country.Requests,
		})
	}
	return result, nil
}

func normalizedRequestHost(value string) string {
	value = strings.TrimSpace(value)
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	return models.NormalizeHostname(value)
}

func normalizedAddress(value string) string {
	address := net.ParseIP(strings.TrimSpace(value))
	if address == nil {
		return ""
	}
	return address.String()
}

func literalRequestPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.ParseRequestURI(value)
	if err == nil && parsed.Path != "" {
		return parsed.EscapedPath()
	}
	path, _, _ := strings.Cut(value, "?")
	if !strings.HasPrefix(path, "/") {
		return ""
	}
	return path
}
