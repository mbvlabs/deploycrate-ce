package setup

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Operations struct {
	NormalizeObjectStorage  func(S3Config) (S3Config, error)
	ValidateRemoteServices  func(context.Context, Config) error
	ValidateDatabase        func(context.Context, string) error
	RunMigrations           func(context.Context, string) error
	RunClickHouseMigrations func(context.Context, ClickHouseMigrationInput) error
	EnsureAdmin             func(context.Context, AdminInput) error
	BootstrapControlPlane   func(context.Context, BootstrapInput) (string, error)
	VerifyControlPlaneRoute func(context.Context, string, string) error
}

type ClickHouseMigrationInput struct {
	Protocol string
	Host     string
	Port     string
	Database string
	User     string
	Password string
}

type AdminInput struct {
	DatabaseURL string
	Email       string
	Password    string
	Pepper      string
}

type BootstrapInput struct {
	DatabaseURL            string
	Domain                 string
	Version                string
	ArtifactReference      string
	ArtifactDigest         []byte
	Distribution           string
	DistributionVersion    string
	Architecture           string
	SessionEncryptionKey   string
	Capabilities           BootstrapCapabilitiesInput
	ClickHouseUser         string
	ClickHousePassword     string
	DatabaseExternal       bool
	DatabaseHost           string
	DatabasePort           int
	DatabaseName           string
	DatabaseUser           string
	DatabasePassword       string
	DatabaseSSLMode        string
	DatabaseInstallationID uuid.UUID
	WireGuard              BootstrapWireGuardInput
	Backup                 BootstrapBackupInput
}

type BootstrapCapabilitiesInput struct {
	BuildpacksPackVersion string
	CaddyVersion          string
	DockerEngineVersion   string
	ResticVersion         string
	WireGuardToolsVersion string
}

type BootstrapBackupInput struct {
	Enabled                    bool
	InstanceID                 string
	Provider                   string
	Endpoint                   string
	Region                     string
	Bucket                     string
	Prefix                     string
	ForcePathStyle             bool
	EncryptedCredentialPayload []byte
	ValidatedAt                time.Time
	ServerSchedule             string
	ServerRetention            json.RawMessage
	DatabaseSchedule           string
	DatabaseRetention          json.RawMessage
}

type BootstrapWireGuardInput struct {
	Interface           string
	NetworkCIDR         string
	PrivateAddress      string
	PublicKey           string
	EncryptedPrivateKey []byte
	Endpoint            string
	ListenPort          int
}
