package queue

import (
	"context"

	"deploycrate-ce/telemetry"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type TelemetryMiddleware struct {
	river.MiddlewareDefaults
}

func (TelemetryMiddleware) Work(
	ctx context.Context,
	job *rivertype.JobRow,
	doInner func(context.Context) error,
) error {
	ctx, span := telemetry.GetTracer("deploycrate-ce/queue").Start(
		ctx,
		"river.process "+job.Kind,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.Int64("river.job.id", job.ID),
			attribute.String("river.job.kind", job.Kind),
			attribute.String("river.job.queue", job.Queue),
			attribute.Int("river.job.attempt", job.Attempt),
			attribute.Int("river.job.max_attempts", job.MaxAttempts),
		),
	)
	defer span.End()

	err := doInner(ctx)
	telemetry.RecErr(span, err)
	return err
}

var _ rivertype.WorkerMiddleware = (*TelemetryMiddleware)(nil)
