package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/telemetry"
)

const (
	systemWireGuardInterface = "wg0"
	systemWireGuardAddress   = "10.99.0.1"
)

type SystemHealthCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

type SystemHealthReport struct {
	OK        bool                `json:"ok"`
	CheckedAt time.Time           `json:"checkedAt"`
	Checks    []SystemHealthCheck `json:"checks"`
}

type SystemHealth struct {
	configuration config.Config
	db            storage.Pool
	clickhouse    *ClickHouseResource
}

type systemHealthChecker func(context.Context) (string, error)

func NewSystemHealth(
	configuration config.Config,
	db storage.Pool,
	clickhouse *ClickHouseResource,
) *SystemHealth {
	return &SystemHealth{configuration: configuration, db: db, clickhouse: clickhouse}
}

func (s *SystemHealth) Run(ctx context.Context) SystemHealthReport {
	checks := []struct {
		name string
		run  systemHealthChecker
	}{
		{name: "services", run: checkSystemServices},
		{name: "listeners", run: checkSystemListeners},
		{name: "wireguard", run: checkSystemWireGuard},
		{name: "node_exporter", run: func(ctx context.Context) (string, error) {
			return getSystemHealth(ctx, "http://10.99.0.1:9100/metrics", "", "")
		}},
		{name: "cadvisor", run: func(ctx context.Context) (string, error) {
			return getSystemHealth(ctx, "http://127.0.0.1:9101/healthz", "", "")
		}},
		{name: "caddy_metrics", run: func(ctx context.Context) (string, error) {
			return getSystemHealth(ctx, "http://127.0.0.1:2019/metrics", "", "")
		}},
		{name: "otel_collector", run: func(ctx context.Context) (string, error) {
			return getSystemHealth(ctx, "http://127.0.0.1:13133/", "", "")
		}},
		{name: "clickhouse_metrics", run: func(ctx context.Context) (string, error) {
			return getSystemHealth(ctx, "http://127.0.0.1:9363/metrics", "", "")
		}},
		{name: "prometheus_targets", run: s.checkPrometheusTargets},
		{name: "clickhouse", run: s.checkClickHouse},
		{name: "disk_headroom", run: checkSystemDisk},
		{name: "active_slot_agreement", run: s.checkActiveSlotAgreement},
	}

	report := SystemHealthReport{
		OK:        true,
		CheckedAt: time.Now().UTC(),
		Checks:    make([]SystemHealthCheck, 0, len(checks)),
	}
	for _, candidate := range checks {
		detail, err := candidate.run(ctx)
		check := SystemHealthCheck{Name: candidate.name, OK: err == nil, Detail: detail}
		if err != nil {
			check.Detail = err.Error()
			report.OK = false
		}
		report.Checks = append(report.Checks, check)
	}
	return report
}

func checkSystemServices(ctx context.Context) (string, error) {
	units := []string{
		"wg-quick@wg0.service",
		"node-exporter.service",
		"docker.service",
		"caddy.service",
		"otelcol-contrib.service",
		"prometheus.service",
		"cadvisor.service",
	}
	for _, unit := range units {
		if _, err := systemHealthCommand(
			ctx,
			"sudo",
			"-n",
			"systemctl",
			"is-active",
			"--quiet",
			unit,
		); err != nil {
			return "", fmt.Errorf("%s is not active: %w", unit, err)
		}
	}
	return "WireGuard, node-exporter, cAdvisor, Docker, Caddy, OpenTelemetry Collector, and Prometheus are active", nil
}

func checkSystemListeners(ctx context.Context) (string, error) {
	output, err := systemHealthCommand(ctx, "ss", "-lnt")
	if err != nil {
		return "", err
	}
	for _, listener := range []string{
		"10.99.0.1:9100",
		"10.99.0.1:4318",
		"127.0.0.1:2019",
		"127.0.0.1:4318",
		"127.0.0.1:8123",
		"127.0.0.1:8888",
		"127.0.0.1:9000",
		"127.0.0.1:9090",
		"127.0.0.1:9101",
		"127.0.0.1:9363",
		"127.0.0.1:13133",
	} {
		if !strings.Contains(output, listener) {
			return "", fmt.Errorf("required listener %s is absent", listener)
		}
	}
	if strings.Contains(output, "0.0.0.0:9100") || strings.Contains(output, "*:9100") {
		return "", errors.New("node-exporter has a public listener")
	}
	for _, listener := range []struct {
		name string
		port string
	}{
		{name: "Caddy admin API", port: "2019"},
		{name: "OpenTelemetry OTLP receiver", port: "4318"},
		{name: "ClickHouse HTTP", port: "8123"},
		{name: "OpenTelemetry metrics", port: "8888"},
		{name: "ClickHouse native protocol", port: "9000"},
		{name: "Prometheus", port: "9090"},
		{name: "cAdvisor", port: "9101"},
		{name: "ClickHouse metrics", port: "9363"},
		{name: "OpenTelemetry health check", port: "13133"},
	} {
		if strings.Contains(output, "0.0.0.0:"+listener.port) ||
			strings.Contains(output, "*:"+listener.port) ||
			strings.Contains(output, "[::]:"+listener.port) {
			return "", fmt.Errorf("%s has a public listener", listener.name)
		}
	}
	return "node-exporter and the managed-node OTLP receiver are on WireGuard; control-plane listeners are local", nil
}

func checkSystemWireGuard(ctx context.Context) (string, error) {
	address, err := systemHealthCommand(
		ctx,
		"ip",
		"-4",
		"address",
		"show",
		"dev",
		systemWireGuardInterface,
	)
	if err != nil {
		return "", err
	}
	if !strings.Contains(address, systemWireGuardAddress+"/16") {
		return "", errors.New("wg0 does not own 10.99.0.1/16")
	}
	if _, err := systemHealthCommand(
		ctx,
		"sudo",
		"-n",
		"wg",
		"show",
		systemWireGuardInterface,
	); err != nil {
		return "", err
	}
	if _, err := systemHealthCommand(
		ctx,
		"sudo",
		"-n",
		"test",
		"-f",
		"/etc/wireguard/wg0.conf",
	); err != nil {
		return "", fmt.Errorf("inspect durable WireGuard configuration: %w", err)
	}
	return "wg0 owns 10.99.0.1/16 and has durable state", nil
}

func (s *SystemHealth) checkPrometheusTargets(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		strings.TrimRight(s.configuration.Metrics.PrometheusURL, "/")+"/api/v1/targets",
		nil,
	)
	if err != nil {
		return "", err
	}
	response, err := telemetry.NewHTTPClient(5 * time.Second).Do(request)
	if err != nil {
		return "", fmt.Errorf("query Prometheus targets: %w", err)
	}
	defer response.Body.Close()
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			ActiveTargets []struct {
				Health string            `json:"health"`
				Labels map[string]string `json:"labels"`
			} `json:"activeTargets"`
		} `json:"data"`
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Prometheus target API returned %s", response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode Prometheus target response: %w", err)
	}
	if payload.Status != "success" {
		return "", errors.New("Prometheus target API is unhealthy")
	}
	if len(payload.Data.ActiveTargets) == 0 {
		return "", errors.New("Prometheus has no active targets")
	}
	for _, target := range payload.Data.ActiveTargets {
		if target.Health != "up" {
			return "", fmt.Errorf(
				"Prometheus target %s is %s",
				target.Labels["instance"],
				target.Health,
			)
		}
	}
	return fmt.Sprintf("%d Prometheus targets are up", len(payload.Data.ActiveTargets)), nil
}

func (s *SystemHealth) checkClickHouse(ctx context.Context) (string, error) {
	client, err := s.clickhouse.Client(ctx)
	if err != nil {
		return "", err
	}
	detail, err := client.Ping(ctx)
	if err != nil {
		return "", err
	}
	return "ClickHouse resource responded " + detail, nil
}

func checkSystemDisk(context.Context) (string, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return "", fmt.Errorf("inspect root filesystem: %w", err)
	}
	freeMB := stat.Bavail * uint64(stat.Bsize) / 1024 / 1024
	if freeMB < 10240 {
		return "", fmt.Errorf("root filesystem has %d MB free, 10240 MB required", freeMB)
	}
	return fmt.Sprintf("root filesystem has %d MB free", freeMB), nil
}

func (s *SystemHealth) checkActiveSlotAgreement(ctx context.Context) (string, error) {
	active := ""
	for _, slot := range []string{"blue", "green"} {
		isActive, err := serviceActive(ctx, "deploycrate-ce@"+slot+".service")
		if err != nil {
			return "", err
		}
		if !isActive {
			continue
		}
		if active != "" {
			return "", errors.New("both application slots are active")
		}
		active = slot
	}
	if active == "" {
		return "", errors.New("no application slot is active")
	}

	state, err := models.Application.FindSystemState(ctx, s.db.Executor())
	if err != nil {
		return "", fmt.Errorf("read persisted active slot: %w", err)
	}
	if state.ActiveInstanceSlot != active {
		return "", fmt.Errorf(
			"systemd slot %s disagrees with persisted slot %s",
			active,
			state.ActiveInstanceSlot,
		)
	}
	return "systemd and PostgreSQL agree on the active " + active + " slot", nil
}

func getSystemHealth(ctx context.Context, endpoint, username, password string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	if username != "" || password != "" {
		request.SetBasicAuth(username, password)
	}
	response, err := telemetry.NewHTTPClient(5 * time.Second).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s returned %s", endpoint, response.Status)
	}
	return endpoint + " is healthy", nil
}

func systemHealthCommand(
	ctx context.Context,
	name string,
	arguments ...string,
) (string, error) {
	output, err := exec.CommandContext(ctx, name, arguments...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf(
			"%s %s failed: %w: %s",
			name,
			strings.Join(arguments, " "),
			err,
			strings.TrimSpace(string(output)),
		)
	}
	return strings.TrimSpace(string(output)), nil
}
