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
	"time"
)

type MetricRollup struct {
	BucketStart   time.Time `json:"bucket_start"`
	ObservedAt    time.Time `json:"observed_at"`
	Metric        string    `json:"metric"`
	Average       float64   `json:"average"`
	Maximum       float64   `json:"maximum"`
	Last          float64   `json:"last"`
	Server        string    `json:"server"`
	Environment   string    `json:"environment"`
	Deployment    string    `json:"deployment"`
	Target        string    `json:"target"`
	Resource      string    `json:"resource"`
	ObservationID string    `json:"observation_id"`
}

type MetricRollupExport struct {
	Rows        int64  `json:"rows"`
	FirstBucket string `json:"first_bucket,omitempty"`
	LastBucket  string `json:"last_bucket,omitempty"`
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
		"SELECT bucket_start, observed_at, metric, average, maximum, `last`, server, environment, deployment, target, resource, observation_id FROM metric_rollups ORDER BY bucket_start, metric, server, target, resource, observation_id FORMAT JSONEachRow",
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
