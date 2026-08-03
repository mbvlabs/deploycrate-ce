package services

import (
	"context"
	"errors"
	"strings"

	clickhouseclient "deploycrate-ce/clients/clickhouse"
	"deploycrate-ce/config"
)

var ErrInvalidTraceID = errors.New("trace ID is invalid")

type SystemApplicationTelemetry struct {
	resource    *ClickHouseResource
	serviceName string
	namespace   string
}

func NewSystemApplicationTelemetry(resource *ClickHouseResource, cfg config.Config) *SystemApplicationTelemetry {
	return &SystemApplicationTelemetry{resource: resource, serviceName: cfg.Telemetry.ServiceName, namespace: cfg.Telemetry.ServiceNamespace}
}

func (service *SystemApplicationTelemetry) Snapshot(ctx context.Context) (clickhouseclient.ApplicationTelemetry, error) {
	client, err := service.resource.Client(ctx)
	if err != nil {
		return clickhouseclient.ApplicationTelemetry{}, err
	}
	return client.ApplicationTelemetry(ctx, service.serviceName, service.namespace)
}

func (service *SystemApplicationTelemetry) Trace(ctx context.Context, traceID string) ([]clickhouseclient.TraceSpan, error) {
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
