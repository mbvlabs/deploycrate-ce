package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	clickhouseclient "deploycrate-ce/clients/clickhouse"
	"deploycrate-ce/config"
)

const clickHouseMetricRollupSchemaVersion = "1"

type ClickHouseBackupArtifact struct {
	Path          string `json:"path"`
	Format        string `json:"format"`
	SchemaVersion string `json:"schema_version"`
	Rows          int64  `json:"rows"`
	FirstBucket   string `json:"first_bucket,omitempty"`
	LastBucket    string `json:"last_bucket,omitempty"`
	SizeBytes     int64  `json:"size_bytes"`
	SHA256        string `json:"sha256"`
}

type ClickHouseBackup struct {
	client clickhouseclient.Client
}

func NewClickHouseBackup(configuration config.Config) *ClickHouseBackup {
	return &ClickHouseBackup{client: clickhouseclient.New(
		configuration.Metrics.ClickHouseURL,
		configuration.Metrics.ClickHouseDatabase,
		configuration.Metrics.ClickHouseUser,
		configuration.Metrics.ClickHousePassword,
	)}
}

func (service *ClickHouseBackup) Export(
	ctx context.Context,
	destination string,
) (ClickHouseBackupArtifact, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return ClickHouseBackupArtifact{}, fmt.Errorf("create ClickHouse backup staging directory: %w", err)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return ClickHouseBackupArtifact{}, fmt.Errorf("create ClickHouse metric rollup export: %w", err)
	}
	digest := sha256.New()
	export, exportErr := service.client.ExportMetricRollups(ctx, io.MultiWriter(file, digest))
	closeErr := file.Close()
	if exportErr != nil || closeErr != nil {
		_ = os.Remove(destination)
		return ClickHouseBackupArtifact{}, fmt.Errorf(
			"write ClickHouse metric rollup export: %w",
			errors.Join(exportErr, closeErr),
		)
	}
	info, err := os.Stat(destination)
	if err != nil {
		_ = os.Remove(destination)
		return ClickHouseBackupArtifact{}, fmt.Errorf("inspect ClickHouse metric rollup export: %w", err)
	}
	return ClickHouseBackupArtifact{
		Path: destination, Format: "JSONEachRow", SchemaVersion: clickHouseMetricRollupSchemaVersion,
		Rows: export.Rows, FirstBucket: export.FirstBucket, LastBucket: export.LastBucket,
		SizeBytes: info.Size(), SHA256: hex.EncodeToString(digest.Sum(nil)),
	}, nil
}
