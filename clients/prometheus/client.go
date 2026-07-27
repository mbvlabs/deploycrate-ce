package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Sample struct {
	Labels map[string]string
	Value  float64
}

type Client struct {
	baseURL string
	client  *http.Client
}

func New(baseURL string) Client {
	return Client{baseURL: baseURL, client: &http.Client{Timeout: 10 * time.Second}}
}

func (client Client) Query(ctx context.Context, expression string, at time.Time) ([]Sample, error) {
	endpoint, err := url.Parse(client.baseURL + "/api/v1/query")
	if err != nil {
		return nil, fmt.Errorf("build Prometheus query URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("query", expression)
	query.Set("time", at.UTC().Format(time.RFC3339Nano))
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build Prometheus query: %w", err)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("query Prometheus: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query Prometheus: unexpected status %s", response.Status)
	}
	var payload struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []json.RawMessage `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Prometheus response: %w", err)
	}
	if payload.Status != "success" || payload.Data.ResultType != "vector" {
		return nil, fmt.Errorf("query Prometheus: %s", payload.Error)
	}
	samples := make([]Sample, 0, len(payload.Data.Result))
	for _, result := range payload.Data.Result {
		if len(result.Value) != 2 {
			continue
		}
		var rawValue string
		if err := json.Unmarshal(result.Value[1], &rawValue); err != nil {
			return nil, fmt.Errorf("decode Prometheus sample: %w", err)
		}
		value, err := strconv.ParseFloat(rawValue, 64)
		if err != nil {
			return nil, fmt.Errorf("parse Prometheus sample: %w", err)
		}
		samples = append(samples, Sample{Labels: result.Metric, Value: value})
	}
	return samples, nil
}
