package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	clickhouseclient "deploycrate-ce/clients/clickhouse"
	"deploycrate-ce/config"
)

const (
	systemLogBatchSize = 200
	maxSystemLogCursor = 1024
)

var ErrInvalidSystemLogCursor = errors.New("system log cursor is invalid")

type SystemLog struct {
	ID             string    `json:"id"`
	Message        string    `json:"message"`
	Severity       string    `json:"severity"`
	SeverityNumber uint8     `json:"severityNumber"`
	TraceID        string    `json:"traceId"`
	SpanID         string    `json:"spanId"`
	Scope          string    `json:"scope"`
	Source         string    `json:"source"`
	Line           string    `json:"line"`
	Instance       string    `json:"instance"`
	Slot           string    `json:"slot"`
	OccurredAt     time.Time `json:"occurredAt"`
}

type SystemLogSnapshot struct {
	Logs       []SystemLog `json:"logs"`
	NextCursor string      `json:"nextCursor"`
	HasMore    bool        `json:"hasMore"`
}

type SystemLogs struct {
	resource    *ClickHouseResource
	serviceName string
}

type systemLogCursor struct {
	TimestampNanoseconds int64  `json:"timestampNanoseconds"`
	Fingerprint          uint64 `json:"fingerprint"`
}

func NewSystemLogs(resource *ClickHouseResource, cfg config.Config) *SystemLogs {
	return &SystemLogs{resource: resource, serviceName: cfg.Telemetry.ServiceName}
}

func (service *SystemLogs) Snapshot(ctx context.Context, after string) (SystemLogSnapshot, error) {
	cursor, err := decodeSystemLogCursor(after)
	if err != nil {
		return SystemLogSnapshot{}, err
	}
	client, err := service.resource.Client(ctx)
	if err != nil {
		return SystemLogSnapshot{}, err
	}
	limit := uint64(systemLogBatchSize)
	if cursor != nil {
		limit++
	}
	page, err := client.SystemLogs(ctx, service.serviceName, cursor, limit)
	if err != nil {
		return SystemLogSnapshot{}, err
	}
	hasMore := len(page.Logs) > systemLogBatchSize
	if hasMore {
		page.Logs = page.Logs[:systemLogBatchSize]
	}

	logs := make([]SystemLog, 0, len(page.Logs))
	nextCursor := after
	for _, row := range page.Logs {
		encodedCursor, err := encodeSystemLogCursor(row.Cursor)
		if err != nil {
			return SystemLogSnapshot{}, err
		}
		nextCursor = encodedCursor
		logs = append(logs, SystemLog{
			ID: encodedCursor, Message: row.Message, Severity: row.Severity,
			SeverityNumber: row.SeverityNumber, TraceID: row.TraceID, SpanID: row.SpanID,
			Scope: row.Scope, Source: row.Source, Line: row.Line,
			Instance: row.Instance, Slot: row.Slot, OccurredAt: row.Cursor.Timestamp,
		})
	}
	return SystemLogSnapshot{Logs: logs, NextCursor: nextCursor, HasMore: hasMore}, nil
}

func encodeSystemLogCursor(cursor clickhouseclient.SystemLogCursor) (string, error) {
	payload, err := json.Marshal(systemLogCursor{
		TimestampNanoseconds: cursor.Timestamp.UnixNano(),
		Fingerprint:          cursor.Fingerprint,
	})
	if err != nil {
		return "", fmt.Errorf("encode system log cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeSystemLogCursor(value string) (*clickhouseclient.SystemLogCursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > maxSystemLogCursor {
		return nil, ErrInvalidSystemLogCursor
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, ErrInvalidSystemLogCursor
	}
	var cursor systemLogCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.TimestampNanoseconds <= 0 {
		return nil, ErrInvalidSystemLogCursor
	}
	return &clickhouseclient.SystemLogCursor{
		Timestamp:   time.Unix(0, cursor.TimestampNanoseconds).UTC(),
		Fingerprint: cursor.Fingerprint,
	}, nil
}
