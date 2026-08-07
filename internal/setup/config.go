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
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
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
	Enabled        bool               `json:"enabled"`
	Provider       string             `json:"provider,omitempty"`
	Endpoint       string             `json:"endpoint,omitempty"`
	Region         string             `json:"region,omitempty"`
	Bucket         string             `json:"bucket,omitempty"`
	Prefix         string             `json:"prefix,omitempty"`
	UsePathStyle   bool               `json:"use_path_style,omitempty"`
	ServerPolicy   BackupPolicyConfig `json:"server_policy"`
	DatabasePolicy BackupPolicyConfig `json:"database_policy"`
	ValidatedAt    time.Time          `json:"validated_at"`
}

type BackupPolicyConfig struct {
	Schedule  string          `json:"schedule"`
	Retention BackupRetention `json:"retention"`
}

type BackupRetention struct {
	KeepLast    int `json:"keep_last,omitempty"`
	KeepDaily   int `json:"keep_daily"`
	KeepWeekly  int `json:"keep_weekly"`
	KeepMonthly int `json:"keep_monthly"`
}

type Secrets struct {
	ServerAdminPassword     string `json:"server_admin_password"`
	SSHPrivateKey           string `json:"ssh_private_key,omitempty"`
	SSHCARecoveryPassphrase string `json:"ssh_ca_recovery_passphrase"`
	ClickHousePassword      string `json:"clickhouse_password"`
	AdminPassword           string `json:"admin_password"`
	DatabasePassword        string `json:"database_password"`
	SessionKey              string `json:"session_key"`
	SessionEncryptionKey    string `json:"session_encryption_key"`
	TokenSigningKey         string `json:"token_signing_key"`
	Pepper                  string `json:"pepper"`
	S3AccessKeyID           string `json:"s3_access_key_id,omitempty"`
	S3SecretAccessKey       string `json:"s3_secret_access_key,omitempty"`
	ResticPassword          string `json:"restic_password,omitempty"`
	AgeIdentity             string `json:"age_identity,omitempty"`
}

type Config struct {
	InstanceID        string         `json:"instance_id"`
	Version           string         `json:"version"`
	Domain            string         `json:"domain"`
	PublicIPv4        string         `json:"public_ipv4"`
	SSHPort           int            `json:"ssh_port"`
	Timezone          string         `json:"timezone"`
	AdminUser         string         `json:"admin_user"`
	ServiceUser       string         `json:"service_user"`
	SSHPublicKey      string         `json:"ssh_public_key"`
	OwnerSSHPublicKey string         `json:"owner_ssh_public_key,omitempty"`
	GeneratedSSHKey   bool           `json:"generated_ssh_key"`
	AdminEmail        string         `json:"admin_email"`
	Database          DatabaseConfig `json:"database"`
	S3                S3Config       `json:"s3"`
	Secrets           Secrets        `json:"-"`
}

func (c Config) DatabaseInstallationID() uuid.UUID {
	return uuid.NewSHA1(
		uuid.NameSpaceOID,
		[]byte("deploycrate-ce:database-installation:"+c.InstanceID),
	)
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
	recoveryPassphrase, err := randomHex(32)
	if err != nil {
		return Config{}, err
	}
	clickHousePassword, err := randomHex(24)
	if err != nil {
		return Config{}, err
	}
	pepper, err := randomHex(24)
	if err != nil {
		return Config{}, err
	}
	resticPassword, err := randomHex(32)
	if err != nil {
		return Config{}, err
	}
	ageIdentity, err := age.GenerateX25519Identity()
	if err != nil {
		return Config{}, fmt.Errorf("generate backup recovery identity: %w", err)
	}

	return Config{
		InstanceID:  uuid.NewString(),
		Version:     version,
		SSHPort:     22,
		Timezone:    "UTC",
		AdminUser:   "admin",
		ServiceUser: "deploycrate",
		Database: DatabaseConfig{
			Host:    "127.0.0.1",
			Port:    5432,
			Name:    "deploycrate_ce",
			User:    "deploycrate",
			SSLMode: "disable",
		},
		S3: S3Config{
			Provider: "s3", Region: "us-east-1", UsePathStyle: true,
			ServerPolicy: BackupPolicyConfig{
				Schedule:  "0 2 * * *",
				Retention: BackupRetention{KeepDaily: 7, KeepWeekly: 4, KeepMonthly: 6},
			},
			DatabasePolicy: BackupPolicyConfig{
				Schedule: "0 */6 * * *",
				Retention: BackupRetention{
					KeepLast:    12,
					KeepDaily:   7,
					KeepWeekly:  4,
					KeepMonthly: 6,
				},
			},
		},
		Secrets: Secrets{
			SSHCARecoveryPassphrase: recoveryPassphrase,
			ClickHousePassword:      clickHousePassword,
			DatabasePassword:        databasePassword,
			SessionKey:              sessionKey,
			SessionEncryptionKey:    sessionEncryptionKey,
			TokenSigningKey:         tokenSigningKey,
			Pepper:                  pepper,
			ResticPassword:          resticPassword,
			AgeIdentity:             ageIdentity.String(),
		},
	}, nil
}

func (c Config) Validate(normalizeObjectStorage func(S3Config) (S3Config, error)) error {
	var errs []error
	if _, err := uuid.Parse(c.InstanceID); err != nil {
		errs = append(errs, errors.New("instance ID is invalid"))
	}
	if c.AdminUser != "admin" {
		errs = append(errs, errors.New("server administrator user must be admin"))
	}
	if c.ServiceUser != "deploycrate" {
		errs = append(errs, errors.New("service user must be deploycrate"))
	}
	if c.AdminUser == c.ServiceUser {
		errs = append(errs, errors.New("server administrator and service users must be different"))
	}
	if !validHostname(c.Domain) {
		errs = append(errs, errors.New("a valid domain without protocol is required"))
	} else if !validHostname("registry-" + c.Domain) {
		errs = append(
			errs,
			errors.New("domain is too long to create the managed registry hostname"),
		)
	}
	publicIPv4 := net.ParseIP(c.PublicIPv4)
	if publicIPv4 == nil || publicIPv4.To4() == nil || publicIPv4.IsPrivate() ||
		publicIPv4.IsLoopback() {
		errs = append(errs, errors.New("a valid public IPv4 address is required"))
	}
	for name, value := range map[string]string{
		"domain": c.Domain, "administrator user": c.AdminUser, "service user": c.ServiceUser,
		"SSH public key":       c.SSHPublicKey,
		"owner SSH public key": c.OwnerSSHPublicKey,
		"admin email":          c.AdminEmail, "database host": c.Database.Host, "database name": c.Database.Name,
		"database user": c.Database.User,
	} {
		if strings.ContainsAny(value, "\r\n\x00") {
			errs = append(errs, fmt.Errorf("%s contains unsupported control characters", name))
		}
	}
	if c.SSHPort < 1 || c.SSHPort > 65535 {
		errs = append(errs, errors.New("SSH port must be between 1 and 65535"))
	}
	if strings.TrimSpace(c.SSHPublicKey) == "" {
		errs = append(errs, errors.New("SSH public key is required"))
	} else if err := ValidateSSHPublicKey(c.SSHPublicKey); err != nil {
		errs = append(errs, errors.New("generated SSH public key is invalid"))
	}
	if strings.TrimSpace(c.OwnerSSHPublicKey) != "" {
		if err := ValidateSSHPublicKey(c.OwnerSSHPublicKey); err != nil {
			errs = append(errs, errors.New("owner SSH public key is invalid"))
		}
	}
	if _, err := mail.ParseAddress(c.AdminEmail); err != nil {
		errs = append(errs, errors.New("a valid application admin email is required"))
	}
	if c.Secrets.ServerAdminPassword == "" {
		errs = append(errs, errors.New("server administrator password is required"))
	}
	if len(c.Secrets.AdminPassword) < 8 {
		errs = append(errs, errors.New("application admin password must be at least 8 characters"))
	}
	if len(c.Secrets.SSHCARecoveryPassphrase) < 32 {
		errs = append(errs, errors.New("SSH CA recovery passphrase is unavailable"))
	}
	if len(c.Secrets.ClickHousePassword) < 24 {
		errs = append(errs, errors.New("ClickHouse password is unavailable"))
	}
	if strings.TrimSpace(c.Secrets.TokenSigningKey) == "" {
		errs = append(errs, errors.New("token signing key is unavailable"))
	}
	if c.Database.Host == "" || c.Database.Port < 1 || c.Database.Name == "" ||
		c.Database.User == "" ||
		c.Secrets.DatabasePassword == "" {
		errs = append(errs, errors.New("complete database details are required"))
	}
	if c.S3.Enabled &&
		(c.S3.Provider == "" || c.S3.Region == "" || c.S3.Bucket == "" ||
			c.Secrets.S3AccessKeyID == "" || c.Secrets.S3SecretAccessKey == "" ||
			c.Secrets.ResticPassword == "" || c.Secrets.AgeIdentity == "" ||
			c.S3.ServerPolicy.Schedule == "" ||
			(!c.Database.External && c.S3.DatabasePolicy.Schedule == "")) {
		errs = append(errs, errors.New("complete S3-compatible storage details are required"))
	}
	if c.S3.Enabled {
		if normalizeObjectStorage == nil {
			errs = append(errs, errors.New("object storage normalization is unavailable"))
		} else if _, err := normalizeObjectStorage(c.S3); err != nil {
			errs = append(errs, err)
		}
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(c.S3.ServerPolicy.Schedule); err != nil {
			errs = append(
				errs,
				errors.New("server backup schedule must be a five-field cron expression"),
			)
		}
		if !c.Database.External {
			if _, err := parser.Parse(c.S3.DatabasePolicy.Schedule); err != nil {
				errs = append(
					errs,
					errors.New("database backup schedule must be a five-field cron expression"),
				)
			}
		}
		if !validBackupRetention(c.S3.ServerPolicy.Retention) {
			errs = append(
				errs,
				errors.New("server backup retention must preserve at least one recovery point"),
			)
		}
		if !c.Database.External && !validBackupRetention(c.S3.DatabasePolicy.Retention) {
			errs = append(
				errs,
				errors.New("database backup retention must preserve at least one recovery point"),
			)
		}
		if c.S3.ValidatedAt.IsZero() {
			errs = append(errs, errors.New("object storage capability validation is required"))
		}
	}

	return errors.Join(errs...)
}

func validBackupRetention(retention BackupRetention) bool {
	if retention.KeepLast < 0 || retention.KeepDaily < 0 ||
		retention.KeepWeekly < 0 || retention.KeepMonthly < 0 {
		return false
	}
	return retention.KeepLast+retention.KeepDaily+
		retention.KeepWeekly+retention.KeepMonthly > 0
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
		c.Secrets.ServerAdminPassword,
		c.Secrets.SSHPrivateKey,
		c.Secrets.SSHCARecoveryPassphrase,
		c.Secrets.ClickHousePassword,
		c.Secrets.AdminPassword,
		c.Secrets.DatabasePassword,
		c.Secrets.SessionKey,
		c.Secrets.SessionEncryptionKey,
		c.Secrets.TokenSigningKey,
		c.Secrets.Pepper,
		c.Secrets.S3AccessKeyID,
		c.Secrets.S3SecretAccessKey,
		c.Secrets.ResticPassword,
		c.Secrets.AgeIdentity,
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
	return filepath.Join(
			etcDir,
			"installer.json",
		), filepath.Join(
			etcDir,
			"installer-secrets.json",
		), stateDir
}

func validHostname(value string) bool {
	if value == "" || !strings.Contains(value, ".") || len(value) > 253 ||
		strings.HasSuffix(value, ".") {
		return false
	}
	for label := range strings.SplitSeq(value, ".") {
		if len(label) == 0 || len(label) > 63 || !hostnameCharacter(label[0]) ||
			!hostnameCharacter(label[len(label)-1]) {
			return false
		}
		for index := 1; index < len(label)-1; index++ {
			if !hostnameCharacter(label[index]) && label[index] != '-' {
				return false
			}
		}
	}
	return true
}

func hostnameCharacter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9'
}

func randomHex(bytes int) (string, error) {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate random secret: %w", err)
	}
	return hex.EncodeToString(data), nil
}
