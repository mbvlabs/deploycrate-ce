// Package caddy provides the adapter for Caddy's local configuration API.
package caddy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"deploycrate-ce/internal/wireguard"
	"deploycrate-ce/telemetry"
)

const (
	DefaultBaseURL           = "http://127.0.0.1:2019"
	routesPath               = "/config/apps/http/servers/srv0/routes"
	applyRouteMaxAttempts    = 3
	applyRouteRetryBaseDelay = 250 * time.Millisecond
	backendHealthInterval    = "2s"
	backendHealthTimeout     = "1s"
	backendTryDuration       = "2s"
	backendTryInterval       = "100ms"
	publicVerifyTimeout      = 2 * time.Minute
	publicVerifyInterval     = 2 * time.Second
	publicVerifyRequestLimit = 10 * time.Second
)

type Backend struct {
	Dial   string
	Weight int
}

type Route struct {
	ID                        string
	Domain                    string
	Backends                  []Backend
	HealthPath                string
	UpstreamTLS               bool
	DisableActiveHealthChecks bool
	Authentication            *BasicAuthentication
	PrivateNetworkOnly        bool
}

type BasicAuthentication struct {
	Username     string
	PasswordHash string
}

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    telemetry.NewHTTPClient(10 * time.Second),
	}
}

func (client *Client) ApplyRoute(ctx context.Context, route Route) error {
	if err := validateRoute(route); err != nil {
		return err
	}

	healthPath := strings.TrimSpace(route.HealthPath)
	if healthPath == "" {
		healthPath = "/api/health"
	}
	logs := []routeHandle{
		{Handler: "log_append", Key: "deploycrate_route", Value: route.ID},
		{Handler: "log_append", Key: "deploycrate_domain", Value: route.Domain},
	}
	inner := make([]routeHandle, 0, 2)
	if route.Authentication != nil {
		inner = append(inner, routeHandle{
			Handler: "authentication",
			Providers: map[string]authenticationProvider{
				"http_basic": {Accounts: []authenticationAccount{
					{
						Username: route.Authentication.Username,
						Password: route.Authentication.PasswordHash,
					},
				}},
			},
		})
	}
	proxy := routeHandle{
		Handler:   "reverse_proxy",
		Upstreams: upstreams(route.Backends),
		LoadBalancing: &loadBalancing{SelectionPolicy: selectionPolicy{
			Policy:  "weighted_round_robin",
			Weights: weights(route.Backends),
		}, TryDuration: backendTryDuration, TryInterval: backendTryInterval},
	}
	if !route.DisableActiveHealthChecks {
		proxy.HealthChecks = &healthChecks{Active: activeHealthChecks{
			URI: healthPath, Interval: backendHealthInterval, Timeout: backendHealthTimeout,
		}}
	}
	if route.UpstreamTLS {
		proxy.Transport = &httpTransport{Protocol: "http", TLS: &tlsTransport{}}
	}
	inner = append(inner, proxy)
	handles := logs
	if route.PrivateNetworkOnly {
		handles = append(handles, routeHandle{
			Handler: "subroute",
			Routes: []subrouteEntry{
				{
					Match: []routeMatch{{
						RemoteIP: &remoteIPMatch{
							Ranges: []string{wireguard.MeshCIDR, "127.0.0.1/32", "::1/128"},
						},
					}},
					Handle: inner,
				},
				{Handle: []routeHandle{{Handler: "static_response", StatusCode: 403}}},
			},
		})
	} else {
		handles = append(handles, inner...)
	}
	entry := routeEntry{
		ID:       route.ID,
		Match:    []routeMatch{{Host: []string{route.Domain}}},
		Terminal: true,
		Handle:   handles,
	}

	for attempt := 1; attempt <= applyRouteMaxAttempts; attempt++ {
		err := client.applyRoute(ctx, entry)
		if err == nil {
			return nil
		}
		if !isRetryableTransportError(ctx, err) || attempt == applyRouteMaxAttempts {
			return err
		}
		if err := waitForApplyRetry(ctx, attempt); err != nil {
			return fmt.Errorf("caddy: retry route %s: %w", route.ID, err)
		}
	}

	return fmt.Errorf("caddy: apply route %s: retry attempts exhausted", route.ID)
}

func (client *Client) applyRoute(ctx context.Context, entry routeEntry) error {
	serverExists, err := client.serverExists(ctx)
	if err != nil {
		return err
	}
	if !serverExists {
		return client.createServerWithRoute(ctx, entry)
	}
	if err := client.ensureServerObservability(ctx); err != nil {
		return err
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("caddy: encode route %s: %w", entry.ID, err)
	}

	exists, err := client.routeExists(ctx, entry.ID)
	if err != nil {
		return err
	}
	method := http.MethodPost
	path := routesPath
	if exists {
		method = http.MethodPatch
		path = "/id/" + url.PathEscape(entry.ID)
	}
	if err := client.request(
		ctx,
		method,
		path,
		payload,
		http.StatusOK,
		http.StatusCreated,
	); err != nil {
		return fmt.Errorf("caddy: apply route %s: %w", entry.ID, err)
	}
	return nil
}

func (client *Client) VerifyRoute(ctx context.Context, id string) error {
	exists, err := client.routeExists(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("caddy: route %s is not present", id)
	}
	return nil
}

func (client *Client) RouteConfig(ctx context.Context, id string) (json.RawMessage, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("caddy: route identifier is required")
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		client.baseURL+"/id/"+url.PathEscape(id),
		nil,
	)
	if err != nil {
		return nil, err
	}
	response, err := client.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("caddy: read route %s: %w", id, err)
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return nil, fmt.Errorf("caddy: read route %s response: %w", id, readErr)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"caddy: read route %s: unexpected status %d: %s",
			id,
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("caddy: route %s returned invalid JSON", id)
	}
	return json.RawMessage(body), nil
}

func (client *Client) DeleteRoute(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("caddy: route identifier is required")
	}
	exists, err := client.routeExists(ctx, id)
	if err != nil || !exists {
		return err
	}
	if err := client.request(
		ctx,
		http.MethodDelete,
		"/id/"+url.PathEscape(id),
		nil,
		http.StatusOK,
	); err != nil {
		return fmt.Errorf("caddy: delete route %s: %w", id, err)
	}
	return nil
}

func (client *Client) VerifyPublic(ctx context.Context, domain, healthPath string) error {
	domain = strings.TrimSpace(domain)
	healthPath = strings.TrimSpace(healthPath)
	if domain == "" {
		return errors.New("caddy: public verification domain is required")
	}
	if healthPath == "" {
		healthPath = "/"
	}
	if !strings.HasPrefix(healthPath, "/") {
		return errors.New("caddy: public verification health path is invalid")
	}
	verificationCtx, cancel := context.WithTimeout(ctx, publicVerifyTimeout)
	defer cancel()
	httpClient := telemetry.NewHTTPClient(publicVerifyRequestLimit)
	var lastErr error
	for {
		request, err := http.NewRequestWithContext(
			verificationCtx,
			http.MethodGet,
			"https://"+domain+healthPath,
			nil,
		)
		if err != nil {
			return err
		}
		response, err := httpClient.Do(request)
		if err != nil {
			lastErr = fmt.Errorf("caddy: verify public Environment route: %w", err)
		} else {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 32*1024))
			_ = response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 400 {
				return nil
			}
			lastErr = fmt.Errorf(
				"caddy: public Environment route returned status %d",
				response.StatusCode,
			)
			if response.StatusCode < 500 {
				return lastErr
			}
		}

		timer := time.NewTimer(publicVerifyInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-verificationCtx.Done():
			timer.Stop()
			return fmt.Errorf("caddy: public Environment route did not become ready: %w", lastErr)
		case <-timer.C:
		}
	}
}

func (client *Client) serverExists(ctx context.Context) (bool, error) {
	status, body, err := client.status(ctx, routesPath)
	if err != nil {
		return false, fmt.Errorf("caddy: inspect HTTP server: %w", err)
	}
	if status == http.StatusOK {
		return true, nil
	}
	if status != http.StatusNotFound && !isInvalidTraversal(status, body) {
		return false, fmt.Errorf(
			"caddy: inspect HTTP server: unexpected status %d: %s",
			status,
			strings.TrimSpace(string(body)),
		)
	}
	return false, nil
}

func (client *Client) createServerWithRoute(ctx context.Context, route routeEntry) error {
	payload, err := json.Marshal(map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"srv0": map[string]any{
						"listen": []string{":443"},
						"logs":   map[string]any{},
						"metrics": map[string]any{
							"per_host": true,
						},
						"routes": []routeEntry{route},
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("caddy: encode HTTP server with route %s: %w", route.ID, err)
	}
	if err := client.request(
		ctx,
		http.MethodPatch,
		"/config/",
		payload,
		http.StatusOK,
	); err != nil {
		return fmt.Errorf("caddy: create HTTP server with route %s: %w", route.ID, err)
	}
	return nil
}

func (client *Client) ensureServerObservability(ctx context.Context) error {
	for path, payload := range map[string][]byte{
		"/config/apps/http/servers/srv0/logs":    []byte(`{}`),
		"/config/apps/http/servers/srv0/metrics": []byte(`{"per_host":true}`),
	} {
		if err := client.request(ctx, http.MethodPost, path, payload, http.StatusOK); err != nil {
			return fmt.Errorf("caddy: enable server observability at %s: %w", path, err)
		}
	}
	return nil
}

func isRetryableTransportError(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}

	if _, ok := errors.AsType[*url.Error](err); ok {
		return true
	}

	var networkError net.Error
	return errors.As(err, &networkError)
}

func waitForApplyRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(attempt) * applyRouteRetryBaseDelay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (client *Client) routeExists(ctx context.Context, id string) (bool, error) {
	status, body, err := client.status(ctx, "/id/"+url.PathEscape(id))
	if err != nil {
		return false, fmt.Errorf("caddy: inspect route %s: %w", id, err)
	}
	switch status {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf(
			"caddy: inspect route %s: unexpected status %d: %s",
			id,
			status,
			strings.TrimSpace(string(body)),
		)
	}
}

func (client *Client) status(ctx context.Context, path string) (int, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+path, nil)
	if err != nil {
		return 0, nil, err
	}
	response, err := client.http.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 32*1024))
	if err != nil {
		return response.StatusCode, nil, err
	}
	return response.StatusCode, body, nil
}

func isInvalidTraversal(status int, body []byte) bool {
	if status != http.StatusBadRequest {
		return false
	}
	var response struct {
		Error string `json:"error"`
	}
	return json.Unmarshal(body, &response) == nil &&
		strings.HasPrefix(response.Error, "invalid traversal path at:")
}

func (client *Client) request(
	ctx context.Context,
	method, path string,
	payload []byte,
	allowed ...int,
) error {
	request, err := http.NewRequestWithContext(
		ctx,
		method,
		client.baseURL+path,
		bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if slices.Contains(allowed, response.StatusCode) {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 32*1024))
	return errors.Join(
		fmt.Errorf(
			"unexpected status %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(body)),
		),
		readErr,
	)
}

func validateRoute(route Route) error {
	if strings.TrimSpace(route.ID) == "" || strings.TrimSpace(route.Domain) == "" {
		return errors.New("caddy: route ID and domain are required")
	}
	if len(route.Backends) == 0 {
		return errors.New("caddy: at least one route backend is required")
	}
	for _, backend := range route.Backends {
		if strings.TrimSpace(backend.Dial) == "" {
			return errors.New("caddy: backend dial address is required")
		}
		if backend.Weight < 0 || backend.Weight > 100 {
			return fmt.Errorf("caddy: backend weight %d must be between 0 and 100", backend.Weight)
		}
	}
	if route.HealthPath != "" && !strings.HasPrefix(route.HealthPath, "/") {
		return errors.New("caddy: health path must be absolute")
	}
	if route.Authentication != nil &&
		(strings.TrimSpace(route.Authentication.Username) == "" || strings.TrimSpace(route.Authentication.PasswordHash) == "") {
		return errors.New("caddy: basic authentication username and password hash are required")
	}
	return nil
}

func upstreams(backends []Backend) []upstream {
	result := make([]upstream, 0, len(backends))
	for _, backend := range backends {
		result = append(result, upstream{Dial: backend.Dial})
	}
	return result
}

func weights(backends []Backend) []int {
	result := make([]int, 0, len(backends))
	for _, backend := range backends {
		result = append(result, backend.Weight)
	}
	return result
}

type routeEntry struct {
	ID       string        `json:"@id"`
	Handle   []routeHandle `json:"handle"`
	Match    []routeMatch  `json:"match"`
	Terminal bool          `json:"terminal"`
}

type routeHandle struct {
	Handler       string                            `json:"handler"`
	Key           string                            `json:"key,omitempty"`
	Value         string                            `json:"value,omitempty"`
	HealthChecks  *healthChecks                     `json:"health_checks,omitempty"`
	LoadBalancing *loadBalancing                    `json:"load_balancing,omitempty"`
	Upstreams     []upstream                        `json:"upstreams,omitempty"`
	Providers     map[string]authenticationProvider `json:"providers,omitempty"`
	Transport     *httpTransport                    `json:"transport,omitempty"`
	Routes        []subrouteEntry                   `json:"routes,omitempty"`
	StatusCode    int                               `json:"status_code,omitempty"`
}

type subrouteEntry struct {
	Match  []routeMatch  `json:"match,omitempty"`
	Handle []routeHandle `json:"handle"`
}

type httpTransport struct {
	Protocol string        `json:"protocol"`
	TLS      *tlsTransport `json:"tls,omitempty"`
}

type tlsTransport struct{}

type authenticationProvider struct {
	Accounts []authenticationAccount `json:"accounts"`
}

type authenticationAccount struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type routeMatch struct {
	Host     []string       `json:"host,omitempty"`
	RemoteIP *remoteIPMatch `json:"remote_ip,omitempty"`
}

type remoteIPMatch struct {
	Ranges []string `json:"ranges"`
}

type loadBalancing struct {
	SelectionPolicy selectionPolicy `json:"selection_policy"`
	TryDuration     string          `json:"try_duration"`
	TryInterval     string          `json:"try_interval"`
}

type healthChecks struct {
	Active activeHealthChecks `json:"active"`
}

type activeHealthChecks struct {
	URI      string `json:"uri"`
	Interval string `json:"interval"`
	Timeout  string `json:"timeout"`
}

type selectionPolicy struct {
	Policy  string `json:"policy"`
	Weights []int  `json:"weights"`
}

type upstream struct {
	Dial string `json:"dial"`
}
