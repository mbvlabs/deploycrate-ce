package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	clickhouseclient "deploycrate-ce/clients/clickhouse"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/uuid"
)

const (
	environmentLogBatchSize = 200
	maxEnvironmentLogCursor = 1024
)

var ErrInvalidEnvironmentLogCursor = errors.New("Environment log cursor is invalid")

type EnvironmentLog struct {
	ID             string    `json:"id"`
	Message        string    `json:"message"`
	Stream         string    `json:"stream"`
	Container      string    `json:"container"`
	Deployment     string    `json:"deployment"`
	Instance       string    `json:"instance"`
	Release        string    `json:"release"`
	ProcessName    string    `json:"processName"`
	ProcessKind    string    `json:"processKind"`
	ProcessReplica string    `json:"processReplica"`
	OccurredAt     time.Time `json:"occurredAt"`
}

type EnvironmentLogSnapshot struct {
	Logs       []EnvironmentLog `json:"logs"`
	NextCursor string           `json:"nextCursor"`
	HasMore    bool             `json:"hasMore"`
}

type EnvironmentLogs struct {
	resource *ClickHouseResource
	db       storage.Pool
}

type environmentLogCursor struct {
	TimestampNanoseconds int64  `json:"timestampNanoseconds"`
	Epoch                string `json:"epoch"`
	Ordinal              uint64 `json:"ordinal"`
}

func NewEnvironmentLogs(resource *ClickHouseResource, db storage.Pool) *EnvironmentLogs {
	return &EnvironmentLogs{resource: resource, db: db}
}

func (service *EnvironmentLogs) Snapshot(
	ctx context.Context,
	applicationID uuid.UUID,
	environmentID uuid.UUID,
	after string,
) (EnvironmentLogSnapshot, error) {
	if _, err := models.Environment.FindForApplication(
		ctx,
		service.db.Executor(),
		applicationID,
		environmentID,
	); err != nil {
		return EnvironmentLogSnapshot{}, err
	}
	cursor, err := decodeEnvironmentLogCursor(after)
	if err != nil {
		return EnvironmentLogSnapshot{}, err
	}
	client, err := service.resource.Client(ctx)
	if err != nil {
		return EnvironmentLogSnapshot{}, err
	}
	limit := uint64(environmentLogBatchSize)
	if cursor != nil {
		limit++
	}
	page, err := client.EnvironmentLogs(ctx, environmentID.String(), cursor, limit)
	if err != nil {
		return EnvironmentLogSnapshot{}, err
	}
	hasMore := len(page.Logs) > environmentLogBatchSize
	if hasMore {
		page.Logs = page.Logs[:environmentLogBatchSize]
	}

	logs := make([]EnvironmentLog, 0, len(page.Logs))
	nextCursor := after
	for _, row := range page.Logs {
		encodedCursor, err := encodeEnvironmentLogCursor(row.Cursor)
		if err != nil {
			return EnvironmentLogSnapshot{}, err
		}
		nextCursor = encodedCursor
		logs = append(logs, EnvironmentLog{
			ID: encodedCursor, Message: ansi.Strip(row.Message), Stream: row.Stream,
			Container: row.Container, Deployment: row.Deployment,
			Instance: row.Instance, Release: row.Release,
			ProcessName: row.ProcessName, ProcessKind: row.ProcessKind, ProcessReplica: row.ProcessReplica,
			OccurredAt: row.Cursor.Timestamp,
		})
	}
	return EnvironmentLogSnapshot{Logs: logs, NextCursor: nextCursor, HasMore: hasMore}, nil
}

func encodeEnvironmentLogCursor(cursor clickhouseclient.LogCursor) (string, error) {
	payload, err := json.Marshal(environmentLogCursor{
		TimestampNanoseconds: cursor.Timestamp.UnixNano(),
		Epoch:                cursor.Epoch,
		Ordinal:              cursor.Ordinal,
	})
	if err != nil {
		return "", fmt.Errorf("encode Environment log cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeEnvironmentLogCursor(value string) (*clickhouseclient.LogCursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > maxEnvironmentLogCursor {
		return nil, ErrInvalidEnvironmentLogCursor
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, ErrInvalidEnvironmentLogCursor
	}
	var cursor environmentLogCursor
	if err := json.Unmarshal(payload, &cursor); err != nil ||
		cursor.TimestampNanoseconds <= 0 || cursor.Epoch == "" || cursor.Ordinal == 0 {
		return nil, ErrInvalidEnvironmentLogCursor
	}
	return &clickhouseclient.LogCursor{
		Timestamp: time.Unix(0, cursor.TimestampNanoseconds).UTC(),
		Epoch:     cursor.Epoch,
		Ordinal:   cursor.Ordinal,
	}, nil
}
