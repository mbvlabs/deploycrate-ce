package clickhouse

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"deploycrate-ce/telemetry"
)

// Client is the transport-level interface to a ClickHouse HTTP endpoint.
// Database-specific statements and result decoding belong in database/clickhouse.
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

// Query starts a parameterized query and returns its response stream. The caller
// must close the returned body.
func (client Client) Query(
	ctx context.Context,
	statement string,
	parameters map[string]string,
) (io.ReadCloser, error) {
	return client.do(ctx, statement, parameters, nil, nil, "")
}

// Insert sends a streaming request body to a parameterized insert statement.
func (client Client) Insert(
	ctx context.Context,
	statement string,
	parameters map[string]string,
	settings map[string]string,
	body io.Reader,
	contentType string,
) error {
	responseBody, err := client.do(ctx, statement, parameters, settings, body, contentType)
	if err != nil {
		return err
	}
	return responseBody.Close()
}

func (client Client) do(
	ctx context.Context,
	statement string,
	parameters map[string]string,
	settings map[string]string,
	body io.Reader,
	contentType string,
) (io.ReadCloser, error) {
	endpoint, err := url.Parse(client.baseURL)
	if err != nil {
		return nil, fmt.Errorf("build ClickHouse URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("database", client.database)
	query.Set("query", statement)
	for key, value := range parameters {
		query.Set("param_"+key, value)
	}
	for key, value := range settings {
		query.Set(key, value)
	}
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build ClickHouse request: %w", err)
	}
	request.SetBasicAuth(client.user, client.password)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("execute ClickHouse request: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		message, _ := io.ReadAll(io.LimitReader(response.Body, 1200))
		return nil, fmt.Errorf(
			"execute ClickHouse request: unexpected status %s: %s",
			response.Status,
			string(message),
		)
	}
	return response.Body, nil
}
