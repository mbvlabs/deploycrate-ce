package clickhouse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

type ApplicationTelemetry struct {
	Available             bool                        `json:"available"`
	ObservedAt            time.Time                   `json:"observedAt"`
	WindowSeconds         float64                     `json:"windowSeconds"`
	RequestsPerSecond     float64                     `json:"requestsPerSecond"`
	ServerErrorRate       float64                     `json:"serverErrorRate"`
	ClientErrorRate       float64                     `json:"clientErrorRate"`
	MeanRequestDurationMS float64                     `json:"meanRequestDurationMs"`
	RuntimeMemoryBytes    float64                     `json:"runtimeMemoryBytes"`
	HeapAllocatedBytes    float64                     `json:"heapAllocatedBytes"`
	HeapAllocations       float64                     `json:"heapAllocations"`
	HeapGoalBytes         float64                     `json:"heapGoalBytes"`
	Goroutines            float64                     `json:"goroutines"`
	History               []ApplicationTelemetryPoint `json:"history"`
	Database              DatabaseTelemetry           `json:"database"`
	RecentTraces          []TraceSummary              `json:"recentTraces"`
	Routes                []RouteTelemetry            `json:"routes"`
	Queries               []QueryTelemetry            `json:"queries"`
	MoreQueries           bool                        `json:"moreQueries"`
}

type RouteTelemetry struct {
	Route             string  `json:"route"`
	Method            string  `json:"method"`
	RequestsPerSecond float64 `json:"requestsPerSecond"`
	ErrorRate         float64 `json:"errorRate"`
	P95DurationMS     float64 `json:"p95DurationMs"`
}

type QueryTelemetry struct {
	Query          string  `json:"query"`
	DatabaseSystem string  `json:"databaseSystem"`
	Operation      string  `json:"operation"`
	Executions     uint64  `json:"executions"`
	P95DurationMS  float64 `json:"p95DurationMs"`
}

type ApplicationTelemetryPoint struct {
	ObservedAt        time.Time `json:"observedAt"`
	RequestsPerSecond float64   `json:"requestsPerSecond"`
	ClientErrorsPS    float64   `json:"clientErrorsPerSecond"`
	ServerErrorsPS    float64   `json:"serverErrorsPerSecond"`
	P50DurationMS     float64   `json:"p50DurationMs"`
	P95DurationMS     float64   `json:"p95DurationMs"`
	P99DurationMS     float64   `json:"p99DurationMs"`
}

type DatabaseTelemetry struct {
	Available           bool                     `json:"available"`
	ObservedAt          time.Time                `json:"observedAt"`
	OperationsPerSecond float64                  `json:"operationsPerSecond"`
	ErrorsPerSecond     float64                  `json:"errorsPerSecond"`
	P95DurationMS       float64                  `json:"p95DurationMs"`
	History             []DatabaseTelemetryPoint `json:"history"`
}

type DatabaseTelemetryPoint struct {
	ObservedAt          time.Time `json:"observedAt"`
	OperationsPerSecond float64   `json:"operationsPerSecond"`
	ErrorsPerSecond     float64   `json:"errorsPerSecond"`
	P50DurationMS       float64   `json:"p50DurationMs"`
	P95DurationMS       float64   `json:"p95DurationMs"`
	P99DurationMS       float64   `json:"p99DurationMs"`
}

type TraceSummary struct {
	TraceID      string    `json:"traceId"`
	RootSpanName string    `json:"rootSpanName"`
	StartedAt    time.Time `json:"startedAt"`
	DurationNS   uint64    `json:"durationNs"`
	SpanCount    uint64    `json:"spanCount"`
	ErrorCount   uint64    `json:"errorCount"`
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

type applicationTelemetryScope struct {
	where      string
	parameters map[string]string
}

func serviceTelemetryScope(service, namespace string) applicationTelemetryScope {
	return applicationTelemetryScope{
		where:      "ServiceName = {service:String} AND ResourceAttributes['service.namespace'] = {namespace:String}",
		parameters: map[string]string{"service": service, "namespace": namespace},
	}
}

func environmentTelemetryScope(environment string) applicationTelemetryScope {
	return applicationTelemetryScope{
		where:      "ResourceAttributes['deploycrate.environment.id'] = {environment:String}",
		parameters: map[string]string{"environment": environment},
	}
}

func (scope applicationTelemetryScope) params(extra map[string]string) map[string]string {
	parameters := make(map[string]string, len(scope.parameters)+len(extra))
	maps.Copy(parameters, scope.parameters)
	maps.Copy(parameters, extra)
	return parameters
}

func telemetryQuery(template string, scope applicationTelemetryScope) string {
	return strings.ReplaceAll(template, "{{scope}}", scope.where)
}

func (client Client) ApplicationTelemetry(
	ctx context.Context,
	service string,
	namespace string,
	since time.Time,
	bucket time.Duration,
) (ApplicationTelemetry, error) {
	return client.applicationTelemetry(ctx, serviceTelemetryScope(service, namespace), since, bucket)
}

func (client Client) EnvironmentApplicationTelemetry(
	ctx context.Context,
	environment string,
	since time.Time,
	bucket time.Duration,
) (ApplicationTelemetry, error) {
	return client.applicationTelemetry(ctx, environmentTelemetryScope(environment), since, bucket)
}

func (client Client) EnvironmentSlowQueries(
	ctx context.Context,
	environment string,
	since time.Time,
) ([]QueryTelemetry, error) {
	queries, _, err := client.slowQueries(ctx, environmentTelemetryScope(environment), since, 25)
	return queries, err
}

func (client Client) applicationTelemetry(
	ctx context.Context,
	scope applicationTelemetryScope,
	since time.Time,
	bucket time.Duration,
) (ApplicationTelemetry, error) {
	requestQuery := telemetryQuery(`
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
  WHERE {{scope}}
    AND MetricName = 'http.server.request.duration'
    AND Attributes['http.route'] != '/api/health'
    AND TimeUnix >= now() - INTERVAL 5 MINUTE
  GROUP BY ResourceAttributes['service.instance.id'], cityHash64(Attributes)
  HAVING last_time > first_time
)
FORMAT JSONEachRow`, scope)
	type requestRow struct {
		ObservedAtMilliseconds string  `json:"observed_at_milliseconds"`
		WindowSeconds          float64 `json:"window_seconds"`
		Requests               float64 `json:"request_count"`
		ClientErrors           float64 `json:"client_errors"`
		ServerErrors           float64 `json:"server_errors"`
		DurationSum            float64 `json:"duration_total"`
	}
	parameters := scope.params(nil)
	requestRows, err := queryJSONRows[requestRow](ctx, client, requestQuery, parameters)
	if err != nil {
		return ApplicationTelemetry{}, err
	}

	runtimeQuery := telemetryQuery(`
SELECT metric, sum(series_value) AS metric_value, toString(toInt64(toUnixTimestamp(max(observed_at))) * 1000) AS observed_at_milliseconds
FROM
(
  SELECT MetricName AS metric, ResourceAttributes['service.instance.id'] AS instance, cityHash64(Attributes) AS attributes,
    argMax(Value, TimeUnix) AS series_value, max(TimeUnix) AS observed_at
  FROM
  (
    SELECT ResourceAttributes, MetricName, Attributes, TimeUnix, Value FROM otel_metrics_gauge
    WHERE {{scope}} AND TimeUnix >= now() - INTERVAL 5 MINUTE
    UNION ALL
    SELECT ResourceAttributes, MetricName, Attributes, TimeUnix, Value FROM otel_metrics_sum
    WHERE {{scope}} AND TimeUnix >= now() - INTERVAL 5 MINUTE
  )
  WHERE MetricName IN ('go.memory.used', 'go.memory.allocated', 'go.memory.allocations', 'go.memory.gc.goal', 'go.goroutine.count')
  GROUP BY metric, instance, attributes
)
GROUP BY metric
FORMAT JSONEachRow`, scope)
	type runtimeRow struct {
		Metric                 string  `json:"metric"`
		Value                  float64 `json:"metric_value"`
		ObservedAtMilliseconds string  `json:"observed_at_milliseconds"`
	}
	runtimeRows, err := queryJSONRows[runtimeRow](ctx, client, runtimeQuery, parameters)
	if err != nil {
		return ApplicationTelemetry{}, err
	}

	result := ApplicationTelemetry{
		History:      []ApplicationTelemetryPoint{},
		Database:     DatabaseTelemetry{History: []DatabaseTelemetryPoint{}},
		RecentTraces: []TraceSummary{},
		Routes:       []RouteTelemetry{},
		Queries:      []QueryTelemetry{},
	}
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

	history, database, routes, err := client.applicationMetricHistory(ctx, scope, since, bucket)
	if err != nil {
		return ApplicationTelemetry{}, err
	}
	result.History = history
	result.Database = database
	result.Routes = routes
	result.RecentTraces, err = client.recentTraces(ctx, scope, since)
	if err != nil {
		return ApplicationTelemetry{}, err
	}
	result.Queries, result.MoreQueries, err = client.slowQueries(ctx, scope, since, 10)
	if err != nil {
		return ApplicationTelemetry{}, err
	}
	return result, nil
}

type histogramDelta struct {
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

func (client Client) applicationMetricHistory(
	ctx context.Context,
	scope applicationTelemetryScope,
	since time.Time,
	bucket time.Duration,
) ([]ApplicationTelemetryPoint, DatabaseTelemetry, []RouteTelemetry, error) {
	histogramQuery := telemetryQuery(`
SELECT
  toString(toUInt64(toUnixTimestamp(bucket_start)) * 1000) AS observed_at_milliseconds,
  metric, status, operation, route, method, toString(count_delta) AS count_delta_value, sum_delta,
  bucket_counts, explicit_bounds, maximum
FROM
(
  SELECT
    toStartOfInterval(TimeUnix, toIntervalSecond({bucket_seconds:UInt32})) AS bucket_start,
    MetricName AS metric,
    toUInt16OrZero(Attributes['http.response.status_code']) AS status,
    Attributes['pgx.operation.type'] AS operation,
	coalesce(nullIf(Attributes['http.route'], ''), nullIf(Attributes['url.path'], ''), Attributes['http.target']) AS route,
	coalesce(nullIf(Attributes['http.request.method'], ''), Attributes['http.method']) AS method,
    greatest(toInt64(argMax(Count, TimeUnix)) - toInt64(argMin(Count, TimeUnix)), 0) AS count_delta,
    greatest(argMax(Sum, TimeUnix) - argMin(Sum, TimeUnix), 0) AS sum_delta,
	arrayMap((latest, earliest) -> toUInt64(if(
		latest >= earliest,
		toInt64(latest) - toInt64(earliest),
		toInt64(latest)
	)),
	  argMax(BucketCounts, TimeUnix), argMin(BucketCounts, TimeUnix)) AS bucket_counts,
    argMax(ExplicitBounds, TimeUnix) AS explicit_bounds,
    max(Max) AS maximum
  FROM otel_metrics_histogram
  WHERE {{scope}}
    AND MetricName IN ('http.server.request.duration', 'db.client.operation.duration')
    AND TimeUnix >= toDateTime({since_seconds:UInt32})
    AND (MetricName != 'http.server.request.duration' OR Attributes['http.route'] != '/api/health')
	AND (MetricName != 'db.client.operation.duration' OR Attributes['pgx.operation.type'] IN ('query', 'batch'))
  GROUP BY bucket_start, metric, ResourceAttributes['service.instance.id'], cityHash64(Attributes), status, operation, route, method
  HAVING max(TimeUnix) > min(TimeUnix)
)
WHERE count_delta > 0
ORDER BY bucket_start, metric, status, operation
FORMAT JSONEachRow`, scope)
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
		return nil, DatabaseTelemetry{}, nil, err
	}

	deltas := make([]histogramDelta, 0, len(rows))
	for _, row := range rows {
		observedAt, parseErr := millisecondsTime(row.ObservedAtMilliseconds)
		if parseErr != nil {
			return nil, DatabaseTelemetry{}, nil, parseErr
		}
		count, parseErr := strconv.ParseUint(row.Count, 10, 64)
		if parseErr != nil {
			return nil, DatabaseTelemetry{}, nil, fmt.Errorf("decode ClickHouse histogram count: %w", parseErr)
		}
		deltas = append(deltas, histogramDelta{
			ObservedAt: observedAt, Metric: row.Metric, Status: row.Status, Operation: row.Operation,
			Route: row.Route, Method: row.Method,
			Count: count, Sum: row.Sum, BucketCounts: row.BucketCounts, Bounds: row.Bounds, Maximum: row.Maximum,
		})
	}

	databaseErrors, err := client.databaseErrorHistory(ctx, scope, since, bucket)
	if err != nil {
		return nil, DatabaseTelemetry{}, nil, err
	}
	httpHistory, database, routes := aggregateApplicationHistory(deltas, databaseErrors, bucket, time.Since(since))
	return httpHistory, database, routes, nil
}

func (client Client) databaseErrorHistory(
	ctx context.Context,
	scope applicationTelemetryScope,
	since time.Time,
	bucket time.Duration,
) (map[int64]float64, error) {
	query := telemetryQuery(`
SELECT toString(toUInt64(toUnixTimestamp(bucket_start)) * 1000) AS observed_at_milliseconds,
  sum(error_delta) AS errors
FROM
(
  SELECT toStartOfInterval(TimeUnix, toIntervalSecond({bucket_seconds:UInt32})) AS bucket_start,
    greatest(argMax(Value, TimeUnix) - argMin(Value, TimeUnix), 0) AS error_delta
  FROM otel_metrics_sum
  WHERE {{scope}}
    AND MetricName = 'db.client.operation.errors'
    AND TimeUnix >= toDateTime({since_seconds:UInt32})
	AND Attributes['pgx.operation.type'] IN ('query', 'batch')
  GROUP BY bucket_start, ResourceAttributes['service.instance.id'], cityHash64(Attributes)
  HAVING max(TimeUnix) > min(TimeUnix)
)
GROUP BY bucket_start
ORDER BY bucket_start
FORMAT JSONEachRow`, scope)
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

type aggregatedHistogram struct {
	count        uint64
	bucketCounts []uint64
	bounds       []float64
	maximum      float64
}

func (aggregate *aggregatedHistogram) add(delta histogramDelta) {
	aggregate.count += delta.Count
	if len(aggregate.bucketCounts) < len(delta.BucketCounts) {
		aggregate.bucketCounts = append(aggregate.bucketCounts, make([]uint64, len(delta.BucketCounts)-len(aggregate.bucketCounts))...)
	}
	for index, count := range delta.BucketCounts {
		aggregate.bucketCounts[index] += count
	}
	if len(delta.Bounds) > len(aggregate.bounds) {
		aggregate.bounds = append([]float64(nil), delta.Bounds...)
	}
	if delta.Maximum > aggregate.maximum {
		aggregate.maximum = delta.Maximum
	}
}

func (aggregate aggregatedHistogram) quantile(value float64) float64 {
	if aggregate.count == 0 {
		return 0
	}
	target := uint64(float64(aggregate.count)*value + 0.999999)
	var cumulative uint64
	for index, count := range aggregate.bucketCounts {
		cumulative += count
		if cumulative < target {
			continue
		}
		if index < len(aggregate.bounds) {
			// Histogram boundaries are upper bounds, not sampled durations. This is
			// particularly important for Environment workloads, which can use the
			// OpenTelemetry default boundaries (for example, a 5 s bucket). Max is
			// recorded with the histogram, so it prevents a fast operation from
			// being reported as the upper edge of its coarse bucket.
			if aggregate.maximum > 0 && aggregate.maximum < aggregate.bounds[index] {
				return aggregate.maximum
			}
			return aggregate.bounds[index]
		}
		return aggregate.maximum
	}
	return aggregate.maximum
}

func aggregateApplicationHistory(
	deltas []histogramDelta,
	databaseErrors map[int64]float64,
	bucket time.Duration,
	window time.Duration,
) ([]ApplicationTelemetryPoint, DatabaseTelemetry, []RouteTelemetry) {
	type bucketData struct {
		http, database aggregatedHistogram
		clientErrors   uint64
		serverErrors   uint64
	}
	buckets := make(map[int64]*bucketData)
	type routeData struct {
		method    string
		route     string
		errors    uint64
		histogram aggregatedHistogram
	}
	routesByKey := make(map[string]*routeData)
	for _, delta := range deltas {
		key := delta.ObservedAt.Unix()
		current := buckets[key]
		if current == nil {
			current = &bucketData{}
			buckets[key] = current
		}
		switch delta.Metric {
		case "http.server.request.duration":
			current.http.add(delta)
			if delta.Route != "" {
				key := delta.Method + "\x00" + delta.Route
				route := routesByKey[key]
				if route == nil {
					route = &routeData{method: delta.Method, route: delta.Route}
					routesByKey[key] = route
				}
				route.histogram.add(delta)
				if delta.Status >= 400 {
					route.errors += delta.Count
				}
			}
			if delta.Status >= 500 {
				current.serverErrors += delta.Count
			} else if delta.Status >= 400 {
				current.clientErrors += delta.Count
			}
		case "db.client.operation.duration":
			current.database.add(delta)
		}
	}
	keys := make([]int64, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	bucketSeconds := bucket.Seconds()
	httpHistory := make([]ApplicationTelemetryPoint, 0, len(keys))
	databaseHistory := make([]DatabaseTelemetryPoint, 0, len(keys))
	for _, key := range keys {
		current := buckets[key]
		observedAt := time.Unix(key, 0).UTC()
		if current.http.count > 0 {
			httpHistory = append(httpHistory, ApplicationTelemetryPoint{
				ObservedAt: observedAt, RequestsPerSecond: float64(current.http.count) / bucketSeconds,
				ClientErrorsPS: float64(current.clientErrors) / bucketSeconds,
				ServerErrorsPS: float64(current.serverErrors) / bucketSeconds,
				P50DurationMS:  current.http.quantile(0.50) * 1000,
				P95DurationMS:  current.http.quantile(0.95) * 1000,
				P99DurationMS:  current.http.quantile(0.99) * 1000,
			})
		}
		if current.database.count > 0 {
			databaseHistory = append(databaseHistory, DatabaseTelemetryPoint{
				ObservedAt: observedAt, OperationsPerSecond: float64(current.database.count) / bucketSeconds,
				ErrorsPerSecond: databaseErrors[key] / bucketSeconds,
				P50DurationMS:   current.database.quantile(0.50) * 1000,
				P95DurationMS:   current.database.quantile(0.95) * 1000,
				P99DurationMS:   current.database.quantile(0.99) * 1000,
			})
		}
	}
	database := DatabaseTelemetry{History: databaseHistory}
	if len(databaseHistory) > 0 {
		latest := databaseHistory[len(databaseHistory)-1]
		database.Available = true
		database.ObservedAt = latest.ObservedAt
		database.OperationsPerSecond = latest.OperationsPerSecond
		database.ErrorsPerSecond = latest.ErrorsPerSecond
		database.P95DurationMS = latest.P95DurationMS
	}
	routes := make([]RouteTelemetry, 0, len(routesByKey))
	windowSeconds := window.Seconds()
	if windowSeconds <= 0 {
		windowSeconds = 1
	}
	for _, route := range routesByKey {
		row := RouteTelemetry{
			Route: route.route, Method: route.method,
			RequestsPerSecond: float64(route.histogram.count) / windowSeconds,
			P95DurationMS:     route.histogram.quantile(0.95) * 1000,
		}
		if route.histogram.count > 0 {
			row.ErrorRate = float64(route.errors) / float64(route.histogram.count)
		}
		routes = append(routes, row)
	}
	slices.SortFunc(routes, func(left, right RouteTelemetry) int {
		if left.P95DurationMS > right.P95DurationMS {
			return -1
		}
		if left.P95DurationMS < right.P95DurationMS {
			return 1
		}
		return strings.Compare(left.Route, right.Route)
	})
	return httpHistory, database, routes
}

func (client Client) recentTraces(
	ctx context.Context,
	scope applicationTelemetryScope,
	since time.Time,
) ([]TraceSummary, error) {
	query := telemetryQuery(`
SELECT TraceId AS trace_id,
  argMaxIf(SpanName, Duration, ParentSpanId = '') AS root_span_name,
  toString(toUnixTimestamp64Nano(min(Timestamp))) AS timestamp_nanoseconds,
  toString(if(countIf(ParentSpanId = '') > 0, argMaxIf(Duration, Duration, ParentSpanId = ''), max(Duration))) AS duration_nanoseconds,
  toString(count()) AS span_count,
  toString(countIf(lower(StatusCode) = 'error')) AS error_count
FROM otel_traces
WHERE {{scope}}
  AND Timestamp >= toDateTime({since_seconds:UInt32})
GROUP BY TraceId
HAVING countIf(SpanAttributes['http.route'] = '/api/health') = 0
ORDER BY min(Timestamp) DESC
LIMIT 100
FORMAT JSONEachRow`, scope)
	type traceSummaryRow struct {
		TraceID              string `json:"trace_id"`
		RootSpanName         string `json:"root_span_name"`
		TimestampNanoseconds string `json:"timestamp_nanoseconds"`
		DurationNanoseconds  string `json:"duration_nanoseconds"`
		SpanCount            string `json:"span_count"`
		ErrorCount           string `json:"error_count"`
	}
	rows, err := queryJSONRows[traceSummaryRow](ctx, client, query, scope.params(map[string]string{
		"since_seconds": strconv.FormatInt(since.Unix(), 10),
	}))
	if err != nil {
		return nil, err
	}
	result := make([]TraceSummary, 0, len(rows))
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
		result = append(result, TraceSummary{
			TraceID: row.TraceID, RootSpanName: row.RootSpanName, StartedAt: time.Unix(0, timestamp).UTC(),
			DurationNS: duration, SpanCount: spanCount, ErrorCount: errorCount,
		})
	}
	return result, nil
}

func (client Client) slowQueries(
	ctx context.Context,
	scope applicationTelemetryScope,
	since time.Time,
	limit int,
) ([]QueryTelemetry, bool, error) {
	query := telemetryQuery(`
SELECT query, database_system, operation, toString(count()) AS executions,
  quantile(0.95)(Duration) / 1000000.0 AS p95_duration_milliseconds
FROM
(
  SELECT
    if(
      SpanAttributes['db.query.text'] != '',
      SpanAttributes['db.query.text'],
      SpanAttributes['db.statement']
    ) AS query,
    if(
      SpanAttributes['db.system.name'] != '',
      SpanAttributes['db.system.name'],
      SpanAttributes['db.system']
    ) AS database_system,
    if(
      SpanAttributes['db.operation.name'] != '',
      SpanAttributes['db.operation.name'],
      SpanAttributes['pgx.operation.type']
    ) AS operation,
    Duration
  FROM otel_traces
  WHERE {{scope}}
    AND Timestamp >= toDateTime({since_seconds:UInt32})
)
WHERE query != ''
GROUP BY query, database_system, operation
ORDER BY p95_duration_milliseconds DESC, executions DESC, query
LIMIT {limit:UInt8}
FORMAT JSONEachRow`, scope)
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
	queries := make([]QueryTelemetry, 0, len(rows))
	for _, row := range rows {
		executions, parseErr := strconv.ParseUint(row.Executions, 10, 64)
		if parseErr != nil {
			return nil, false, fmt.Errorf("decode ClickHouse slow query executions: %w", parseErr)
		}
		queries = append(queries, QueryTelemetry{
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

func (client Client) Trace(ctx context.Context, traceID string) ([]TraceSpan, error) {
	return client.trace(ctx, traceID, "", nil)
}

func (client Client) EnvironmentTrace(ctx context.Context, environment, traceID string) ([]TraceSpan, error) {
	return client.trace(
		ctx,
		traceID,
		" AND TraceId IN (SELECT TraceId FROM otel_traces WHERE TraceId = {trace_id:String} AND ResourceAttributes['deploycrate.environment.id'] = {environment:String})",
		map[string]string{"environment": environment},
	)
}

func (client Client) trace(
	ctx context.Context,
	traceID string,
	scopeFilter string,
	parameters map[string]string,
) ([]TraceSpan, error) {
	traceQuery := `
SELECT toString(toUnixTimestamp64Nano(Timestamp)) AS timestamp_nanoseconds,
  TraceId AS trace_id, SpanId AS span_id, ParentSpanId AS parent_span_id,
  SpanName AS name, SpanKind AS kind, ServiceName AS service_name, ScopeName AS scope,
  StatusCode AS status_code, StatusMessage AS status_message,
  ResourceAttributes AS resource_attributes, SpanAttributes AS span_attributes,
  toString(Duration) AS duration_nanoseconds
FROM otel_traces
WHERE TraceId = {trace_id:String}` + scopeFilter + `
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
	if parameters == nil {
		parameters = map[string]string{}
	}
	parameters["trace_id"] = traceID
	rows, err := queryJSONRows[traceRow](ctx, client, traceQuery, parameters)
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
