package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	caddyclients "deploycrate-ce/clients/caddy"
	"deploycrate-ce/clients/objectstorage"
	"deploycrate-ce/database"
	"deploycrate-ce/internal/setup"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/services"
)

func newSetupOperations() setup.Operations {
	return setup.Operations{
		NormalizeObjectStorage:  normalizeSetupObjectStorage,
		ValidateRemoteServices:  validateSetupRemoteServices,
		ValidateDatabase:        validateSetupDatabase,
		RunMigrations:           runSetupMigrations,
		EnsureAdmin:             ensureSetupAdmin,
		BootstrapControlPlane:   bootstrapSetupControlPlane,
		VerifyControlPlaneRoute: verifySetupControlPlaneRoute,
	}
}

func normalizeSetupObjectStorage(input setup.S3Config) (setup.S3Config, error) {
	normalized, err := objectstorage.Normalize(objectstorage.Config{
		Provider: input.Provider, Endpoint: input.Endpoint, Region: input.Region,
		Bucket: input.Bucket, Prefix: input.Prefix, ForcePathStyle: input.UsePathStyle,
	})
	if err != nil {
		return setup.S3Config{}, err
	}
	input.Provider = normalized.Provider
	input.Endpoint = normalized.Endpoint
	input.Region = normalized.Region
	input.Bucket = normalized.Bucket
	input.Prefix = normalized.Prefix
	input.UsePathStyle = normalized.ForcePathStyle
	return input, nil
}

func validateSetupRemoteServices(ctx context.Context, cfg setup.Config) error {
	if cfg.Database.External {
		if err := validateSetupDatabase(ctx, cfg.DatabaseURL()); err != nil {
			return fmt.Errorf("external database validation failed: %w", err)
		}
	}
	if !cfg.S3.Enabled {
		return nil
	}
	client, err := objectstorage.New(ctx, objectstorage.Config{
		Provider: cfg.S3.Provider, Endpoint: cfg.S3.Endpoint, Region: cfg.S3.Region,
		Bucket: cfg.S3.Bucket, Prefix: cfg.S3.Prefix, ForcePathStyle: cfg.S3.UsePathStyle,
	}, objectstorage.Credentials{
		AccessKeyID:     cfg.Secrets.S3AccessKeyID,
		SecretAccessKey: cfg.Secrets.S3SecretAccessKey,
	})
	if err != nil {
		return fmt.Errorf("object storage validation failed: %w", err)
	}
	if err := client.Probe(ctx, cfg.InstanceID); err != nil {
		return fmt.Errorf("object storage validation failed: %w", err)
	}
	return nil
}

func validateSetupDatabase(ctx context.Context, databaseURL string) error {
	db, err := storage.NewPostgres(ctx, databaseURL)
	if err != nil {
		return err
	}
	return db.Close()
}

func runSetupMigrations(ctx context.Context, databaseURL string) error {
	db, err := storage.NewPostgres(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	return database.ApplyMigrations(ctx, db)
}

func ensureSetupAdmin(ctx context.Context, input setup.AdminInput) error {
	db, err := storage.NewPostgres(ctx, input.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	hashedPassword, err := models.HashPassword(input.Password, input.Pepper)
	if err != nil {
		return fmt.Errorf("hash administrator password: %w", err)
	}
	admin, err := models.User.FindByEmail(ctx, db.Executor(), input.Email)
	if errors.Is(err, models.ErrNotFound) {
		admin, err = models.User.Create(ctx, db.Executor(), input.Pepper, models.CreateUserData{
			Email: input.Email,
			PasswordPair: models.PasswordPair{
				Password: input.Password, ConfirmPassword: input.Password,
			},
		})
	}
	if err != nil {
		return fmt.Errorf("find or create administrator: %w", err)
	}
	_, err = models.User.Update(ctx, db.Executor(), models.UpdateUserData{
		ID: admin.ID, Email: input.Email, Password: []byte(hashedPassword), IsAdmin: true,
		EmailValidatedAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	return err
}

func bootstrapSetupControlPlane(ctx context.Context, input setup.BootstrapInput) (string, error) {
	db, err := storage.NewPostgres(ctx, input.DatabaseURL)
	if err != nil {
		return "", err
	}
	defer db.Close()
	routes := services.NewCaddyRouteService(db, caddyclients.New(""))
	bootstrap := services.NewBootstrapService(db, routes)
	result, err := bootstrap.Bootstrap(ctx, services.BootstrapInput{
		Domain: input.Domain, Version: input.Version,
		ArtifactReference: input.ArtifactReference, ArtifactDigest: input.ArtifactDigest,
		Distribution: input.Distribution, DistributionVersion: input.DistributionVersion,
		Architecture:         input.Architecture,
		SessionEncryptionKey: input.SessionEncryptionKey,
		ClickHouseUser:       input.ClickHouseUser,
		ClickHousePassword:   input.ClickHousePassword,
		Capabilities: services.BootstrapCapabilitiesInput{
			BuildpacksPackVersion: input.Capabilities.BuildpacksPackVersion,
			CaddyVersion:          input.Capabilities.CaddyVersion,
			DockerEngineVersion:   input.Capabilities.DockerEngineVersion,
			ResticVersion:         input.Capabilities.ResticVersion,
			WireGuardToolsVersion: input.Capabilities.WireGuardToolsVersion,
		},
		DatabaseExternal: input.DatabaseExternal,
		DatabaseHost:     input.DatabaseHost, DatabasePort: input.DatabasePort,
		DatabaseName: input.DatabaseName, DatabaseUser: input.DatabaseUser,
		DatabasePassword: input.DatabasePassword, DatabaseSSLMode: input.DatabaseSSLMode,
		DatabaseInstallationID: input.DatabaseInstallationID,
		Backup: services.BootstrapBackupInput{
			Enabled: input.Backup.Enabled, InstanceID: input.Backup.InstanceID,
			Provider: input.Backup.Provider, Endpoint: input.Backup.Endpoint,
			Region: input.Backup.Region, Bucket: input.Backup.Bucket, Prefix: input.Backup.Prefix,
			ForcePathStyle:             input.Backup.ForcePathStyle,
			EncryptedCredentialPayload: input.Backup.EncryptedCredentialPayload,
			ValidatedAt:                input.Backup.ValidatedAt, ServerSchedule: input.Backup.ServerSchedule,
			ServerRetention:   input.Backup.ServerRetention,
			DatabaseSchedule:  input.Backup.DatabaseSchedule,
			DatabaseRetention: input.Backup.DatabaseRetention,
		},
		WireGuard: services.BootstrapWireGuardInput{
			Interface: input.WireGuard.Interface, NetworkCIDR: input.WireGuard.NetworkCIDR,
			PrivateAddress: input.WireGuard.PrivateAddress, PublicKey: input.WireGuard.PublicKey,
			EncryptedPrivateKey: input.WireGuard.EncryptedPrivateKey,
			Endpoint:            input.WireGuard.Endpoint, ListenPort: input.WireGuard.ListenPort,
		},
	})
	if err != nil {
		return "", err
	}
	return result.ExternalRouteID, nil
}

func verifySetupControlPlaneRoute(ctx context.Context, databaseURL, externalRouteID string) error {
	db, err := storage.NewPostgres(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	routes := services.NewCaddyRouteService(db, caddyclients.New(""))
	return services.NewBootstrapService(db, routes).VerifyRoute(ctx, externalRouteID)
}
