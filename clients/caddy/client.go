// Package caddy provides the adapter for Caddy's local configuration API.
package caddy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "http://127.0.0.1:2019"
	routesPath     = "/config/apps/http/servers/srv0/routes"
)

type Backend struct {
	Dial   string
	Weight int
}

type Route struct {
	ID       string
	Domain   string
	Backends []Backend
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
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (client *Client) ApplyRoute(ctx context.Context, route Route) error {
	if err := validateRoute(route); err != nil {
		return err
	}
	if err := client.ensureServer(ctx); err != nil {
		return err
	}

	payload, err := json.Marshal(routeEntry{
		ID:       route.ID,
		Match:    []routeMatch{{Host: []string{route.Domain}}},
		Terminal: true,
		Handle: []routeHandle{{
			Handler:   "reverse_proxy",
			Upstreams: upstreams(route.Backends),
			LoadBalancing: loadBalancing{SelectionPolicy: selectionPolicy{
				Policy:  "weighted_round_robin",
				Weights: weights(route.Backends),
			}},
		}},
	})
	if err != nil {
		return fmt.Errorf("caddy: encode route %s: %w", route.ID, err)
	}

	exists, err := client.routeExists(ctx, route.ID)
	if err != nil {
		return err
	}
	method := http.MethodPost
	path := routesPath
	if exists {
		method = http.MethodPatch
		path = "/id/" + url.PathEscape(route.ID)
	}
	if err := client.request(ctx, method, path, payload, http.StatusOK, http.StatusCreated); err != nil {
		return fmt.Errorf("caddy: apply route %s: %w", route.ID, err)
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

func (client *Client) ensureServer(ctx context.Context) error {
	status, err := client.status(ctx, routesPath)
	if err != nil {
		return fmt.Errorf("caddy: inspect HTTP server: %w", err)
	}
	if status == http.StatusOK {
		return nil
	}
	if status != http.StatusNotFound {
		return fmt.Errorf("caddy: inspect HTTP server: unexpected status %d", status)
	}

	payload, err := json.Marshal(map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"srv0": map[string]any{
						"listen": []string{":443"},
						"routes": []any{},
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("caddy: encode HTTP server: %w", err)
	}
	if err := client.request(ctx, http.MethodPatch, "/config/", payload, http.StatusOK); err != nil {
		return fmt.Errorf("caddy: create HTTP server: %w", err)
	}
	return nil
}

func (client *Client) routeExists(ctx context.Context, id string) (bool, error) {
	status, err := client.status(ctx, "/id/"+url.PathEscape(id))
	if err != nil {
		return false, fmt.Errorf("caddy: inspect route %s: %w", id, err)
	}
	switch status {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("caddy: inspect route %s: unexpected status %d", id, status)
	}
}

func (client *Client) status(ctx context.Context, path string) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+path, nil)
	if err != nil {
		return 0, err
	}
	response, err := client.http.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode, nil
}

func (client *Client) request(ctx context.Context, method, path string, payload []byte, allowed ...int) error {
	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	for _, status := range allowed {
		if response.StatusCode == status {
			_, _ = io.Copy(io.Discard, response.Body)
			return nil
		}
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 32*1024))
	return errors.Join(
		fmt.Errorf("unexpected status %d: %s", response.StatusCode, strings.TrimSpace(string(body))),
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
	Handler       string        `json:"handler"`
	LoadBalancing loadBalancing `json:"load_balancing"`
	Upstreams     []upstream    `json:"upstreams"`
}

type routeMatch struct {
	Host []string `json:"host"`
}

type loadBalancing struct {
	SelectionPolicy selectionPolicy `json:"selection_policy"`
}

type selectionPolicy struct {
	Policy  string `json:"policy"`
	Weights []int  `json:"weights"`
}

type upstream struct {
	Dial string `json:"dial"`
}
