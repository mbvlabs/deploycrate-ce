package telemetry

import (
	"context"
	"fmt"
	"time"

	"deploycrate-ce/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

type MetricExporter interface {
	Name() string
	GetSdkMetricExporter(ctx context.Context, res *resource.Resource) (sdkmetric.Exporter, error)
	Shutdown(ctx context.Context) error
}

func GetMeter(serviceName string) metric.Meter {
	return otel.Meter(serviceName)
}

func HTTPRequestsInFlight() (metric.Int64UpDownCounter, error) {
	counter, err := GetMeter(config.ServiceName).Int64UpDownCounter(
		"http.server.active_requests",
		metric.WithDescription("Number of active HTTP server requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create http.server.active_requests counter: %w", err)
	}
	return counter, nil
}

func RecordMetricRollup(
	ctx context.Context,
	succeeded bool,
	insertedRows, rejectedSamples int,
	duration time.Duration,
) {
	meter := GetMeter(config.ServiceName)
	status := "success"
	if !succeeded {
		status = "error"
	}
	if runs, err := meter.Int64Counter(
		"metric_rollup_runs_total",
		metric.WithDescription("Metric rollup collection runs by outcome"),
		metric.WithUnit("1"),
	); err == nil {
		runs.Add(ctx, 1, metric.WithAttributes(attribute.String("status", status)))
	}
	if rows, err := meter.Int64Counter(
		"metric_rollup_inserted_rows_total",
		metric.WithDescription("Metric rollup rows inserted into ClickHouse"),
		metric.WithUnit("1"),
	); err == nil && insertedRows > 0 {
		rows.Add(ctx, int64(insertedRows))
	}
	if rejected, err := meter.Int64Counter(
		"metric_rollup_rejected_samples_total",
		metric.WithDescription("Metric samples rejected because stable identity validation failed"),
		metric.WithUnit("1"),
	); err == nil && rejectedSamples > 0 {
		rejected.Add(ctx, int64(rejectedSamples))
	}
	if elapsed, err := meter.Float64Histogram(
		"metric_rollup_duration_seconds",
		metric.WithDescription("Metric rollup collection duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60),
	); err == nil {
		elapsed.Record(
			ctx,
			duration.Seconds(),
			metric.WithAttributes(attribute.String("status", status)),
		)
	}
}
