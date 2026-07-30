package telemetry

import (
	"context"
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
	"go.opentelemetry.io/otel/sdk/resource"
)

type StdoutExporter struct {
	LogLevel slog.Level
}

func NewStdoutExporter() *StdoutExporter {
	return &StdoutExporter{
		LogLevel: slog.LevelInfo,
	}
}

func NewStdoutExporterWithLevel(level slog.Level) *StdoutExporter {
	return &StdoutExporter{
		LogLevel: level,
	}
}

func (s *StdoutExporter) GetSlogHandler(
	_ context.Context,
	_ *resource.Resource,
) (slog.Handler, error) {
	handler := tint.NewTextHandler(
		os.Stdout,
		&tint.Options{Level: s.LogLevel, TimeFormat: "15:04:05", AddSource: true},
	)

	return handler, nil
}

func (s *StdoutExporter) Name() string {
	return "stdout"
}

func (s *StdoutExporter) Shutdown(ctx context.Context) error {
	return nil
}

var _ LogExporter = (*StdoutExporter)(nil)
