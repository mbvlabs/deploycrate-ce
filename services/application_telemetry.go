package services

import (
	"context"
	"slices"
	"strings"
	"time"

	clickhouse "deploycrate-ce/database/clickhouse"
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
	Countries             []CountryTelemetry          `json:"countries"`
	Queries               []QueryTelemetry            `json:"queries"`
	MoreQueries           bool                        `json:"moreQueries"`
}

type RouteTelemetry struct {
	Route             string  `json:"route"`
	Method            string  `json:"method"`
	Requests          uint64  `json:"requests"`
	RequestsPerSecond float64 `json:"requestsPerSecond"`
	ErrorRate         float64 `json:"errorRate"`
	P95DurationMS     float64 `json:"p95DurationMs"`
}

type CountryTelemetry struct {
	Code     string `json:"code"`
	Requests uint64 `json:"requests"`
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
	TraceID       string    `json:"traceId"`
	RootSpanName  string    `json:"rootSpanName"`
	RequestMethod string    `json:"requestMethod"`
	RequestRoute  string    `json:"requestRoute"`
	ResponseCode  uint16    `json:"responseCode"`
	StartedAt     time.Time `json:"startedAt"`
	DurationNS    uint64    `json:"durationNs"`
	SpanCount     uint64    `json:"spanCount"`
	ErrorCount    uint64    `json:"errorCount"`
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

func loadApplicationTelemetry(
	ctx context.Context,
	queries clickhouse.Queries,
	scope clickhouse.TelemetryScope,
	since time.Time,
	bucket time.Duration,
	responseClass uint8,
) (ApplicationTelemetry, error) {
	result := ApplicationTelemetry{
		History:      []ApplicationTelemetryPoint{},
		Database:     DatabaseTelemetry{History: []DatabaseTelemetryPoint{}},
		RecentTraces: []TraceSummary{},
		Routes:       []RouteTelemetry{},
		Countries:    []CountryTelemetry{},
		Queries:      []QueryTelemetry{},
	}
	overview, err := queries.RequestOverview(ctx, scope)
	if err != nil {
		return ApplicationTelemetry{}, err
	}
	if overview != nil && overview.WindowSeconds > 0 {
		result.Available = true
		result.ObservedAt = overview.ObservedAt
		result.WindowSeconds = overview.WindowSeconds
		result.RequestsPerSecond = overview.Requests / overview.WindowSeconds
		if overview.Requests > 0 {
			result.ClientErrorRate = overview.ClientErrors / overview.Requests
			result.ServerErrorRate = overview.ServerErrors / overview.Requests
			result.MeanRequestDurationMS = overview.DurationSum / overview.Requests * 1000
		}
	}
	runtime, err := queries.RuntimeOverview(ctx, scope)
	if err != nil {
		return ApplicationTelemetry{}, err
	}
	for _, metric := range runtime {
		result.Available = true
		if metric.ObservedAt.After(result.ObservedAt) {
			result.ObservedAt = metric.ObservedAt
		}
		switch metric.Metric {
		case "go.memory.used":
			result.RuntimeMemoryBytes = metric.Value
		case "go.memory.allocated":
			result.HeapAllocatedBytes = metric.Value
		case "go.memory.allocations":
			result.HeapAllocations = metric.Value
		case "go.memory.gc.goal":
			result.HeapGoalBytes = metric.Value
		case "go.goroutine.count":
			result.Goroutines = metric.Value
		}
	}
	deltas, err := queries.ApplicationMetricDeltas(ctx, scope, since, bucket)
	if err != nil {
		return ApplicationTelemetry{}, err
	}
	databaseErrors, err := queries.DatabaseErrorHistory(ctx, scope, since, bucket)
	if err != nil {
		return ApplicationTelemetry{}, err
	}
	result.History, result.Database, result.Routes = aggregateApplicationHistory(
		deltas,
		databaseErrors,
		bucket,
		time.Since(since),
	)
	recentTraces, err := queries.RecentTraces(ctx, scope, since, responseClass)
	if err != nil {
		return ApplicationTelemetry{}, err
	}
	result.RecentTraces = traceSummaries(recentTraces)
	slowQueries, moreQueries, err := queries.SlowQueries(ctx, scope, since, 10)
	if err != nil {
		return ApplicationTelemetry{}, err
	}
	result.Queries = queryTelemetry(slowQueries)
	result.MoreQueries = moreQueries
	return result, nil
}

func queryTelemetry(rows []clickhouse.SlowQueryResult) []QueryTelemetry {
	result := make([]QueryTelemetry, 0, len(rows))
	for _, row := range rows {
		result = append(result, QueryTelemetry{
			Query: row.Query, DatabaseSystem: row.DatabaseSystem, Operation: row.Operation,
			Executions: row.Executions, P95DurationMS: row.P95DurationMS,
		})
	}
	return result
}

func traceSummaries(rows []clickhouse.TraceSummaryResult) []TraceSummary {
	result := make([]TraceSummary, 0, len(rows))
	for _, row := range rows {
		result = append(result, TraceSummary{
			TraceID: row.TraceID, RootSpanName: row.RootSpanName,
			RequestMethod: row.RequestMethod, RequestRoute: row.RequestRoute,
			ResponseCode: row.ResponseCode, StartedAt: row.StartedAt,
			DurationNS: row.DurationNS, SpanCount: row.SpanCount, ErrorCount: row.ErrorCount,
		})
	}
	return result
}

func traceSpans(rows []clickhouse.TraceSpanResult) []TraceSpan {
	result := make([]TraceSpan, 0, len(rows))
	for _, row := range rows {
		result = append(result, TraceSpan{
			TraceID: row.TraceID, SpanID: row.SpanID, ParentSpanID: row.ParentSpanID,
			Name: row.Name, Kind: row.Kind, ServiceName: row.ServiceName, Scope: row.Scope,
			StatusCode: row.StatusCode, StatusMessage: row.StatusMessage,
			ResourceAttributes: row.ResourceAttributes, SpanAttributes: row.SpanAttributes,
			StartedAt: row.StartedAt, DurationNS: row.DurationNS,
		})
	}
	return result
}

type aggregatedHistogram struct {
	count        uint64
	bucketCounts []uint64
	bounds       []float64
	maximum      float64
}

func (aggregate *aggregatedHistogram) add(delta clickhouse.HistogramDelta) {
	aggregate.count += delta.Count
	if len(aggregate.bucketCounts) < len(delta.BucketCounts) {
		aggregate.bucketCounts = append(
			aggregate.bucketCounts,
			make([]uint64, len(delta.BucketCounts)-len(aggregate.bucketCounts))...,
		)
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
	deltas []clickhouse.HistogramDelta,
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
				ObservedAt:        observedAt,
				RequestsPerSecond: float64(current.http.count) / bucketSeconds,
				ClientErrorsPS:    float64(current.clientErrors) / bucketSeconds,
				ServerErrorsPS:    float64(current.serverErrors) / bucketSeconds,
				P50DurationMS:     current.http.quantile(0.50) * 1000,
				P95DurationMS:     current.http.quantile(0.95) * 1000,
				P99DurationMS:     current.http.quantile(0.99) * 1000,
			})
		}
		if current.database.count > 0 {
			databaseHistory = append(databaseHistory, DatabaseTelemetryPoint{
				ObservedAt:          observedAt,
				OperationsPerSecond: float64(current.database.count) / bucketSeconds,
				ErrorsPerSecond:     databaseErrors[key] / bucketSeconds,
				P50DurationMS:       current.database.quantile(0.50) * 1000,
				P95DurationMS:       current.database.quantile(0.95) * 1000,
				P99DurationMS:       current.database.quantile(0.99) * 1000,
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
			Requests:          route.histogram.count,
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
