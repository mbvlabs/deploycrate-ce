package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

type OtlpLogExporter struct {
	endpoint     string
	headers      map[string]string
	batchSize    int
	batchTimeout time.Duration
	queueSize    int
	provider     *sdklog.LoggerProvider
}

func NewOtlpLogExporter(
	endpoint string,
	headers map[string]string,
	batchSize int,
	batchTimeout time.Duration,
	queueSize int,
) *OtlpLogExporter {
	return &OtlpLogExporter{
		endpoint:     endpoint,
		headers:      headers,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		queueSize:    queueSize,
	}
}

func (o *OtlpLogExporter) GetSlogHandler(
	ctx context.Context,
	res *resource.Resource,
) (slog.Handler, error) {
	endpoint, err := normalizeOtlpLogsEndpoint(o.endpoint)
	if err != nil {
		return nil, err
	}

	options := []otlploghttp.Option{otlploghttp.WithEndpointURL(endpoint)}
	if len(o.headers) > 0 {
		options = append(options, otlploghttp.WithHeaders(o.headers))
	}
	exporter, err := otlploghttp.New(ctx, options...)
	if err != nil {
		return nil, err
	}

	processor := sdklog.NewBatchProcessor(
		exporter,
		sdklog.WithMaxQueueSize(o.queueSize),
		sdklog.WithExportMaxBatchSize(o.batchSize),
		sdklog.WithExportInterval(o.batchTimeout),
	)
	o.provider = sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(processor),
	)

	return otelslog.NewHandler(
		"deploycrate-ce",
		otelslog.WithLoggerProvider(o.provider),
		otelslog.WithSource(true),
	), nil
}

func (o *OtlpLogExporter) Name() string {
	return "otlp-logs"
}

func (o *OtlpLogExporter) Shutdown(ctx context.Context) error {
	if o.provider == nil {
		return nil
	}
	return o.provider.Shutdown(ctx)
}

func normalizeOtlpLogsEndpoint(endpoint string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("otlp logs endpoint must be an absolute HTTP URL")
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/v1/logs"
	}
	return parsed.String(), nil
}

var _ LogExporter = (*OtlpLogExporter)(nil)
