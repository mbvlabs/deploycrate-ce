package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	clickhouseclient "deploycrate-ce/clients/clickhouse"
	postgresqlclient "deploycrate-ce/clients/postgresql"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
)

const resourceHealthSweepLimit = 100

type ResourceHealth struct {
	db       storage.Pool
	config   config.Config
	postgres postgresqlclient.Client
}

func NewResourceHealth(db storage.Pool, cfg config.Config) *ResourceHealth {
	return &ResourceHealth{db: db, config: cfg, postgres: postgresqlclient.New()}
}

func (service *ResourceHealth) Sweep(ctx context.Context) error {
	now := time.Now().UTC()
	checks, err := models.ResourceHealthCheck.DueApplicationChecks(
		ctx,
		service.db.Executor(),
		now,
		resourceHealthSweepLimit,
	)
	if err != nil {
		return fmt.Errorf("load due Resource health checks: %w", err)
	}

	errList := make([]error, 0)
	for _, check := range checks {
		if err := service.observe(ctx, check); err != nil {
			errList = append(errList, fmt.Errorf("observe Resource health check %s: %w", check.ID, err))
		}
	}
	return errors.Join(errList...)
}

func (service *ResourceHealth) observe(ctx context.Context, check models.DueResourceHealthCheck) error {
	startedAt := time.Now().UTC()
	timeout := time.Duration(check.TimeoutSeconds) * time.Second
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	message, probeErr := service.probe(probeCtx, check)
	cancel()
	observedAt := time.Now().UTC()

	latencyMilliseconds := min(max(observedAt.Sub(startedAt).Milliseconds(), 0), int64(^uint32(0)>>1))

	state := check.StatusState
	consecutiveSuccesses := check.StatusConsecutiveSuccesses
	consecutiveFailures := check.StatusConsecutiveFailures
	if !check.StatusPresent || !check.StatusExpiresAt.Valid || !check.StatusExpiresAt.Time.After(startedAt) {
		state = "unknown"
		consecutiveSuccesses = 0
		consecutiveFailures = 0
	}
	if probeErr == nil {
		consecutiveSuccesses++
		consecutiveFailures = 0
		if consecutiveSuccesses >= check.SuccessThreshold {
			state = "healthy"
		} else if state == "unhealthy" || state == "degraded" {
			state = "degraded"
		} else {
			state = "unknown"
		}
	} else {
		consecutiveSuccesses = 0
		consecutiveFailures++
		state = "degraded"
		if consecutiveFailures >= check.FailureThreshold {
			state = "unhealthy"
		}
		message = safeResourceHealthError(check.ResourceEngine, probeErr)
	}

	details, err := json.Marshal(map[string]any{
		"resource_engine": check.ResourceEngine,
		"check_kind":      check.Kind,
	})
	if err != nil {
		return fmt.Errorf("encode Resource health details: %w", err)
	}
	expiresAt := observedAt.Add(2*time.Duration(check.IntervalSeconds)*time.Second + timeout)
	status, err := models.ResourceHealthCheckStatus.Upsert(
		ctx,
		service.db.Executor(),
		models.CreateResourceHealthCheckStatusData{
			HealthCheckID:        check.ID,
			State:                state,
			LatencyMs:            sql.NullInt32{Int32: int32(latencyMilliseconds), Valid: true},
			Message:              nullableString(message),
			ConsecutiveSuccesses: consecutiveSuccesses,
			ConsecutiveFailures:  consecutiveFailures,
			Details:              details,
			ObservedAt:           observedAt,
			ExpiresAt:            expiresAt,
		},
	)
	if err != nil {
		return fmt.Errorf("persist Resource health status: %w", err)
	}

	slog.InfoContext(
		ctx,
		"Resource health observed",
		"event_name", "resource_health_observation",
		"resource_health_check_id", check.ID,
		"resource_id", check.ResourceID,
		"resource_name", check.ResourceName,
		"resource_endpoint_id", check.ResourceEndpointID,
		"resource_engine", check.ResourceEngine,
		"state", status.State,
		"latency_ms", status.LatencyMs.Int32,
		"consecutive_successes", status.ConsecutiveSuccesses,
		"consecutive_failures", status.ConsecutiveFailures,
		"message", status.Message.String,
	)
	return nil
}

func (service *ResourceHealth) probe(
	ctx context.Context,
	check models.DueResourceHealthCheck,
) (string, error) {
	if check.ResourceEndpointID == nil || strings.TrimSpace(check.EndpointAddress) == "" || check.EndpointPort < 1 {
		return "", errors.New("the health check endpoint is unavailable")
	}
	values, err := service.credentialValues(check)
	if err != nil {
		return "", err
	}
	password := values["password"]

	var settings struct {
		Database string `json:"database"`
		User     string `json:"user"`
	}
	if err := json.Unmarshal(check.EndpointSettings, &settings); err != nil {
		return "", errors.New("the health check endpoint settings are invalid")
	}

	switch check.Kind {
	case "postgresql":
		if !check.CredentialUsername.Valid || strings.TrimSpace(check.CredentialUsername.String) == "" || password == "" {
			return "", errors.New("the PostgreSQL health check credential is unavailable")
		}
		database := strings.TrimSpace(check.CredentialDatabaseName)
		if err := service.postgres.Check(ctx, postgresqlclient.Connection{
			Host: check.EndpointAddress, Port: check.EndpointPort,
			Username: check.CredentialUsername.String, Password: password,
		}, database, check.EndpointTLSMode); err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return "", errors.New("the PostgreSQL health probe timed out")
			}
			return "", errors.New("the PostgreSQL connection or readiness query failed")
		}
		return "PostgreSQL accepted SELECT 1", nil
	case "clickhouse":
		protocol := strings.ToLower(strings.TrimSpace(check.EndpointProtocol))
		if protocol == "clickhouse" {
			protocol = "http"
		}
		if protocol != "http" && protocol != "https" {
			return "", errors.New("the ClickHouse health check requires an HTTP endpoint")
		}
		username := strings.TrimSpace(settings.User)
		if check.CredentialUsername.Valid && strings.TrimSpace(check.CredentialUsername.String) != "" {
			username = strings.TrimSpace(check.CredentialUsername.String)
		}
		if username == "" {
			username = "default"
		}
		baseURL := (&url.URL{
			Scheme: protocol,
			Host: net.JoinHostPort(
				strings.TrimSpace(check.EndpointAddress),
				strconv.Itoa(int(check.EndpointPort)),
			),
		}).String()
		client := clickhouseclient.New(baseURL, strings.TrimSpace(settings.Database), username, password)
		response, err := client.Ping(ctx)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return "", errors.New("the ClickHouse health probe timed out")
			}
			return "", errors.New("the ClickHouse connection or ping failed")
		}
		return "ClickHouse responded " + response, nil
	default:
		return "", fmt.Errorf("unsupported Resource health check kind %q", check.Kind)
	}
}

func (service *ResourceHealth) credentialValues(check models.DueResourceHealthCheck) (map[string]string, error) {
	if check.ResourceCredentialID == nil {
		return map[string]string{}, nil
	}
	if len(check.CredentialEncryptedPayload) == 0 {
		return nil, errors.New("the health check credential is unavailable")
	}
	plaintext, err := secretcrypto.DecryptForPurpose(
		check.CredentialEncryptedPayload,
		service.config.App.SessionEncryptionKey,
		resourceCredentialPurpose,
	)
	if err != nil {
		return nil, errors.New("the health check credential could not be decrypted")
	}
	var payload struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}
	if err := json.Unmarshal(plaintext, &payload); err != nil || payload.SchemaVersion != 1 {
		return nil, errors.New("the health check credential payload is invalid")
	}
	return payload.Values, nil
}

func safeResourceHealthError(kind string, err error) string {
	message := strings.TrimSpace(err.Error())
	runes := []rune(message)
	if len(runes) > 500 {
		message = string(runes[:500])
	}
	definition, ok := models.FindResourceEngine(kind)
	if !ok {
		return "Resource health probe failed: " + message
	}
	return definition.Label + " health probe failed: " + message
}
