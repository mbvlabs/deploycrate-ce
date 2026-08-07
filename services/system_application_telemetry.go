package services

import (
	"context"
	"errors"
	"strings"
	"time"

	clickhouseclient "deploycrate-ce/clients/clickhouse"
	"deploycrate-ce/config"
)

var ErrInvalidTraceID = errors.New("trace ID is invalid")

type TelemetryRange string

const (
	TelemetryRangeOneHour   TelemetryRange = "1h"
	TelemetryRangeSixHours  TelemetryRange = "6h"
	TelemetryRangeOneDay    TelemetryRange = "24h"
	TelemetryRangeSevenDays TelemetryRange = "7d"
)

func ParseTelemetryRange(value string) TelemetryRange {
	switch TelemetryRange(strings.TrimSpace(value)) {
	case TelemetryRangeOneHour:
		return TelemetryRangeOneHour
	case TelemetryRangeSixHours:
		return TelemetryRangeSixHours
	case TelemetryRangeSevenDays:
		return TelemetryRangeSevenDays
	default:
		return TelemetryRangeOneDay
	}
}

func (telemetryRange TelemetryRange) Duration() time.Duration {
	switch telemetryRange {
	case TelemetryRangeOneHour:
		return time.Hour
	case TelemetryRangeSixHours:
		return 6 * time.Hour
	case TelemetryRangeSevenDays:
		return 7 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func (telemetryRange TelemetryRange) Bucket() time.Duration {
	switch telemetryRange {
	case TelemetryRangeOneHour:
		return time.Minute
	case TelemetryRangeSixHours:
		return 5 * time.Minute
	case TelemetryRangeSevenDays:
		return 2 * time.Hour
	default:
		return 15 * time.Minute
	}
}

type SystemApplicationTelemetry struct {
	resource    *ClickHouseResource
	serviceName string
	namespace   string
}

func NewSystemApplicationTelemetry(
	resource *ClickHouseResource,
	cfg config.Config,
) *SystemApplicationTelemetry {
	return &SystemApplicationTelemetry{
		resource:    resource,
		serviceName: cfg.Telemetry.ServiceName,
		namespace:   cfg.Telemetry.ServiceNamespace,
	}
}

func (service *SystemApplicationTelemetry) Snapshot(
	ctx context.Context,
	telemetryRange TelemetryRange,
) (clickhouseclient.ApplicationTelemetry, error) {
	client, err := service.resource.Client(ctx)
	if err != nil {
		return clickhouseclient.ApplicationTelemetry{}, err
	}
	return client.ApplicationTelemetry(
		ctx,
		service.serviceName,
		service.namespace,
		time.Now().UTC().Add(-telemetryRange.Duration()),
		telemetryRange.Bucket(),
	)
}

func (service *SystemApplicationTelemetry) Trace(
	ctx context.Context,
	traceID string,
) ([]clickhouseclient.TraceSpan, error) {
	traceID = strings.ToLower(strings.TrimSpace(traceID))
	if len(traceID) != 32 || strings.Trim(traceID, "0123456789abcdef") != "" {
		return nil, ErrInvalidTraceID
	}
	client, err := service.resource.Client(ctx)
	if err != nil {
		return nil, err
	}
	return client.Trace(ctx, traceID)
}
