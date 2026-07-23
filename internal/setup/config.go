package setup

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultEtcDir   = "/etc/deploycrate-ce"
	DefaultStateDir = "/var/lib/deploycrate-ce"
)

type DatabaseConfig struct {
	External  bool   `json:"external"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Name      string `json:"name"`
	User      string `json:"user"`
	SSLMode   string `json:"ssl_mode"`
	TLSCAPath string `json:"tls_ca_path,omitempty"`
}

type S3Config struct {
	Enabled      bool   `json:"enabled"`
	Endpoint     string `json:"endpoint,omitempty"`
	Region       string `json:"region,omitempty"`
	Bucket       string `json:"bucket,omitempty"`
	UsePathStyle bool   `json:"use_path_style,omitempty"`
}

type Secrets struct {
	LinuxPassword        string `json:"linux_password"`
	SSHPrivateKey        string `json:"ssh_private_key,omitempty"`
	AdminPassword        string `json:"admin_password"`
	DatabasePassword     string `json:"database_password"`
	SessionKey           string `json:"session_key"`
	SessionEncryptionKey string `json:"session_encryption_key"`
	TokenSigningKey      string `json:"token_signing_key"`
	Pepper               string `json:"pepper"`
	S3AccessKeyID        string `json:"s3_access_key_id,omitempty"`
	S3SecretAccessKey    string `json:"s3_secret_access_key,omitempty"`
}

type Config struct {
	Version         string         `json:"version"`
	Domain          string         `json:"domain"`
	SSHPort         int            `json:"ssh_port"`
	Timezone        string         `json:"timezone"`
	LinuxUser       string         `json:"linux_user"`
	SSHPublicKey    string         `json:"ssh_public_key"`
	GeneratedSSHKey bool           `json:"generated_ssh_key"`
	AdminEmail      string         `json:"admin_email"`
	Database        DatabaseConfig `json:"database"`
	S3              S3Config       `json:"s3"`
	Secrets         Secrets        `json:"-"`
}

func NewConfig(version string) (Config, error) {
	databasePassword, err := randomHex(24)
	if err != nil {
		return Config{}, err
	}
	sessionKey, err := randomHex(64)
	if err != nil {
		return Config{}, err
	}
	sessionEncryptionKey, err := randomHex(32)
	if err != nil {
		return Config{}, err
	}
	tokenSigningKey, err := randomHex(32)
	if err != nil {
		return Config{}, err
	}
	pepper, err := randomHex(24)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Version:   version,
		SSHPort:   22,
		Timezone:  "UTC",
		LinuxUser: "deploycrate",
		Database: DatabaseConfig{
			Host:    "127.0.0.1",
			Port:    5432,
			Name:    "deploycrate_ce",
			User:    "deploycrate",
			SSLMode: "disable",
		},
		S3: S3Config{Region: "us-east-1", UsePathStyle: true},
		Secrets: Secrets{
			DatabasePassword:     databasePassword,
			SessionKey:           sessionKey,
			SessionEncryptionKey: sessionEncryptionKey,
			TokenSigningKey:      tokenSigningKey,
			Pepper:               pepper,
		},
	}, nil
}

func (c Config) Validate() error {
	var errs []error
	if c.LinuxUser != "deploycrate" {
		errs = append(errs, errors.New("Linux user must be deploycrate"))
	}
	if strings.TrimSpace(c.Domain) == "" || strings.ContainsAny(c.Domain, " /:") {
		errs = append(errs, errors.New("a valid domain without protocol is required"))
	}
	if c.SSHPort < 1 || c.SSHPort > 65535 {
		errs = append(errs, errors.New("SSH port must be between 1 and 65535"))
	}
	if strings.TrimSpace(c.SSHPublicKey) == "" {
		errs = append(errs, errors.New("SSH public key is required"))
	}
	if _, err := mail.ParseAddress(c.AdminEmail); err != nil {
		errs = append(errs, errors.New("a valid application admin email is required"))
	}
	if len(c.Secrets.LinuxPassword) < 12 {
		errs = append(errs, errors.New("Linux password must be at least 12 characters"))
	}
	if len(c.Secrets.AdminPassword) < 8 {
		errs = append(errs, errors.New("application admin password must be at least 8 characters"))
	}
	if c.Database.Host == "" || c.Database.Port < 1 || c.Database.Name == "" || c.Database.User == "" || c.Secrets.DatabasePassword == "" {
		errs = append(errs, errors.New("complete database details are required"))
	}
	if c.S3.Enabled && (c.S3.Region == "" || c.S3.Bucket == "" || c.Secrets.S3AccessKeyID == "" || c.Secrets.S3SecretAccessKey == "") {
		errs = append(errs, errors.New("complete S3-compatible storage details are required"))
	}

	return errors.Join(errs...)
}

func (c Config) DatabaseURL() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.Database.User, c.Secrets.DatabasePassword),
		Host:   net.JoinHostPort(c.Database.Host, strconv.Itoa(c.Database.Port)),
		Path:   c.Database.Name,
	}
	q := u.Query()
	q.Set("sslmode", c.Database.SSLMode)
	if c.Database.TLSCAPath != "" {
		q.Set("sslrootcert", c.Database.TLSCAPath)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (c Config) SecretValues() []string {
	return []string{
		c.Secrets.LinuxPassword,
		c.Secrets.SSHPrivateKey,
		c.Secrets.AdminPassword,
		c.Secrets.DatabasePassword,
		c.Secrets.SessionKey,
		c.Secrets.SessionEncryptionKey,
		c.Secrets.TokenSigningKey,
		c.Secrets.Pepper,
		c.Secrets.S3AccessKeyID,
		c.Secrets.S3SecretAccessKey,
	}
}

func ConfigPaths() (string, string, string) {
	etcDir := os.Getenv("DEPLOYCRATE_CE_ETC_DIR")
	if etcDir == "" {
		etcDir = DefaultEtcDir
	}
	stateDir := os.Getenv("DEPLOYCRATE_CE_STATE_DIR")
	if stateDir == "" {
		stateDir = DefaultStateDir
	}
	return filepath.Join(etcDir, "installer.json"), filepath.Join(etcDir, "installer-secrets.json"), stateDir
}

func randomHex(bytes int) (string, error) {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate random secret: %w", err)
	}
	return hex.EncodeToString(data), nil
}
