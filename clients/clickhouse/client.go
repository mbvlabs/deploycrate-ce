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
	"strconv"
	"strings"
	"time"
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
		client: &http.Client{Timeout: 15 * time.Second},
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
	query.Set("query", "INSERT INTO metric_rollups_v2 FORMAT JSONEachRow")
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
		"SELECT metric, argMax(`last`, observed_at) AS value, toString(toUnixTimestamp64Milli(max(observed_at))) AS observed_at_milliseconds FROM metric_rollups_v2 WHERE scope = 'host' AND server = {server:String} AND metric IN ('cpu_cores_used', 'cpu_cores_total', 'memory_available_bytes', 'memory_total_bytes', 'root_filesystem_available_bytes', 'root_filesystem_size_bytes', 'disk_read_bytes_per_second', 'disk_write_bytes_per_second', 'network_receive_bytes_per_second', 'network_transmit_bytes_per_second', 'oom_events', 'tasks') GROUP BY metric FORMAT JSONEachRow",
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
		"SELECT toString(toUInt64(toUnixTimestamp(bucket_start)) * 1000) AS bucket_start_milliseconds, metric, argMax(`last`, observed_at) AS value FROM metric_rollups_v2 WHERE scope = 'host' AND server = {server:String} AND bucket_start >= toDateTime({since_seconds:UInt32}) AND metric IN ('cpu_cores_used', 'cpu_cores_total', 'memory_available_bytes', 'memory_total_bytes', 'root_filesystem_available_bytes', 'root_filesystem_size_bytes', 'disk_read_bytes_per_second', 'disk_write_bytes_per_second', 'network_receive_bytes_per_second', 'network_transmit_bytes_per_second', 'oom_events', 'tasks') GROUP BY bucket_start, metric ORDER BY bucket_start, metric FORMAT JSONEachRow",
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
	const query = "SELECT scope, component, application, environment, release, deployment, target, instance, resource, installation, argMax(runtime_id, observed_at) AS runtime_id, metric, argMax(`last`, observed_at) AS value, toString(toUnixTimestamp64Milli(max(observed_at))) AS observed_at_milliseconds FROM metric_rollups_v2 WHERE scope = {scope:String} AND server = {server:String} AND ({environment:String} = '' OR environment = {environment:String}) GROUP BY scope, component, application, environment, release, deployment, target, instance, resource, installation, metric ORDER BY component, application, environment, instance, installation, metric FORMAT JSONEachRow"
	return client.queryAttributedMetrics(ctx, query, scope, server, environment, time.Time{})
}

func (client Client) AttributedMetricHistory(
	ctx context.Context,
	scope string,
	server string,
	environment string,
	since time.Time,
) ([]AttributedMetricValue, error) {
	const query = "SELECT bucket_start, scope, component, application, environment, release, deployment, target, instance, resource, installation, argMax(runtime_id, observed_at) AS runtime_id, metric, argMax(`last`, observed_at) AS value, toString(toUnixTimestamp64Milli(max(observed_at))) AS observed_at_milliseconds FROM metric_rollups_v2 WHERE scope = {scope:String} AND server = {server:String} AND ({environment:String} = '' OR environment = {environment:String}) AND bucket_start >= toDateTime({since_seconds:UInt32}) GROUP BY bucket_start, scope, component, application, environment, release, deployment, target, instance, resource, installation, metric ORDER BY bucket_start, component, application, environment, instance, installation, metric FORMAT JSONEachRow"
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
		"SELECT bucket_start, observed_at, scope, component, metric, average, maximum, `last`, server, application, environment, release, deployment, target, instance, resource, installation, runtime_id, observation_id FROM metric_rollups_v2 ORDER BY bucket_start, scope, metric, server, environment, component, instance, installation, runtime_id, observation_id FORMAT JSONEachRow",
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
