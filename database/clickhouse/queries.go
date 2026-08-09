package clickhouse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"time"

	clickhouseclient "deploycrate-ce/clients/clickhouse"
)

type MetricRollup struct {
	BucketStart   time.Time `json:"bucket_start"`
	ObservedAt    time.Time `json:"observed_at"`
	Scope         string    `json:"scope"`
	Component     string    `json:"component"`
	Metric        string    `json:"metric"`
	Average       float64   `json:"average"`
	Maximum       float64   `json:"maximum"`
	Last          float64   `json:"last"`
	Server        string    `json:"server"`
	Application   string    `json:"application"`
	Environment   string    `json:"environment"`
	Release       string    `json:"release"`
	Deployment    string    `json:"deployment"`
	Target        string    `json:"target"`
	Instance      string    `json:"instance"`
	Resource      string    `json:"resource"`
	Installation  string    `json:"installation"`
	RuntimeID     string    `json:"runtime_id"`
	ObservationID string    `json:"observation_id"`
}

type MetricRollupExport struct {
	Rows        int64  `json:"rows"`
	FirstBucket string `json:"first_bucket,omitempty"`
	LastBucket  string `json:"last_bucket,omitempty"`
}

type MetricValue struct {
	Metric     string
	Value      float64
	ObservedAt time.Time
}

type MetricHistoryValue struct {
	BucketStart time.Time
	Metric      string
	Value       float64
}

type AttributedMetricValue struct {
	Scope        string
	Component    string
	Application  string
	Environment  string
	Release      string
	Deployment   string
	Target       string
	Instance     string
	Resource     string
	Installation string
	RuntimeID    string
	Metric       string
	Value        float64
	ObservedAt   time.Time
	BucketStart  time.Time
}

type LogCursor struct {
	Timestamp time.Time
	Epoch     string
	Ordinal   uint64
}

type EnvironmentLog struct {
	Cursor         LogCursor
	Message        string
	Stream         string
	Container      string
	Deployment     string
	Instance       string
	Release        string
	ProcessName    string
	ProcessKind    string
	ProcessReplica string
}

type EnvironmentLogPage struct {
	Logs []EnvironmentLog
}

type SystemLogCursor struct {
	Timestamp   time.Time
	Fingerprint uint64
}

type SystemLog struct {
	Cursor         SystemLogCursor
	Message        string
	Severity       string
	SeverityNumber uint8
	Attributes     map[string]string
	TraceID        string
	SpanID         string
	Scope          string
	Source         string
	Line           string
	Instance       string
	Slot           string
	Service        string
	ProcessName    string
	ProcessKind    string
	ProcessReplica string
}

type SystemLogPage struct {
	Logs []SystemLog
}

type Queries struct {
	client clickhouseclient.Client
}

func NewQueries(baseURL, database, user, password string) Queries {
	return Queries{client: clickhouseclient.New(baseURL, database, user, password)}
}

func (client Queries) InsertMetricRollups(ctx context.Context, rollups []MetricRollup) error {
	if len(rollups) == 0 {
		return nil
	}
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	for _, rollup := range rollups {
		if err := encoder.Encode(rollup); err != nil {
			return fmt.Errorf("encode ClickHouse metric rollup: %w", err)
		}
	}
	if err := client.client.Insert(
		ctx,
		insertMetricRollupsQuery,
		nil,
		map[string]string{"date_time_input_format": "best_effort"},
		&body,
		"application/x-ndjson",
	); err != nil {
		return fmt.Errorf("insert ClickHouse metric rollups: %w", err)
	}
	return nil
}

func (client Queries) LatestSystemMetricValues(
	ctx context.Context,
	server string,
) ([]MetricValue, error) {
	type metricRow struct {
		Metric                 string  `json:"metric"`
		Value                  float64 `json:"value"`
		ObservedAtMilliseconds string  `json:"observed_at_milliseconds"`
	}
	rows, err := queryJSONRows[metricRow](
		ctx,
		client,
		latestSystemMetricsQuery,
		map[string]string{"server": server},
	)
	if err != nil {
		return nil, fmt.Errorf("query ClickHouse system metrics: %w", err)
	}
	values := make([]MetricValue, 0, len(rows))
	for _, row := range rows {
		observedAtMilliseconds, err := strconv.ParseInt(
			row.ObservedAtMilliseconds,
			10,
			64,
		)
		if err != nil {
			return nil, fmt.Errorf("decode ClickHouse system metric timestamp: %w", err)
		}
		values = append(values, MetricValue{
			Metric:     row.Metric,
			Value:      row.Value,
			ObservedAt: time.UnixMilli(observedAtMilliseconds).UTC(),
		})
	}
	return values, nil
}

func (client Queries) SystemMetricHistory(
	ctx context.Context,
	server string,
	since time.Time,
	bucket time.Duration,
) ([]MetricHistoryValue, error) {
	type historyRow struct {
		BucketStartMilliseconds string  `json:"bucket_start_milliseconds"`
		Metric                  string  `json:"metric"`
		Value                   float64 `json:"value"`
	}
	rows, err := queryJSONRows[historyRow](ctx, client, systemMetricHistoryQuery, map[string]string{
		"server":         server,
		"since_seconds":  strconv.FormatInt(since.Unix(), 10),
		"bucket_seconds": strconv.FormatInt(int64(bucket/time.Second), 10),
	})
	if err != nil {
		return nil, fmt.Errorf("query ClickHouse system metric history: %w", err)
	}
	values := make([]MetricHistoryValue, 0, len(rows))
	for _, row := range rows {
		bucketStartMilliseconds, err := strconv.ParseInt(
			row.BucketStartMilliseconds,
			10,
			64,
		)
		if err != nil {
			return nil, fmt.Errorf("decode ClickHouse system metric history timestamp: %w", err)
		}
		values = append(values, MetricHistoryValue{
			BucketStart: time.UnixMilli(bucketStartMilliseconds).UTC(),
			Metric:      row.Metric,
			Value:       row.Value,
		})
	}
	return values, nil
}

func (client Queries) LatestAttributedMetricValues(
	ctx context.Context,
	scope string,
	server string,
	environment string,
) ([]AttributedMetricValue, error) {
	return client.queryAttributedMetrics(
		ctx,
		latestAttributedMetricsQuery,
		scope,
		server,
		environment,
		time.Time{},
		0,
	)
}

func (client Queries) AttributedMetricHistory(
	ctx context.Context,
	scope string,
	server string,
	environment string,
	since time.Time,
	bucket time.Duration,
) ([]AttributedMetricValue, error) {
	return client.queryAttributedMetrics(
		ctx,
		attributedMetricHistoryQuery,
		scope,
		server,
		environment,
		since,
		bucket,
	)
}

func (client Queries) queryAttributedMetrics(
	ctx context.Context,
	queryText string,
	scope string,
	server string,
	environment string,
	since time.Time,
	bucket time.Duration,
) ([]AttributedMetricValue, error) {
	parameters := map[string]string{
		"scope": scope, "server": server, "environment": environment,
	}
	if !since.IsZero() {
		parameters["since_seconds"] = strconv.FormatInt(since.Unix(), 10)
		parameters["bucket_seconds"] = strconv.FormatInt(int64(bucket/time.Second), 10)
	}
	type attributedMetricRow struct {
		BucketStart            string  `json:"bucket_start"`
		Scope                  string  `json:"scope"`
		Component              string  `json:"component"`
		Application            string  `json:"application"`
		Environment            string  `json:"environment"`
		Release                string  `json:"release"`
		Deployment             string  `json:"deployment"`
		Target                 string  `json:"target"`
		Instance               string  `json:"instance"`
		Resource               string  `json:"resource"`
		Installation           string  `json:"installation"`
		RuntimeID              string  `json:"runtime_id"`
		Metric                 string  `json:"metric"`
		Value                  float64 `json:"value"`
		ObservedAtMilliseconds string  `json:"observed_at_milliseconds"`
	}
	rows, err := queryJSONRows[attributedMetricRow](ctx, client, queryText, parameters)
	if err != nil {
		return nil, fmt.Errorf("query ClickHouse attributed metrics: %w", err)
	}
	result := make([]AttributedMetricValue, 0, len(rows))
	for _, row := range rows {
		observedAtMilliseconds, err := strconv.ParseInt(row.ObservedAtMilliseconds, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("decode ClickHouse attributed metric timestamp: %w", err)
		}
		bucketStart := time.Time{}
		if row.BucketStart != "" {
			bucketStart, err = time.Parse("2006-01-02 15:04:05", row.BucketStart)
			if err != nil {
				return nil, fmt.Errorf("decode ClickHouse attributed metric bucket: %w", err)
			}
			bucketStart = bucketStart.UTC()
		}
		result = append(result, AttributedMetricValue{
			Scope: row.Scope, Component: row.Component, Application: row.Application,
			Environment: row.Environment, Release: row.Release, Deployment: row.Deployment,
			Target: row.Target, Instance: row.Instance, Resource: row.Resource,
			Installation: row.Installation, RuntimeID: row.RuntimeID, Metric: row.Metric,
			Value: row.Value, ObservedAt: time.UnixMilli(observedAtMilliseconds).UTC(),
			BucketStart: bucketStart,
		})
	}
	return result, nil
}

func (client Queries) EnvironmentLogs(
	ctx context.Context,
	environment string,
	after *LogCursor,
	limit uint64,
) (EnvironmentLogPage, error) {
	parameters := map[string]string{
		"environment": environment,
		"limit":       strconv.FormatUint(limit, 10),
	}
	queryText := environmentLogsInitialQuery
	if after != nil {
		queryText = environmentLogsIncrementalQuery
		parameters["after_nanoseconds"] = strconv.FormatInt(after.Timestamp.UnixNano(), 10)
		parameters["after_epoch"] = after.Epoch
		parameters["after_ordinal"] = strconv.FormatUint(after.Ordinal, 10)
	}
	type environmentLogRow struct {
		TimestampNanoseconds string `json:"timestamp_nanoseconds"`
		Message              string `json:"message"`
		Stream               string `json:"stream"`
		Container            string `json:"container"`
		Deployment           string `json:"deployment"`
		Instance             string `json:"instance"`
		Release              string `json:"release"`
		ProcessName          string `json:"process_name"`
		ProcessKind          string `json:"process_kind"`
		ProcessReplica       string `json:"process_replica"`
		Epoch                string `json:"epoch"`
		Ordinal              string `json:"ordinal"`
	}
	rows, err := queryJSONRows[environmentLogRow](ctx, client, queryText, parameters)
	if err != nil {
		return EnvironmentLogPage{}, fmt.Errorf("query ClickHouse Environment logs: %w", err)
	}
	logs := make([]EnvironmentLog, 0, len(rows))
	for _, row := range rows {
		timestampNanoseconds, err := strconv.ParseInt(row.TimestampNanoseconds, 10, 64)
		if err != nil {
			return EnvironmentLogPage{}, fmt.Errorf(
				"decode ClickHouse Environment log timestamp: %w",
				err,
			)
		}
		ordinal, err := strconv.ParseUint(row.Ordinal, 10, 64)
		if err != nil {
			return EnvironmentLogPage{}, fmt.Errorf(
				"decode ClickHouse Environment log ordinal: %w",
				err,
			)
		}
		logs = append(logs, EnvironmentLog{
			Cursor: LogCursor{
				Timestamp: time.Unix(0, timestampNanoseconds).UTC(),
				Epoch:     row.Epoch,
				Ordinal:   ordinal,
			},
			Message:        row.Message,
			Stream:         row.Stream,
			Container:      row.Container,
			Deployment:     row.Deployment,
			Instance:       row.Instance,
			Release:        row.Release,
			ProcessName:    row.ProcessName,
			ProcessKind:    row.ProcessKind,
			ProcessReplica: row.ProcessReplica,
		})
	}
	if after == nil {
		slices.Reverse(logs)
	}
	return EnvironmentLogPage{Logs: logs}, nil
}

func (client Queries) SystemLogs(
	ctx context.Context,
	service string,
	since time.Time,
	search string,
	after *SystemLogCursor,
	limit uint64,
) (SystemLogPage, error) {
	return client.telemetryLogs(
		ctx,
		"system",
		map[string]string{"service": service, "environment": ""},
		since,
		search,
		after,
		limit,
	)
}

func (client Queries) EnvironmentTelemetryLogs(
	ctx context.Context,
	environment string,
	since time.Time,
	search string,
	after *SystemLogCursor,
	limit uint64,
) (SystemLogPage, error) {
	return client.telemetryLogs(
		ctx,
		"environment",
		map[string]string{"service": "", "environment": environment},
		since,
		search,
		after,
		limit,
	)
}

func (client Queries) telemetryLogs(
	ctx context.Context,
	scope string,
	parameters map[string]string,
	since time.Time,
	search string,
	after *SystemLogCursor,
	limit uint64,
) (SystemLogPage, error) {
	parameters["scope"] = scope
	parameters["since_nanoseconds"] = strconv.FormatInt(since.UnixNano(), 10)
	parameters["limit"] = strconv.FormatUint(limit, 10)
	parameters["search"] = search
	queryText := telemetryLogsInitialQuery
	if after != nil {
		queryText = telemetryLogsIncrementalQuery
		parameters["after_nanoseconds"] = strconv.FormatInt(after.Timestamp.UnixNano(), 10)
		parameters["after_fingerprint"] = strconv.FormatUint(after.Fingerprint, 10)
	}
	type systemLogRow struct {
		TimestampNanoseconds string            `json:"timestamp_nanoseconds"`
		Fingerprint          string            `json:"fingerprint"`
		Message              string            `json:"message"`
		Severity             string            `json:"severity"`
		SeverityNumber       uint8             `json:"severity_number"`
		Attributes           map[string]string `json:"attributes"`
		TraceID              string            `json:"trace_id"`
		SpanID               string            `json:"span_id"`
		Scope                string            `json:"scope"`
		Source               string            `json:"source"`
		Line                 string            `json:"line"`
		Instance             string            `json:"instance"`
		Slot                 string            `json:"slot"`
		Service              string            `json:"service"`
		ProcessName          string            `json:"process_name"`
		ProcessKind          string            `json:"process_kind"`
		ProcessReplica       string            `json:"process_replica"`
	}
	rows, err := queryJSONRows[systemLogRow](ctx, client, queryText, parameters)
	if err != nil {
		return SystemLogPage{}, fmt.Errorf("query ClickHouse telemetry logs: %w", err)
	}
	logs := make([]SystemLog, 0, len(rows))
	for _, row := range rows {
		timestampNanoseconds, err := strconv.ParseInt(row.TimestampNanoseconds, 10, 64)
		if err != nil {
			return SystemLogPage{}, fmt.Errorf("decode ClickHouse telemetry log timestamp: %w", err)
		}
		fingerprint, err := strconv.ParseUint(row.Fingerprint, 10, 64)
		if err != nil {
			return SystemLogPage{}, fmt.Errorf(
				"decode ClickHouse telemetry log fingerprint: %w",
				err,
			)
		}
		logs = append(logs, SystemLog{
			Cursor: SystemLogCursor{
				Timestamp:   time.Unix(0, timestampNanoseconds).UTC(),
				Fingerprint: fingerprint,
			},
			Message: row.Message, Severity: row.Severity, SeverityNumber: row.SeverityNumber,
			Attributes: row.Attributes,
			TraceID:    row.TraceID, SpanID: row.SpanID, Scope: row.Scope,
			Source: row.Source, Line: row.Line, Instance: row.Instance, Slot: row.Slot,
			Service: row.Service, ProcessName: row.ProcessName,
			ProcessKind: row.ProcessKind, ProcessReplica: row.ProcessReplica,
		})
	}
	if after == nil {
		slices.Reverse(logs)
	}
	return SystemLogPage{Logs: logs}, nil
}

func (client Queries) ExportMetricRollups(
	ctx context.Context,
	destination io.Writer,
) (MetricRollupExport, error) {
	responseBody, err := client.client.Query(ctx, exportMetricRollupsQuery, nil)
	if err != nil {
		return MetricRollupExport{}, fmt.Errorf("export ClickHouse metric rollups: %w", err)
	}
	defer responseBody.Close()

	decoder := json.NewDecoder(io.TeeReader(responseBody, destination))
	result := MetricRollupExport{}
	for {
		var row struct {
			BucketStart string `json:"bucket_start"`
		}
		if err := decoder.Decode(&row); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return MetricRollupExport{}, fmt.Errorf(
				"decode ClickHouse metric rollup export: %w",
				err,
			)
		}
		if row.BucketStart == "" {
			return MetricRollupExport{}, errors.New(
				"ClickHouse metric rollup export is missing bucket_start",
			)
		}
		result.Rows++
		if result.FirstBucket == "" || row.BucketStart < result.FirstBucket {
			result.FirstBucket = row.BucketStart
		}
		if result.LastBucket == "" || row.BucketStart > result.LastBucket {
			result.LastBucket = row.BucketStart
		}
	}
	return result, nil
}
