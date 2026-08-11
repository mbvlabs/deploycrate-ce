package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"deploycrate-ce/telemetry"
)

const (
	ipAPIBatchEndpoint = "http://ip-api.com/batch?fields=status,query,countryCode,country"
	ipAPIMaxBatchSize  = 100
	geoCacheLifetime   = 24 * time.Hour
	geoFailureLifetime = time.Minute
)

type GeoResult struct {
	CountryCode string
	CountryName string
}

type GeoResolver interface {
	Resolve(context.Context, []string) (map[string]GeoResult, error)
}

type cachedGeoResult struct {
	result    GeoResult
	expiresAt time.Time
}

type IPAPIGeoResolver struct {
	client *http.Client
	mu     sync.Mutex
	cache  map[string]cachedGeoResult
}

func NewIPAPIGeoResolver() GeoResolver {
	return &IPAPIGeoResolver{
		client: telemetry.NewHTTPClient(2 * time.Second),
		cache:  make(map[string]cachedGeoResult),
	}
}

func (resolver *IPAPIGeoResolver) Resolve(
	ctx context.Context,
	addresses []string,
) (map[string]GeoResult, error) {
	now := time.Now().UTC()
	results := make(map[string]GeoResult, len(addresses))
	missing := make([]string, 0, len(addresses))
	seen := make(map[string]struct{}, len(addresses))

	resolver.mu.Lock()
	for _, address := range addresses {
		if net.ParseIP(address) == nil {
			continue
		}
		if _, ok := seen[address]; ok {
			continue
		}
		seen[address] = struct{}{}
		cached, ok := resolver.cache[address]
		if ok && now.Before(cached.expiresAt) {
			results[address] = cached.result
			continue
		}
		missing = append(missing, address)
	}
	resolver.mu.Unlock()

	if len(missing) == 0 {
		return results, nil
	}
	if len(missing) > ipAPIMaxBatchSize {
		missing = missing[:ipAPIMaxBatchSize]
	}
	resolved, err := resolver.resolveBatch(ctx, missing)
	expiresAt := now.Add(geoCacheLifetime)
	if err != nil {
		expiresAt = now.Add(geoFailureLifetime)
	}

	resolver.mu.Lock()
	for _, address := range missing {
		result := resolved[address]
		resolver.cache[address] = cachedGeoResult{result: result, expiresAt: expiresAt}
		results[address] = result
	}
	resolver.mu.Unlock()
	return results, err
}

func (resolver *IPAPIGeoResolver) resolveBatch(
	ctx context.Context,
	addresses []string,
) (map[string]GeoResult, error) {
	body, err := json.Marshal(addresses)
	if err != nil {
		return nil, fmt.Errorf("encode IP geolocation request: %w", err)
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		ipAPIBatchEndpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := resolver.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IP geolocation returned status %d", response.StatusCode)
	}
	var payload []struct {
		Status      string `json:"status"`
		Query       string `json:"query"`
		CountryCode string `json:"countryCode"`
		CountryName string `json:"country"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode IP geolocation response: %w", err)
	}
	results := make(map[string]GeoResult, len(payload))
	for _, item := range payload {
		if item.Status != "success" || item.Query == "" || item.CountryCode == "" {
			continue
		}
		results[item.Query] = GeoResult{
			CountryCode: item.CountryCode,
			CountryName: item.CountryName,
		}
	}
	return results, nil
}
