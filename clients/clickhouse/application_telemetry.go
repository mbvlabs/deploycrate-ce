package clickhouse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type ApplicationTelemetry struct {
	Available             bool      `json:"available"`
	ObservedAt            time.Time `json:"observedAt"`
	WindowSeconds         float64   `json:"windowSeconds"`
	RequestsPerSecond     float64   `json:"requestsPerSecond"`
	ServerErrorRate       float64   `json:"serverErrorRate"`
	ClientErrorRate       float64   `json:"clientErrorRate"`
	MeanRequestDurationMS float64   `json:"meanRequestDurationMs"`
	RuntimeMemoryBytes    float64   `json:"runtimeMemoryBytes"`
	HeapAllocatedBytes    float64   `json:"heapAllocatedBytes"`
	HeapAllocations       float64   `json:"heapAllocations"`
	HeapGoalBytes         float64   `json:"heapGoalBytes"`
	Goroutines            float64   `json:"goroutines"`
}

type TraceSpan struct {
	TraceID            string            `json:"traceId"`
	SpanID             string            `json:"spanId"`
	ParentSpanID       string            `json:"parentSpanId"`
	Name               string            `json:"name"`
	Kind               string            `json:"kind"`
	ServiceName        string            `json:"serviceName"`
	Scope              string            `json:"scope"`
	StatusCode         string            `json:"statusCode"`
	StatusMessage      string            `json:"statusMessage"`
	ResourceAttributes map[string]string `json:"resourceAttributes"`
	SpanAttributes     map[string]string `json:"spanAttributes"`
	StartedAt          time.Time         `json:"startedAt"`
	DurationNS         uint64            `json:"durationNs"`
}

func (client Client) ApplicationTelemetry(ctx context.Context, service, namespace string) (ApplicationTelemetry, error) {
	const requestQuery = `
SELECT
  toString(toInt64(toUnixTimestamp(max(last_time))) * 1000) AS observed_at_milliseconds,
  dateDiff('millisecond', min(first_time), max(last_time)) / 1000.0 AS window_seconds,
  sum(request_delta) AS request_count,
  sumIf(request_delta, status >= 400 AND status < 500) AS client_errors,
  sumIf(request_delta, status >= 500) AS server_errors,
  sum(duration_delta) AS duration_total
FROM
(
  SELECT
    greatest(toFloat64(argMax(Count, TimeUnix)) - toFloat64(argMin(Count, TimeUnix)), 0) AS request_delta,
    greatest(argMax(Sum, TimeUnix) - argMin(Sum, TimeUnix), 0) AS duration_delta,
    toUInt16OrZero(argMax(Attributes['http.response.status_code'], TimeUnix)) AS status,
    min(TimeUnix) AS first_time,
    max(TimeUnix) AS last_time
  FROM otel_metrics_histogram
  WHERE ServiceName = {service:String}
    AND ResourceAttributes['service.namespace'] = {namespace:String}
    AND MetricName = 'http.server.request.duration'
    AND TimeUnix >= now() - INTERVAL 5 MINUTE
  GROUP BY ResourceAttributes['service.instance.id'], cityHash64(Attributes)
  HAVING last_time > first_time
)
FORMAT JSONEachRow`
	type requestRow struct {
		ObservedAtMilliseconds string  `json:"observed_at_milliseconds"`
		WindowSeconds          float64 `json:"window_seconds"`
		Requests               float64 `json:"request_count"`
		ClientErrors           float64 `json:"client_errors"`
		ServerErrors           float64 `json:"server_errors"`
		DurationSum            float64 `json:"duration_total"`
	}
	parameters := map[string]string{"service": service, "namespace": namespace}
	requestRows, err := queryJSONRows[requestRow](ctx, client, requestQuery, parameters)
	if err != nil {
		return ApplicationTelemetry{}, err
	}

	const runtimeQuery = `
SELECT metric, sum(series_value) AS metric_value, toString(toInt64(toUnixTimestamp(max(observed_at))) * 1000) AS observed_at_milliseconds
FROM
(
  SELECT MetricName AS metric, ResourceAttributes['service.instance.id'] AS instance, cityHash64(Attributes) AS attributes,
    argMax(Value, TimeUnix) AS series_value, max(TimeUnix) AS observed_at
  FROM
  (
    SELECT ResourceAttributes, MetricName, Attributes, TimeUnix, Value FROM otel_metrics_gauge
    WHERE ServiceName = {service:String} AND ResourceAttributes['service.namespace'] = {namespace:String} AND TimeUnix >= now() - INTERVAL 5 MINUTE
    UNION ALL
    SELECT ResourceAttributes, MetricName, Attributes, TimeUnix, Value FROM otel_metrics_sum
    WHERE ServiceName = {service:String} AND ResourceAttributes['service.namespace'] = {namespace:String} AND TimeUnix >= now() - INTERVAL 5 MINUTE
  )
  WHERE MetricName IN ('go.memory.used', 'go.memory.allocated', 'go.memory.allocations', 'go.memory.gc.goal', 'go.goroutine.count')
  GROUP BY metric, instance, attributes
)
GROUP BY metric
FORMAT JSONEachRow`
	type runtimeRow struct {
		Metric                 string  `json:"metric"`
		Value                  float64 `json:"metric_value"`
		ObservedAtMilliseconds string  `json:"observed_at_milliseconds"`
	}
	runtimeRows, err := queryJSONRows[runtimeRow](ctx, client, runtimeQuery, parameters)
	if err != nil {
		return ApplicationTelemetry{}, err
	}

	result := ApplicationTelemetry{}
	if len(requestRows) == 1 && requestRows[0].WindowSeconds > 0 {
		row := requestRows[0]
		result.Available = true
		result.WindowSeconds = row.WindowSeconds
		result.RequestsPerSecond = row.Requests / row.WindowSeconds
		if row.Requests > 0 {
			result.ClientErrorRate = row.ClientErrors / row.Requests
			result.ServerErrorRate = row.ServerErrors / row.Requests
			result.MeanRequestDurationMS = row.DurationSum / row.Requests * 1000
		}
		result.ObservedAt, err = millisecondsTime(row.ObservedAtMilliseconds)
		if err != nil {
			return ApplicationTelemetry{}, err
		}
	}
	for _, row := range runtimeRows {
		observedAt, parseErr := millisecondsTime(row.ObservedAtMilliseconds)
		if parseErr != nil {
			return ApplicationTelemetry{}, parseErr
		}
		result.Available = true
		if observedAt.After(result.ObservedAt) {
			result.ObservedAt = observedAt
		}
		switch row.Metric {
		case "go.memory.used":
			result.RuntimeMemoryBytes = row.Value
		case "go.memory.allocated":
			result.HeapAllocatedBytes = row.Value
		case "go.memory.allocations":
			result.HeapAllocations = row.Value
		case "go.memory.gc.goal":
			result.HeapGoalBytes = row.Value
		case "go.goroutine.count":
			result.Goroutines = row.Value
		}
	}
	return result, nil
}

func (client Client) Trace(ctx context.Context, traceID string) ([]TraceSpan, error) {
	const traceQuery = `
SELECT toString(toUnixTimestamp64Nano(Timestamp)) AS timestamp_nanoseconds,
  TraceId AS trace_id, SpanId AS span_id, ParentSpanId AS parent_span_id,
  SpanName AS name, SpanKind AS kind, ServiceName AS service_name, ScopeName AS scope,
  StatusCode AS status_code, StatusMessage AS status_message,
  ResourceAttributes AS resource_attributes, SpanAttributes AS span_attributes,
  toString(Duration) AS duration_nanoseconds
FROM otel_traces
WHERE TraceId = {trace_id:String}
ORDER BY Timestamp, Duration DESC
LIMIT 1000
FORMAT JSONEachRow`
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
	rows, err := queryJSONRows[traceRow](ctx, client, traceQuery, map[string]string{"trace_id": traceID})
	if err != nil {
		return nil, err
	}
	spans := make([]TraceSpan, 0, len(rows))
	for _, row := range rows {
		timestamp, parseErr := strconv.ParseInt(row.TimestampNanoseconds, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("decode ClickHouse trace timestamp: %w", parseErr)
		}
		duration, parseErr := strconv.ParseUint(row.DurationNanoseconds, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("decode ClickHouse trace duration: %w", parseErr)
		}
		spans = append(spans, TraceSpan{
			TraceID: row.TraceID, SpanID: row.SpanID, ParentSpanID: row.ParentSpanID,
			Name: row.Name, Kind: row.Kind, ServiceName: row.ServiceName, Scope: row.Scope,
			StatusCode: row.StatusCode, StatusMessage: row.StatusMessage,
			ResourceAttributes: row.ResourceAttributes, SpanAttributes: row.SpanAttributes,
			StartedAt: time.Unix(0, timestamp).UTC(), DurationNS: duration,
		})
	}
	return spans, nil
}

func queryJSONRows[T any](ctx context.Context, client Client, queryText string, parameters map[string]string) ([]T, error) {
	endpoint, err := url.Parse(client.baseURL)
	if err != nil {
		return nil, fmt.Errorf("build ClickHouse URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("database", client.database)
	query.Set("query", queryText)
	for key, value := range parameters {
		query.Set("param_"+key, value)
	}
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build ClickHouse telemetry request: %w", err)
	}
	request.SetBasicAuth(client.user, client.password)
	response, err := client.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("query ClickHouse telemetry: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 1200))
		return nil, fmt.Errorf("query ClickHouse telemetry: unexpected status %s: %s", response.Status, string(message))
	}
	rows := make([]T, 0)
	decoder := json.NewDecoder(response.Body)
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
