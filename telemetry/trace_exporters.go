package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type OtlpHttpTraceExporter struct {
	endpoint string
	headers  map[string]string
	exporter sdktrace.SpanExporter
}

func NewOtlpTraceExporter(endpoint string, headers map[string]string) *OtlpHttpTraceExporter {
	if endpoint == "" {
		return nil
	}
	return &OtlpHttpTraceExporter{
		endpoint: endpoint,
		headers:  headers,
		exporter: nil,
	}
}

func (o *OtlpHttpTraceExporter) Name() string {
	return "otlp-http-traces"
}

func (o *OtlpHttpTraceExporter) GetSpanExporter(
	ctx context.Context,
	res *resource.Resource,
) (sdktrace.SpanExporter, error) {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(o.endpoint),
	}

	if len(o.headers) > 0 {
		opts = append(opts, otlptracehttp.WithHeaders(o.headers))
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP HTTP trace exporter: %w", err)
	}

	o.exporter = exporter
	return exporter, nil
}

func (o *OtlpHttpTraceExporter) Shutdown(ctx context.Context) error {
	if o.exporter != nil {
		if err := o.exporter.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown OTLP HTTP trace exporter: %w", err)
		}
	}
	return nil
}

var _ TraceExporter = (*OtlpHttpTraceExporter)(nil)

type noopSpanExporter struct{}

func (e *noopSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e *noopSpanExporter) Shutdown(ctx context.Context) error {
	return nil
}

var _ sdktrace.SpanExporter = (*noopSpanExporter)(nil)

type NoopTraceExporter struct{}

func NewNoopTraceExporter() *NoopTraceExporter {
	return &NoopTraceExporter{}
}

func (n *NoopTraceExporter) Name() string {
	return "noop-traces"
}

func (n *NoopTraceExporter) GetSpanExporter(
	ctx context.Context,
	res *resource.Resource,
) (sdktrace.SpanExporter, error) {
	return &noopSpanExporter{}, nil
}

func (n *NoopTraceExporter) Shutdown(ctx context.Context) error {
	return nil
}

var _ TraceExporter = (*NoopTraceExporter)(nil)
