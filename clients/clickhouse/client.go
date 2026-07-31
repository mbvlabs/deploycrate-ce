package clickhouse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"deploycrate-ce/telemetry"
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
	Cursor     LogCursor
	Message    string
	Stream     string
	Container  string
	Deployment string
	Instance   string
	Release    string
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
	TraceID        string
	SpanID         string
	Scope          string
	Source         string
	Line           string
	Instance       string
	Slot           string
}

type SystemLogPage struct {
	Logs []SystemLog
}

type Client struct {
	baseURL  string
	database string
	user     string
	password string
	client   *http.Client
}

func New(baseURL, database, user, password string) Client {
	return Client{
		baseURL: baseURL, database: database, user: user, password: password,
		client: telemetry.NewHTTPClient(15 * time.Second),
	}
}

func (client Client) Ping(ctx context.Context) (string, error) {
	endpoint, err := url.Parse(client.baseURL)
	if err != nil {
		return "", fmt.Errorf("build ClickHouse ping URL: %w", err)
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/ping"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build ClickHouse ping request: %w", err)
	}
	request.SetBasicAuth(client.user, client.password)
	response, err := client.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("ping ClickHouse: %w", err)
	}
	defer response.Body.Close()
	message, err := io.ReadAll(io.LimitReader(response.Body, 800))
	if err != nil {
		return "", fmt.Errorf("read ClickHouse ping response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"ping ClickHouse: unexpected status %s: %s",
			response.Status,
			string(message),
		)
	}
	return strings.TrimSpace(string(message)), nil
}

func (client Client) InsertMetricRollups(ctx context.Context, rollups []MetricRollup) error {
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
	endpoint, err := url.Parse(client.baseURL)
	if err != nil {
		return fmt.Errorf("build ClickHouse URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("database", client.database)
	query.Set("query", "INSERT INTO metric_rollups FORMAT JSONEachRow")
	query.Set("date_time_input_format", "best_effort")
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), &body)
	if err != nil {
		return fmt.Errorf("build ClickHouse insert: %w", err)
	}
	request.SetBasicAuth(client.user, client.password)
	request.Header.Set("Content-Type", "application/x-ndjson")
	response, err := client.client.Do(request)
	if err != nil {
		return fmt.Errorf("insert ClickHouse metric rollups: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("insert ClickHouse metric rollups: unexpected status %s", response.Status)
	}
	return nil
}

func (client Client) LatestSystemMetricValues(
	ctx context.Context,
	server string,
) ([]MetricValue, error) {
	endpoint, err := url.Parse(client.baseURL)
	if err != nil {
		return nil, fmt.Errorf("build ClickHouse URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("database", client.database)
	query.Set("param_server", server)
	query.Set(
		"query",
		"SELECT metric, argMax(`last`, observed_at) AS value, toString(toUnixTimestamp64Milli(max(observed_at))) AS observed_at_milliseconds FROM metric_rollups WHERE scope = 'host' AND server = {server:String} AND metric IN ('cpu_cores_used', 'cpu_cores_total', 'memory_available_bytes', 'memory_total_bytes', 'root_filesystem_available_bytes', 'root_filesystem_size_bytes', 'disk_read_bytes_per_second', 'disk_write_bytes_per_second', 'network_receive_bytes_per_second', 'network_transmit_bytes_per_second', 'oom_events', 'tasks') GROUP BY metric FORMAT JSONEachRow",
	)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build ClickHouse system metric request: %w", err)
	}
	request.SetBasicAuth(client.user, client.password)
	response, err := client.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("query ClickHouse system metrics: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 800))
		return nil, fmt.Errorf(
			"query ClickHouse system metrics: unexpected status %s: %s",
			response.Status,
			string(message),
		)
	}

	values := make([]MetricValue, 0, 12)
	decoder := json.NewDecoder(response.Body)
	for {
		var row struct {
			Metric                 string  `json:"metric"`
			Value                  float64 `json:"value"`
			ObservedAtMilliseconds string  `json:"observed_at_milliseconds"`
		}
		if err := decoder.Decode(&row); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decode ClickHouse system metrics: %w", err)
		}
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

func (client Client) SystemMetricHistory(
	ctx context.Context,
	server string,
	since time.Time,
) ([]MetricHistoryValue, error) {
	endpoint, err := url.Parse(client.baseURL)
	if err != nil {
		return nil, fmt.Errorf("build ClickHouse URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("database", client.database)
	query.Set("param_server", server)
	query.Set("param_since_seconds", strconv.FormatInt(since.Unix(), 10))
	query.Set(
		"query",
		"SELECT toString(toUInt64(toUnixTimestamp(bucket_start)) * 1000) AS bucket_start_milliseconds, metric, argMax(`last`, observed_at) AS value FROM metric_rollups WHERE scope = 'host' AND server = {server:String} AND bucket_start >= toDateTime({since_seconds:UInt32}) AND metric IN ('cpu_cores_used', 'cpu_cores_total', 'memory_available_bytes', 'memory_total_bytes', 'root_filesystem_available_bytes', 'root_filesystem_size_bytes', 'disk_read_bytes_per_second', 'disk_write_bytes_per_second', 'network_receive_bytes_per_second', 'network_transmit_bytes_per_second', 'oom_events', 'tasks') GROUP BY bucket_start, metric ORDER BY bucket_start, metric FORMAT JSONEachRow",
	)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build ClickHouse system metric history request: %w", err)
	}
	request.SetBasicAuth(client.user, client.password)
	response, err := client.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("query ClickHouse system metric history: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 800))
		return nil, fmt.Errorf(
			"query ClickHouse system metric history: unexpected status %s: %s",
			response.Status,
			string(message),
		)
	}

	values := make([]MetricHistoryValue, 0, 24*60*4)
	decoder := json.NewDecoder(response.Body)
	for {
		var row struct {
			BucketStartMilliseconds string  `json:"bucket_start_milliseconds"`
			Metric                  string  `json:"metric"`
			Value                   float64 `json:"value"`
		}
		if err := decoder.Decode(&row); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decode ClickHouse system metric history: %w", err)
		}
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

func (client Client) LatestAttributedMetricValues(
	ctx context.Context,
	scope string,
	server string,
	environment string,
) ([]AttributedMetricValue, error) {
	const query = "SELECT scope, component, application, environment, release, deployment, target, instance, resource, installation, argMax(runtime_id, observed_at) AS runtime_id, metric, argMax(`last`, observed_at) AS value, toString(toUnixTimestamp64Milli(max(observed_at))) AS observed_at_milliseconds FROM metric_rollups WHERE scope = {scope:String} AND server = {server:String} AND ({environment:String} = '' OR environment = {environment:String}) GROUP BY scope, component, application, environment, release, deployment, target, instance, resource, installation, metric ORDER BY component, application, environment, instance, installation, metric FORMAT JSONEachRow"
	return client.queryAttributedMetrics(ctx, query, scope, server, environment, time.Time{})
}

func (client Client) AttributedMetricHistory(
	ctx context.Context,
	scope string,
	server string,
	environment string,
	since time.Time,
) ([]AttributedMetricValue, error) {
	const query = "SELECT bucket_start, scope, component, application, environment, release, deployment, target, instance, resource, installation, argMax(runtime_id, observed_at) AS runtime_id, metric, argMax(`last`, observed_at) AS value, toString(toUnixTimestamp64Milli(max(observed_at))) AS observed_at_milliseconds FROM metric_rollups WHERE scope = {scope:String} AND server = {server:String} AND ({environment:String} = '' OR environment = {environment:String}) AND bucket_start >= toDateTime({since_seconds:UInt32}) GROUP BY bucket_start, scope, component, application, environment, release, deployment, target, instance, resource, installation, metric ORDER BY bucket_start, component, application, environment, instance, installation, metric FORMAT JSONEachRow"
	return client.queryAttributedMetrics(ctx, query, scope, server, environment, since)
}

func (client Client) queryAttributedMetrics(
	ctx context.Context,
	queryText string,
	scope string,
	server string,
	environment string,
	since time.Time,
) ([]AttributedMetricValue, error) {
	endpoint, err := url.Parse(client.baseURL)
	if err != nil {
		return nil, fmt.Errorf("build ClickHouse URL: %w", err)
	}
	values := endpoint.Query()
	values.Set("database", client.database)
	values.Set("param_scope", scope)
	values.Set("param_server", server)
	values.Set("param_environment", environment)
	if !since.IsZero() {
		values.Set("param_since_seconds", strconv.FormatInt(since.Unix(), 10))
	}
	values.Set("query", queryText)
	endpoint.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build ClickHouse attributed metric request: %w", err)
	}
	request.SetBasicAuth(client.user, client.password)
	response, err := client.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("query ClickHouse attributed metrics: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 800))
		return nil, fmt.Errorf(
			"query ClickHouse attributed metrics: unexpected status %s: %s",
			response.Status,
			string(message),
		)
	}

	result := make([]AttributedMetricValue, 0)
	decoder := json.NewDecoder(response.Body)
	for {
		var row struct {
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
		if err := decoder.Decode(&row); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decode ClickHouse attributed metrics: %w", err)
		}
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

func (client Client) EnvironmentLogs(
	ctx context.Context,
	environment string,
	after *LogCursor,
	limit uint64,
) (EnvironmentLogPage, error) {
	const initialQuery = "SELECT toString(toUnixTimestamp64Nano(Timestamp)) AS timestamp_nanoseconds, Body AS message, LogAttributes['log.iostream'] AS stream, LogAttributes['container.name'] AS container, LogAttributes['deploycrate.deployment.id'] AS deployment, LogAttributes['deploycrate.instance.id'] AS instance, LogAttributes['deploycrate.release.id'] AS release, LogAttributes['deploycrate.log.epoch'] AS epoch, LogAttributes['deploycrate.log.ordinal'] AS ordinal FROM otel_logs WHERE LogAttributes['deploycrate.environment.id'] = {environment:String} ORDER BY Timestamp DESC, epoch DESC, toUInt64OrZero(LogAttributes['deploycrate.log.ordinal']) DESC LIMIT {limit:UInt64} FORMAT JSONEachRow"
	const incrementalQuery = "SELECT toString(toUnixTimestamp64Nano(Timestamp)) AS timestamp_nanoseconds, Body AS message, LogAttributes['log.iostream'] AS stream, LogAttributes['container.name'] AS container, LogAttributes['deploycrate.deployment.id'] AS deployment, LogAttributes['deploycrate.instance.id'] AS instance, LogAttributes['deploycrate.release.id'] AS release, LogAttributes['deploycrate.log.epoch'] AS epoch, LogAttributes['deploycrate.log.ordinal'] AS ordinal FROM otel_logs WHERE LogAttributes['deploycrate.environment.id'] = {environment:String} AND (Timestamp, LogAttributes['deploycrate.log.epoch'], toUInt64OrZero(LogAttributes['deploycrate.log.ordinal'])) > (fromUnixTimestamp64Nano({after_nanoseconds:Int64}), {after_epoch:String}, {after_ordinal:UInt64}) ORDER BY Timestamp, epoch, toUInt64OrZero(LogAttributes['deploycrate.log.ordinal']) LIMIT {limit:UInt64} FORMAT JSONEachRow"

	endpoint, err := url.Parse(client.baseURL)
	if err != nil {
		return EnvironmentLogPage{}, fmt.Errorf("build ClickHouse URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("database", client.database)
	query.Set("param_environment", environment)
	query.Set("param_limit", strconv.FormatUint(limit, 10))
	queryText := initialQuery
	if after != nil {
		queryText = incrementalQuery
		query.Set("param_after_nanoseconds", strconv.FormatInt(after.Timestamp.UnixNano(), 10))
		query.Set("param_after_epoch", after.Epoch)
		query.Set("param_after_ordinal", strconv.FormatUint(after.Ordinal, 10))
	}
	query.Set("query", queryText)
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return EnvironmentLogPage{}, fmt.Errorf("build ClickHouse Environment log request: %w", err)
	}
	request.SetBasicAuth(client.user, client.password)
	response, err := client.client.Do(request)
	if err != nil {
		return EnvironmentLogPage{}, fmt.Errorf("query ClickHouse Environment logs: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 800))
		return EnvironmentLogPage{}, fmt.Errorf(
			"query ClickHouse Environment logs: unexpected status %s: %s",
			response.Status,
			string(message),
		)
	}

	logs := make([]EnvironmentLog, 0, limit)
	decoder := json.NewDecoder(response.Body)
	for {
		var row struct {
			TimestampNanoseconds string `json:"timestamp_nanoseconds"`
			Message              string `json:"message"`
			Stream               string `json:"stream"`
			Container            string `json:"container"`
			Deployment           string `json:"deployment"`
			Instance             string `json:"instance"`
			Release              string `json:"release"`
			Epoch                string `json:"epoch"`
			Ordinal              string `json:"ordinal"`
		}
		if err := decoder.Decode(&row); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return EnvironmentLogPage{}, fmt.Errorf("decode ClickHouse Environment log: %w", err)
		}
		timestampNanoseconds, err := strconv.ParseInt(row.TimestampNanoseconds, 10, 64)
		if err != nil {
			return EnvironmentLogPage{}, fmt.Errorf("decode ClickHouse Environment log timestamp: %w", err)
		}
		ordinal, err := strconv.ParseUint(row.Ordinal, 10, 64)
		if err != nil {
			return EnvironmentLogPage{}, fmt.Errorf("decode ClickHouse Environment log ordinal: %w", err)
		}
		logs = append(logs, EnvironmentLog{
			Cursor: LogCursor{
				Timestamp: time.Unix(0, timestampNanoseconds).UTC(),
				Epoch:     row.Epoch,
				Ordinal:   ordinal,
			},
			Message: row.Message, Stream: row.Stream, Container: row.Container,
			Deployment: row.Deployment, Instance: row.Instance, Release: row.Release,
		})
	}
	if after == nil {
		slices.Reverse(logs)
	}
	return EnvironmentLogPage{Logs: logs}, nil
}

func (client Client) SystemLogs(
	ctx context.Context,
	service string,
	after *SystemLogCursor,
	limit uint64,
) (SystemLogPage, error) {
	const fingerprint = "sipHash64(SeverityText, Body, TraceId, SpanId, ScopeName, toString(LogAttributes), toString(ResourceAttributes))"
	const columns = "toString(toUnixTimestamp64Nano(Timestamp)) AS timestamp_nanoseconds, toString(" + fingerprint + ") AS fingerprint, Body AS message, SeverityText AS severity, SeverityNumber AS severity_number, TraceId AS trace_id, SpanId AS span_id, ScopeName AS scope, LogAttributes['code.file.path'] AS source, LogAttributes['code.line.number'] AS line, ResourceAttributes['service.instance.id'] AS instance, ResourceAttributes['deploycrate.slot'] AS slot"
	const initialQuery = "SELECT " + columns + " FROM otel_logs WHERE ServiceName = {service:String} ORDER BY Timestamp DESC, " + fingerprint + " DESC LIMIT {limit:UInt64} FORMAT JSONEachRow"
	const incrementalQuery = "SELECT " + columns + " FROM otel_logs WHERE ServiceName = {service:String} AND (Timestamp, " + fingerprint + ") > (fromUnixTimestamp64Nano({after_nanoseconds:Int64}), {after_fingerprint:UInt64}) ORDER BY Timestamp, " + fingerprint + " LIMIT {limit:UInt64} FORMAT JSONEachRow"

	endpoint, err := url.Parse(client.baseURL)
	if err != nil {
		return SystemLogPage{}, fmt.Errorf("build ClickHouse URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("database", client.database)
	query.Set("param_service", service)
	query.Set("param_limit", strconv.FormatUint(limit, 10))
	queryText := initialQuery
	if after != nil {
		queryText = incrementalQuery
		query.Set("param_after_nanoseconds", strconv.FormatInt(after.Timestamp.UnixNano(), 10))
		query.Set("param_after_fingerprint", strconv.FormatUint(after.Fingerprint, 10))
	}
	query.Set("query", queryText)
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return SystemLogPage{}, fmt.Errorf("build ClickHouse system log request: %w", err)
	}
	request.SetBasicAuth(client.user, client.password)
	response, err := client.client.Do(request)
	if err != nil {
		return SystemLogPage{}, fmt.Errorf("query ClickHouse system logs: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 800))
		return SystemLogPage{}, fmt.Errorf(
			"query ClickHouse system logs: unexpected status %s: %s",
			response.Status,
			string(message),
		)
	}

	logs := make([]SystemLog, 0, limit)
	decoder := json.NewDecoder(response.Body)
	for {
		var row struct {
			TimestampNanoseconds string `json:"timestamp_nanoseconds"`
			Fingerprint          string `json:"fingerprint"`
			Message              string `json:"message"`
			Severity             string `json:"severity"`
			SeverityNumber       uint8  `json:"severity_number"`
			TraceID              string `json:"trace_id"`
			SpanID               string `json:"span_id"`
			Scope                string `json:"scope"`
			Source               string `json:"source"`
			Line                 string `json:"line"`
			Instance             string `json:"instance"`
			Slot                 string `json:"slot"`
		}
		if err := decoder.Decode(&row); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return SystemLogPage{}, fmt.Errorf("decode ClickHouse system log: %w", err)
		}
		timestampNanoseconds, err := strconv.ParseInt(row.TimestampNanoseconds, 10, 64)
		if err != nil {
			return SystemLogPage{}, fmt.Errorf("decode ClickHouse system log timestamp: %w", err)
		}
		fingerprint, err := strconv.ParseUint(row.Fingerprint, 10, 64)
		if err != nil {
			return SystemLogPage{}, fmt.Errorf("decode ClickHouse system log fingerprint: %w", err)
		}
		logs = append(logs, SystemLog{
			Cursor: SystemLogCursor{
				Timestamp:   time.Unix(0, timestampNanoseconds).UTC(),
				Fingerprint: fingerprint,
			},
			Message: row.Message, Severity: row.Severity, SeverityNumber: row.SeverityNumber,
			TraceID: row.TraceID, SpanID: row.SpanID, Scope: row.Scope,
			Source: row.Source, Line: row.Line, Instance: row.Instance, Slot: row.Slot,
		})
	}
	if after == nil {
		slices.Reverse(logs)
	}
	return SystemLogPage{Logs: logs}, nil
}

func (client Client) ExportMetricRollups(
	ctx context.Context,
	destination io.Writer,
) (MetricRollupExport, error) {
	endpoint, err := url.Parse(client.baseURL)
	if err != nil {
		return MetricRollupExport{}, fmt.Errorf("build ClickHouse export URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("database", client.database)
	query.Set(
		"query",
		"SELECT bucket_start, observed_at, scope, component, metric, average, maximum, `last`, server, application, environment, release, deployment, target, instance, resource, installation, runtime_id, observation_id FROM metric_rollups ORDER BY bucket_start, scope, metric, server, environment, component, instance, installation, runtime_id, observation_id FORMAT JSONEachRow",
	)
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return MetricRollupExport{}, fmt.Errorf("build ClickHouse export request: %w", err)
	}
	request.SetBasicAuth(client.user, client.password)
	response, err := client.client.Do(request)
	if err != nil {
		return MetricRollupExport{}, fmt.Errorf("export ClickHouse metric rollups: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 800))
		return MetricRollupExport{}, fmt.Errorf(
			"export ClickHouse metric rollups: unexpected status %s: %s",
			response.Status,
			string(message),
		)
	}

	decoder := json.NewDecoder(io.TeeReader(response.Body, destination))
	result := MetricRollupExport{}
	for {
		var row struct {
			BucketStart string `json:"bucket_start"`
		}
		if err := decoder.Decode(&row); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return MetricRollupExport{}, fmt.Errorf("decode ClickHouse metric rollup export: %w", err)
		}
		if row.BucketStart == "" {
			return MetricRollupExport{}, errors.New("ClickHouse metric rollup export is missing bucket_start")
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
