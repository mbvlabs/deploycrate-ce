package clickhouse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"strconv"
	"strings"
	"time"
)

type SlowQueryResult struct {
	Query          string
	DatabaseSystem string
	Operation      string
	Executions     uint64
	P95DurationMS  float64
}

type TraceSummaryResult struct {
	TraceID       string
	RootSpanName  string
	RequestMethod string
	RequestRoute  string
	ResponseCode  uint16
	StartedAt     time.Time
	DurationNS    uint64
	SpanCount     uint64
	ErrorCount    uint64
}

type TraceSpanResult struct {
	TraceID            string
	SpanID             string
	ParentSpanID       string
	Name               string
	Kind               string
	ServiceName        string
	Scope              string
	StatusCode         string
	StatusMessage      string
	ResourceAttributes map[string]string
	SpanAttributes     map[string]string
	StartedAt          time.Time
	DurationNS         uint64
}

type TelemetryScope struct {
	where      string
	parameters map[string]string
}

func ServiceTelemetryScope(service, namespace string) TelemetryScope {
	return TelemetryScope{
		where:      "ServiceName = {service:String} AND ResourceAttributes['service.namespace'] = {namespace:String}",
		parameters: map[string]string{"service": service, "namespace": namespace},
	}
}

func EnvironmentTelemetryScope(environment string) TelemetryScope {
	return TelemetryScope{
		where:      "ResourceAttributes['deploycrate.environment.id'] = {environment:String}",
		parameters: map[string]string{"environment": environment},
	}
}

func (scope TelemetryScope) params(extra map[string]string) map[string]string {
	parameters := make(map[string]string, len(scope.parameters)+len(extra))
	maps.Copy(parameters, scope.parameters)
	maps.Copy(parameters, extra)
	return parameters
}

func telemetryQuery(template string, scope TelemetryScope) string {
	return strings.ReplaceAll(template, "{{scope}}", scope.where)
}

type RequestOverview struct {
	ObservedAt    time.Time
	WindowSeconds float64
	Requests      float64
	ClientErrors  float64
	ServerErrors  float64
	DurationSum   float64
}

type RuntimeMetric struct {
	Metric     string
	Value      float64
	ObservedAt time.Time
}

func (client Queries) RequestOverview(
	ctx context.Context,
	scope TelemetryScope,
) (*RequestOverview, error) {
	type requestRow struct {
		ObservedAtMilliseconds string  `json:"observed_at_milliseconds"`
		WindowSeconds          float64 `json:"window_seconds"`
		Requests               float64 `json:"request_count"`
		ClientErrors           float64 `json:"client_errors"`
		ServerErrors           float64 `json:"server_errors"`
		DurationSum            float64 `json:"duration_total"`
	}
	rows, err := queryJSONRows[requestRow](
		ctx,
		client,
		telemetryQuery(applicationRequestOverviewQuery, scope),
		scope.params(nil),
	)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 || rows[0].WindowSeconds <= 0 {
		return nil, nil
	}
	observedAt, err := millisecondsTime(rows[0].ObservedAtMilliseconds)
	if err != nil {
		return nil, err
	}
	return &RequestOverview{
		ObservedAt: observedAt, WindowSeconds: rows[0].WindowSeconds,
		Requests: rows[0].Requests, ClientErrors: rows[0].ClientErrors,
		ServerErrors: rows[0].ServerErrors, DurationSum: rows[0].DurationSum,
	}, nil
}

func (client Queries) RuntimeOverview(
	ctx context.Context,
	scope TelemetryScope,
) ([]RuntimeMetric, error) {
	type runtimeRow struct {
		Metric                 string  `json:"metric"`
		Value                  float64 `json:"metric_value"`
		ObservedAtMilliseconds string  `json:"observed_at_milliseconds"`
	}
	rows, err := queryJSONRows[runtimeRow](
		ctx,
		client,
		telemetryQuery(applicationRuntimeOverviewQuery, scope),
		scope.params(nil),
	)
	if err != nil {
		return nil, err
	}
	metrics := make([]RuntimeMetric, 0, len(rows))
	for _, row := range rows {
		observedAt, parseErr := millisecondsTime(row.ObservedAtMilliseconds)
		if parseErr != nil {
			return nil, parseErr
		}
		metrics = append(metrics, RuntimeMetric{
			Metric: row.Metric, Value: row.Value, ObservedAt: observedAt,
		})
	}
	return metrics, nil
}

type HistogramDelta struct {
	ObservedAt   time.Time
	Metric       string
	Status       uint16
	Operation    string
	Route        string
	Method       string
	Count        uint64
	Sum          float64
	BucketCounts []uint64
	Bounds       []float64
	Maximum      float64
}

func (client Queries) ApplicationMetricDeltas(
	ctx context.Context,
	scope TelemetryScope,
	since time.Time,
	bucket time.Duration,
) ([]HistogramDelta, error) {
	histogramQuery := telemetryQuery(applicationMetricHistoryQuery, scope)
	type histogramRow struct {
		ObservedAtMilliseconds string    `json:"observed_at_milliseconds"`
		Metric                 string    `json:"metric"`
		Status                 uint16    `json:"status"`
		Operation              string    `json:"operation"`
		Route                  string    `json:"route"`
		Method                 string    `json:"method"`
		Count                  string    `json:"count_delta_value"`
		Sum                    float64   `json:"sum_delta"`
		BucketCounts           []uint64  `json:"bucket_counts"`
		Bounds                 []float64 `json:"explicit_bounds"`
		Maximum                float64   `json:"maximum"`
	}
	parameters := scope.params(map[string]string{
		"since_seconds":  strconv.FormatInt(since.Unix(), 10),
		"bucket_seconds": strconv.FormatInt(int64(bucket/time.Second), 10),
	})
	rows, err := queryJSONRows[histogramRow](ctx, client, histogramQuery, parameters)
	if err != nil {
		return nil, err
	}

	deltas := make([]HistogramDelta, 0, len(rows))
	for _, row := range rows {
		observedAt, parseErr := millisecondsTime(row.ObservedAtMilliseconds)
		if parseErr != nil {
			return nil, parseErr
		}
		count, parseErr := strconv.ParseUint(row.Count, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf(
				"decode ClickHouse histogram count: %w",
				parseErr,
			)
		}
		deltas = append(deltas, HistogramDelta{
			ObservedAt:   observedAt,
			Metric:       row.Metric,
			Status:       row.Status,
			Operation:    row.Operation,
			Route:        row.Route,
			Method:       row.Method,
			Count:        count,
			Sum:          row.Sum,
			BucketCounts: row.BucketCounts,
			Bounds:       row.Bounds,
			Maximum:      row.Maximum,
		})
	}

	return deltas, nil
}

func (client Queries) DatabaseErrorHistory(
	ctx context.Context,
	scope TelemetryScope,
	since time.Time,
	bucket time.Duration,
) (map[int64]float64, error) {
	query := telemetryQuery(databaseErrorHistoryQuery, scope)
	type errorRow struct {
		ObservedAtMilliseconds string  `json:"observed_at_milliseconds"`
		Errors                 float64 `json:"errors"`
	}
	rows, err := queryJSONRows[errorRow](ctx, client, query, scope.params(map[string]string{
		"since_seconds":  strconv.FormatInt(since.Unix(), 10),
		"bucket_seconds": strconv.FormatInt(int64(bucket/time.Second), 10),
	}))
	if err != nil {
		return nil, err
	}
	errorsByBucket := make(map[int64]float64, len(rows))
	for _, row := range rows {
		observedAt, parseErr := millisecondsTime(row.ObservedAtMilliseconds)
		if parseErr != nil {
			return nil, parseErr
		}
		errorsByBucket[observedAt.Unix()] = row.Errors
	}
	return errorsByBucket, nil
}

func (client Queries) RecentTraces(
	ctx context.Context,
	scope TelemetryScope,
	since time.Time,
	responseClass uint8,
) ([]TraceSummaryResult, error) {
	query := telemetryQuery(recentTracesQuery, scope)
	type traceSummaryRow struct {
		TraceID              string `json:"trace_id"`
		RootSpanName         string `json:"root_span_name"`
		RequestMethod        string `json:"request_method"`
		RequestRoute         string `json:"request_route"`
		ResponseCode         uint16 `json:"response_code"`
		TimestampNanoseconds string `json:"timestamp_nanoseconds"`
		DurationNanoseconds  string `json:"duration_nanoseconds"`
		SpanCount            string `json:"span_count"`
		ErrorCount           string `json:"error_count"`
	}
	rows, err := queryJSONRows[traceSummaryRow](ctx, client, query, scope.params(map[string]string{
		"since_seconds":  strconv.FormatInt(since.Unix(), 10),
		"response_class": strconv.FormatUint(uint64(responseClass), 10),
	}))
	if err != nil {
		return nil, err
	}
	result := make([]TraceSummaryResult, 0, len(rows))
	for _, row := range rows {
		timestamp, parseErr := strconv.ParseInt(row.TimestampNanoseconds, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("decode ClickHouse trace summary timestamp: %w", parseErr)
		}
		duration, parseErr := strconv.ParseUint(row.DurationNanoseconds, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("decode ClickHouse trace summary duration: %w", parseErr)
		}
		spanCount, parseErr := strconv.ParseUint(row.SpanCount, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("decode ClickHouse trace summary span count: %w", parseErr)
		}
		errorCount, parseErr := strconv.ParseUint(row.ErrorCount, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("decode ClickHouse trace summary error count: %w", parseErr)
		}
		result = append(result, TraceSummaryResult{
			TraceID: row.TraceID, RootSpanName: row.RootSpanName,
			RequestMethod: row.RequestMethod, RequestRoute: row.RequestRoute,
			ResponseCode: row.ResponseCode,
			StartedAt:    time.Unix(0, timestamp).UTC(),
			DurationNS:   duration,
			SpanCount:    spanCount,
			ErrorCount:   errorCount,
		})
	}
	return result, nil
}

func (client Queries) SlowQueries(
	ctx context.Context,
	scope TelemetryScope,
	since time.Time,
	limit int,
) ([]SlowQueryResult, bool, error) {
	query := telemetryQuery(slowQueriesQuery, scope)
	type slowQueryRow struct {
		Query          string  `json:"query"`
		DatabaseSystem string  `json:"database_system"`
		Operation      string  `json:"operation"`
		Executions     string  `json:"executions"`
		P95DurationMS  float64 `json:"p95_duration_milliseconds"`
	}
	rows, err := queryJSONRows[slowQueryRow](ctx, client, query, scope.params(map[string]string{
		"since_seconds": strconv.FormatInt(since.Unix(), 10),
		"limit":         strconv.Itoa(limit + 1),
	}))
	if err != nil {
		return nil, false, err
	}
	queries := make([]SlowQueryResult, 0, len(rows))
	for _, row := range rows {
		executions, parseErr := strconv.ParseUint(row.Executions, 10, 64)
		if parseErr != nil {
			return nil, false, fmt.Errorf("decode ClickHouse slow query executions: %w", parseErr)
		}
		queries = append(queries, SlowQueryResult{
			Query: row.Query, DatabaseSystem: row.DatabaseSystem, Operation: row.Operation,
			Executions: executions, P95DurationMS: row.P95DurationMS,
		})
	}
	more := len(queries) > limit
	if more {
		queries = queries[:limit]
	}
	return queries, more, nil
}

func (client Queries) Trace(ctx context.Context, traceID string) ([]TraceSpanResult, error) {
	return client.trace(ctx, traceID, "")
}

func (client Queries) EnvironmentTrace(
	ctx context.Context,
	environment, traceID string,
) ([]TraceSpanResult, error) {
	return client.trace(ctx, traceID, environment)
}

func (client Queries) trace(
	ctx context.Context,
	traceID string,
	environment string,
) ([]TraceSpanResult, error) {
	type traceRow struct {
		TimestampNanoseconds string            `json:"timestamp_nanoseconds"`
		TraceID              string            `json:"trace_id"`
		SpanID               string            `json:"span_id"`
		ParentSpanID         string            `json:"parent_span_id"`
		Name                 string            `json:"name"`
		Kind                 string            `json:"kind"`
		ServiceName          string            `json:"service_name"`
		Scope                string            `json:"scope"`
		StatusCode           string            `json:"status_code"`
		StatusMessage        string            `json:"status_message"`
		ResourceAttributes   map[string]string `json:"resource_attributes"`
		SpanAttributes       map[string]string `json:"span_attributes"`
		DurationNanoseconds  string            `json:"duration_nanoseconds"`
	}
	rows, err := queryJSONRows[traceRow](ctx, client, traceQuery, map[string]string{
		"trace_id": traceID, "environment": environment,
	})
	if err != nil {
		return nil, err
	}
	spans := make([]TraceSpanResult, 0, len(rows))
	for _, row := range rows {
		timestamp, parseErr := strconv.ParseInt(row.TimestampNanoseconds, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("decode ClickHouse trace timestamp: %w", parseErr)
		}
		duration, parseErr := strconv.ParseUint(row.DurationNanoseconds, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("decode ClickHouse trace duration: %w", parseErr)
		}
		spans = append(spans, TraceSpanResult{
			TraceID: row.TraceID, SpanID: row.SpanID, ParentSpanID: row.ParentSpanID,
			Name: row.Name, Kind: row.Kind, ServiceName: row.ServiceName, Scope: row.Scope,
			StatusCode: row.StatusCode, StatusMessage: row.StatusMessage,
			ResourceAttributes: row.ResourceAttributes, SpanAttributes: row.SpanAttributes,
			StartedAt: time.Unix(0, timestamp).UTC(), DurationNS: duration,
		})
	}
	return spans, nil
}

func queryJSONRows[T any](
	ctx context.Context,
	client Queries,
	queryText string,
	parameters map[string]string,
) ([]T, error) {
	responseBody, err := client.client.Query(ctx, queryText, parameters)
	if err != nil {
		return nil, fmt.Errorf("query ClickHouse telemetry: %w", err)
	}
	defer responseBody.Close()
	rows := make([]T, 0)
	decoder := json.NewDecoder(responseBody)
	for {
		var row T
		if err := decoder.Decode(&row); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decode ClickHouse telemetry: %w", err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func millisecondsTime(value string) (time.Time, error) {
	milliseconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("decode ClickHouse telemetry timestamp: %w", err)
	}
	return time.UnixMilli(milliseconds).UTC(), nil
}
