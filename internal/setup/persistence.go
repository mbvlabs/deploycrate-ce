package setup

import (
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
	ApplicationEnvPath = "/etc/deploycrate-ce/app.env"
	BackupEnvPath      = "/etc/deploycrate-ce/backup.env"
)

func ApplicationReleaseBinaryPath(version string) string {
	return filepath.Join("/opt/deploycrate-ce/releases", releaseDirectoryName(version), "deploycrate-ce")
}

func ApplicationSlotBinaryPath(slot string) string {
	return filepath.Join("/opt/deploycrate-ce/slots", slot, "deploycrate-ce")
}

func SaveConfig(cfg Config) error {
	configPath, secretPath, _ := ConfigPaths()
	publicData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode installer config: %w", err)
	}
	secretData, err := json.MarshalIndent(cfg.Secrets, "", "  ")
	if err != nil {
		return fmt.Errorf("encode installer secrets: %w", err)
	}
	if err := writeProtectedFile(configPath, publicData, 0o600); err != nil {
		return err
	}
	return writeProtectedFile(secretPath, secretData, 0o600)
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

func HasSavedConfig() bool {
	configPath, secretPath, _ := ConfigPaths()
	_, configErr := os.Stat(configPath)
	_, secretErr := os.Stat(secretPath)
	return configErr == nil && secretErr == nil
}

func RemoveTransientSecrets() error {
	_, secretPath, _ := ConfigPaths()
	if err := os.Remove(secretPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove installer secrets: %w", err)
	}
	return nil
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
		{"DEPLOYCRATE_CE_UPDATE_STATUS_PATH", "/var/lib/deploycrate-ce/runtime/self-update.json"},
		{"CORS_ALLOWED_ORIGINS", "https://" + cfg.Domain},
		{"CSRF_STRATEGY", "header_only"},
		{"CSRF_TRUSTED_ORIGINS", "https://" + cfg.Domain},
		{"PEPPER", cfg.Secrets.Pepper},
		{"PREVIOUS_PEPPERS", ""},
		{"AWS_REGION", "us-east-1"},
		{"AWS_SES_ACCESS_KEY_ID", ""},
		{"AWS_SES_SECRET_ACCESS_KEY", ""},
		{"AWS_SES_CONFIGURATION_SET", ""},
	}
	return writeProtectedFile(ApplicationEnvPath, []byte(formatEnvironment(values)), 0o600)
}

func WriteBackupEnvironment(cfg Config) error {
	if !cfg.S3.Enabled {
		return nil
	}
	values := [][2]string{
		{"S3_ENDPOINT", cfg.S3.Endpoint},
		{"S3_REGION", cfg.S3.Region},
		{"S3_BUCKET", cfg.S3.Bucket},
		{"S3_ACCESS_KEY_ID", cfg.Secrets.S3AccessKeyID},
		{"S3_SECRET_ACCESS_KEY", cfg.Secrets.S3SecretAccessKey},
		{"S3_USE_PATH_STYLE", strconv.FormatBool(cfg.S3.UsePathStyle)},
	}
	return writeProtectedFile(BackupEnvPath, []byte(formatEnvironment(values)), 0o600)
}

func InstallApplicationBinary(source string) error {
	return InstallApplicationReleaseBinary(source, "bootstrap")
}

func InstallApplicationReleaseBinary(source, version string) error {
	if source == "" {
		source = os.Getenv("DEPLOYCRATE_CE_RELEASE_BINARY")
	}
	if source == "" {
		source = "/usr/local/bin/deploycrate-ce"
	}
	return installApplicationBinary(source, version)
}

func installApplicationBinary(source, version string) error {
	target := ApplicationReleaseBinaryPath(version)
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open release binary %s: %w (GitHub/Cloudflare acquisition adapter is pending)", source, err)
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
		if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '.' || value == '-' || value == '_' {
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
