package setup

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	ApplicationEnvPath      = "/etc/deploycrate-ce/app.env"
	DefaultDatabaseCAPath   = "/etc/ssl/certs/deploycrate-ce-postgresql-ca.crt"
	BootstrapCLIPath        = "/usr/local/bin/bootstrap"
	BootstrapAppPayloadPath = "/usr/local/bin/deploycrate-ce"
	maxDatabaseCAFileSize   = 1024 * 1024
	finalInstallerSetupStep = "initial-backups-v1"
)

type InstallationStatus string

const (
	InstallationFresh          InstallationStatus = "fresh"
	InstallationResumable      InstallationStatus = "resumable"
	InstallationCleanupPending InstallationStatus = "cleanup-pending"
	InstallationComplete       InstallationStatus = "complete"
	InstallationInconsistent   InstallationStatus = "inconsistent"
)

func ApplicationReleaseBinaryPath(version string) string {
	return filepath.Join(
		"/opt/deploycrate-ce/releases",
		releaseDirectoryName(version),
		"deploycrate-ce",
	)
}

func ApplicationSlotBinaryPath(slot string) string {
	return filepath.Join("/opt/deploycrate-ce/slots", slot, "deploycrate-ce")
}

func SaveConfig(cfg Config) (Config, error) {
	configPath, secretPath, _ := ConfigPaths()
	normalized, err := persistDatabaseCA(cfg)
	if err != nil {
		return Config{}, err
	}
	publicData, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return Config{}, fmt.Errorf("encode installer config: %w", err)
	}
	secretData, err := json.MarshalIndent(normalized.Secrets, "", "  ")
	if err != nil {
		return Config{}, fmt.Errorf("encode installer secrets: %w", err)
	}
	if err := writeProtectedFile(configPath, publicData, 0o600); err != nil {
		return Config{}, err
	}
	if err := writeProtectedFile(secretPath, secretData, 0o600); err != nil {
		removeErr := os.Remove(configPath)
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
		return Config{}, errors.Join(err, removeErr)
	}
	return normalized, nil
}

func LoadConfig() (Config, error) {
	configPath, secretPath, _ := ConfigPaths()
	publicData, err := os.ReadFile(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("read installer config: %w", err)
	}
	secretData, err := os.ReadFile(secretPath)
	if err != nil {
		return Config{}, fmt.Errorf("read installer secrets: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(publicData, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode installer config: %w", err)
	}
	if err := json.Unmarshal(secretData, &cfg.Secrets); err != nil {
		return Config{}, fmt.Errorf("decode installer secrets: %w", err)
	}
	return cfg, nil
}

func InspectInstallation() (InstallationStatus, error) {
	configPath, secretPath, _ := ConfigPaths()
	configExists, err := fileExists(configPath)
	if err != nil {
		return InstallationInconsistent, fmt.Errorf("inspect installer config: %w", err)
	}
	secretExists, err := fileExists(secretPath)
	if err != nil {
		return InstallationInconsistent, fmt.Errorf("inspect installer secrets: %w", err)
	}
	store := NewStateStore()
	stateExists, err := fileExists(store.Path())
	if err != nil {
		return InstallationInconsistent, fmt.Errorf("inspect installer state: %w", err)
	}

	if !configExists {
		if !secretExists && !stateExists {
			return InstallationFresh, nil
		}
		return InstallationInconsistent, nil
	}

	var state State
	if stateExists {
		state, err = store.Load("")
		if err != nil {
			return InstallationInconsistent, err
		}
	}
	if secretExists {
		if !state.CredentialsVerifiedAt.IsZero() {
			return InstallationCleanupPending, nil
		}
		return InstallationResumable, nil
	}
	if !state.CredentialsVerifiedAt.IsZero() || setupStepCompleted(state, finalInstallerSetupStep) {
		return InstallationComplete, nil
	}
	return InstallationInconsistent, nil
}

func RemoveTransientSecrets() error {
	_, secretPath, _ := ConfigPaths()
	if err := os.Remove(secretPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove installer secrets: %w", err)
	}
	return nil
}

func CompleteCredentialHandoff() error {
	if err := NewStateStore().MarkCredentialsVerified(); err != nil {
		return fmt.Errorf("record credential verification: %w", err)
	}
	if err := RemoveTransientSecrets(); err != nil {
		return err
	}
	for _, path := range []string{BootstrapCLIPath, BootstrapAppPayloadPath} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove temporary bootstrap binary %s: %w", path, err)
		}
	}
	return nil
}

func persistDatabaseCA(cfg Config) (Config, error) {
	source := strings.TrimSpace(cfg.Database.TLSCAPath)
	if source == "" {
		return cfg, nil
	}
	target := os.Getenv("DEPLOYCRATE_CE_DATABASE_CA_PATH")
	if target == "" {
		target = DefaultDatabaseCAPath
	}
	if filepath.Clean(source) == filepath.Clean(target) {
		return cfg, nil
	}

	info, err := os.Stat(source)
	if err != nil {
		return Config{}, fmt.Errorf("inspect database CA certificate: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Config{}, errors.New("database CA certificate must be a regular file")
	}
	if info.Size() == 0 {
		return Config{}, errors.New("database CA certificate is empty")
	}
	if info.Size() > maxDatabaseCAFileSize {
		return Config{}, fmt.Errorf(
			"database CA certificate exceeds %d bytes",
			maxDatabaseCAFileSize,
		)
	}

	file, err := os.Open(source)
	if err != nil {
		return Config{}, fmt.Errorf("open database CA certificate: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxDatabaseCAFileSize+1))
	closeErr := file.Close()
	if readErr != nil {
		return Config{}, fmt.Errorf("read database CA certificate: %w", readErr)
	}
	if closeErr != nil {
		return Config{}, fmt.Errorf("close database CA certificate: %w", closeErr)
	}
	if len(data) > maxDatabaseCAFileSize {
		return Config{}, fmt.Errorf(
			"database CA certificate exceeds %d bytes",
			maxDatabaseCAFileSize,
		)
	}
	if err := writeProtectedFile(target, data, 0o644); err != nil {
		return Config{}, fmt.Errorf("persist database CA certificate: %w", err)
	}
	cfg.Database.TLSCAPath = target
	return cfg, nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func setupStepCompleted(state State, id string) bool {
	step, ok := state.Steps[id]
	return ok && step.Status == StepCompleted
}

func WriteApplicationEnvironment(cfg Config) error {
	values := [][2]string{
		{"ENVIRONMENT", "production"},
		{"PROJECT_NAME", "deploycrate-ce"},
		{"DOMAIN", cfg.Domain},
		{"PROTOCOL", "https"},
		{"DEFAULT_SENDER_SIGNATURE", "noreply@" + cfg.Domain},
		{"HOST", "127.0.0.1"},
		{"PORT", "8080"},
		{"DB_KIND", "postgres"},
		{"DB_HOST", cfg.Database.Host},
		{"DB_PORT", strconv.Itoa(cfg.Database.Port)},
		{"DB_NAME", cfg.Database.Name},
		{"DB_USER", cfg.Database.User},
		{"DB_PASSWORD", cfg.Secrets.DatabasePassword},
		{"DB_SSL_MODE", cfg.Database.SSLMode},
		{"DB_SSL_ROOT_CERT", cfg.Database.TLSCAPath},
		{"SESSION_KEY", cfg.Secrets.SessionKey},
		{"SESSION_ENCRYPTION_KEY", cfg.Secrets.SessionEncryptionKey},
		{"SESSION_MAX_AGE", "604800"},
		{"TOKEN_SIGNING_KEY", cfg.Secrets.TokenSigningKey},
		{"SSH_CA_USER_PRINCIPAL", cfg.AdminUser},
		{"DEPLOYCRATE_CE_UPDATE_STATUS_PATH", "/var/lib/deploycrate-ce/runtime/self-update.json"},
		{"CORS_ALLOWED_ORIGINS", "https://" + cfg.Domain},
		{"CSRF_STRATEGY", "header_only"},
		{"CSRF_TRUSTED_ORIGINS", "https://" + cfg.Domain},
		{"PEPPER", cfg.Secrets.Pepper},
		{"PREVIOUS_PEPPERS", ""},
		{"METRICS_ROLLUP_ENABLED", "true"},
		{"PROMETHEUS_URL", "http://127.0.0.1:9090"},
		{"CLICKHOUSE_URL", "http://127.0.0.1:8123"},
		{"CLICKHOUSE_DATABASE", "deploycrate"},
		{"CLICKHOUSE_USER", "deploycrate"},
		{"CLICKHOUSE_PASSWORD", cfg.Secrets.ClickHousePassword},
		{"AWS_REGION", "us-east-1"},
		{"AWS_SES_ACCESS_KEY_ID", ""},
		{"AWS_SES_SECRET_ACCESS_KEY", ""},
		{"AWS_SES_CONFIGURATION_SET", ""},
	}
	return writeProtectedFile(ApplicationEnvPath, []byte(formatEnvironment(values)), 0o600)
}

func InstallApplicationBinary(source string) error {
	return InstallApplicationReleaseBinary(context.Background(), source, "bootstrap")
}

func InstallApplicationReleaseBinary(ctx context.Context, source, version string) error {
	if source == "" {
		source = os.Getenv("DEPLOYCRATE_CE_RELEASE_BINARY")
	}
	if source == "" {
		acquired, cleanup, err := acquireDevelopmentApplicationBinary(ctx)
		if err != nil {
			return err
		}
		defer cleanup()
		source = acquired
	}
	return installApplicationBinary(source, version)
}

func installApplicationBinary(source, version string) error {
	target := ApplicationReleaseBinaryPath(version)
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open release binary %s: %w", source, err)
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create application binary directory: %w", err)
	}
	temporary := target + ".tmp"
	output, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("create application binary: %w", err)
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("copy application binary: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close application binary: %w", closeErr)
	}
	if err := os.Chmod(temporary, 0o755); err != nil {
		return fmt.Errorf("mark application binary executable: %w", err)
	}
	if err := os.Rename(temporary, target); err != nil {
		return fmt.Errorf("install application binary: %w", err)
	}
	if err := linkSlotBinary("blue", target); err != nil {
		return err
	}
	return nil
}

func ApplicationArtifactDigest(version string) ([]byte, error) {
	file, err := os.Open(ApplicationReleaseBinaryPath(version))
	if err != nil {
		return nil, fmt.Errorf("open application release for digest: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, fmt.Errorf("digest application release: %w", err)
	}
	return hash.Sum(nil), nil
}

func linkSlotBinary(slot, target string) error {
	path := ApplicationSlotBinaryPath(slot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create application slot directory: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.Remove(temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale application slot link: %w", err)
	}
	if err := os.Symlink(target, temporary); err != nil {
		return fmt.Errorf("create application slot link: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("activate application slot link: %w", err)
	}
	return nil
}

func releaseDirectoryName(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "bootstrap"
	}
	var builder strings.Builder
	for _, value := range version {
		if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
			value >= '0' && value <= '9' ||
			value == '.' ||
			value == '-' ||
			value == '_' {
			builder.WriteRune(value)
		} else {
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "bootstrap"
	}
	return builder.String()
}

func formatEnvironment(values [][2]string) string {
	var builder strings.Builder
	for _, value := range values {
		builder.WriteString(value[0])
		builder.WriteByte('=')
		builder.WriteString(strconv.Quote(value[1]))
		builder.WriteByte('\n')
	}
	return builder.String()
}

func writeProtectedFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, mode); err != nil {
		return fmt.Errorf("write %s: %w", temporary, err)
	}
	if err := os.Chmod(temporary, mode); err != nil {
		return fmt.Errorf("protect %s: %w", temporary, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("install %s: %w", path, err)
	}
	return nil
}
