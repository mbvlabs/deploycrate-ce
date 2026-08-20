package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProbeWorkloadHealthReportsSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/health" {
			t.Errorf("health probe path = %q, want /health", request.URL.Path)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := &http.Client{Timeout: time.Second}
	healthy, result := probeWorkloadHealth(
		context.Background(),
		client,
		strings.TrimPrefix(server.URL, "http://"),
		"/health",
	)
	if !healthy {
		t.Fatalf("probeWorkloadHealth() healthy = false, result = %q", result)
	}
	if result != "HTTP probe returned 204 No Content" {
		t.Fatalf("probeWorkloadHealth() result = %q", result)
	}
}

func TestProbeWorkloadHealthReportsHTTPStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	healthy, result := probeWorkloadHealth(
		context.Background(),
		&http.Client{Timeout: time.Second},
		strings.TrimPrefix(server.URL, "http://"),
		"/health",
	)
	if healthy {
		t.Fatal("probeWorkloadHealth() healthy = true, want false")
	}
	if result != "HTTP probe returned 503 Service Unavailable" {
		t.Fatalf("probeWorkloadHealth() result = %q", result)
	}
}
