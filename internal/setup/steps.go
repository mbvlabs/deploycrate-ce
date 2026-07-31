package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	buildpacksclient "deploycrate-ce/clients/buildpacks"
	"deploycrate-ce/internal/secretcrypto"
)

const (
	BuildpacksPackVersion          = "0.40.6"
	CadvisorVersion                = "0.57.0"
	CaddyVersion                   = "2.11.4"
	DockerBuildxPackageVersion     = "0.35.0-1~debian.13~trixie"
	DockerCEPackageVersion         = "5:29.6.2-1~debian.13~trixie"
	DockerComposePackageVersion    = "5.3.1-1~debian.13~trixie"
	DockerContainerdPackageVersion = "2.2.6-1~debian.13~trixie"
	DockerEngineVersion            = "29.6.2"
	OpenTelemetryCollectorVersion  = "0.157.0"
	ResticVersion                  = "0.18.1"
)

type setupStep struct {
	id          string
	description string
	check       func(context.Context, Config, Runtime) (CheckResult, error)
	apply       func(context.Context, Config, Runtime, Reporter) error
}

func (step setupStep) ID() string             { return step.id }
func (step setupStep) Describe(Config) string { return step.description }

func (step setupStep) Check(ctx context.Context, cfg Config, runtime Runtime) (CheckResult, error) {
	if step.check == nil {
		return CheckResult{}, nil
	}
	return step.check(ctx, cfg, runtime)
}

func (step setupStep) Apply(
	ctx context.Context,
	cfg Config,
	runtime Runtime,
	report Reporter,
) error {
	return step.apply(ctx, cfg, runtime, report)
}

func scriptSetupStep(
	id, description, scriptName string,
	environment func(Config) map[string]string,
) Step {
	return setupStep{
		id: id, description: description,
		apply: func(ctx context.Context, cfg Config, runtime Runtime, report Reporter) error {
			script, err := loadScript(scriptName)
			if err != nil {
				return err
			}
			values := map[string]string{}
			if environment != nil {
				values = environment(cfg)
			}
			return runtime.Shell.Run(ctx, id, script, values, report)
		},
	}
}

func DefaultSteps(operations Operations) []Step {
	return []Step{
		scriptSetupStep("host-packages", "Install baseline host packages", "packages.sh", nil),
		scriptSetupStep(
			"server-users",
			"Create the administrator and internal service account",
			"user.sh",
			func(cfg Config) map[string]string {
				return map[string]string{
					"ADMIN_USER":           cfg.AdminUser,
					"SERVICE_USER":         cfg.ServiceUser,
					"PASSWORD":             cfg.Secrets.ServerAdminPassword,
					"SSH_PUBLIC_KEY":       cfg.SSHPublicKey,
					"OWNER_SSH_PUBLIC_KEY": cfg.OwnerSSHPublicKey,
				}
			},
		),
		scriptSetupStep(
			"backup-tools-0-18-1",
			"Install pinned backup tools and manifests",
			"backup.sh",
			func(Config) map[string]string {
				return map[string]string{"RESTIC_VERSION": ResticVersion}
			},
		),
		sshCASetupStep(),
		scriptSetupStep(
			"host-safety",
			"Configure journald, fail2ban, and swap",
			"host.sh",
			func(cfg Config) map[string]string {
				return map[string]string{"SSH_PORT": fmt.Sprint(cfg.SSHPort)}
			},
		),
		scriptSetupStep(
			"wireguard",
			"Configure the WireGuard mesh interface",
			"wireguard.sh",
			func(Config) map[string]string {
				return map[string]string{
					"WG_ADDRESS":     WireGuardPrivateAddress + "/16",
					"WG_LISTEN_PORT": fmt.Sprint(WireGuardListenPort),
				}
			},
		),
		scriptSetupStep(
			"node-exporter-1-11-1-netlink-v1",
			"Install hardened node-exporter on WireGuard",
			"node-exporter.sh",
			nil,
		),
		scriptSetupStep(
			"docker-29-6-2-journald-v1",
			"Install and configure the pinned Docker toolchain",
			"docker.sh",
			func(cfg Config) map[string]string {
				return map[string]string{
					"ADMIN_USER":                     cfg.AdminUser,
					"CONTAINERD_PACKAGE_VERSION":     DockerContainerdPackageVersion,
					"DOCKER_BUILDX_PACKAGE_VERSION":  DockerBuildxPackageVersion,
					"DOCKER_CE_PACKAGE_VERSION":      DockerCEPackageVersion,
					"DOCKER_COMPOSE_PACKAGE_VERSION": DockerComposePackageVersion,
					"DOCKER_ENGINE_VERSION":          DockerEngineVersion,
					"SERVICE_USER":                   cfg.ServiceUser,
				}
			},
		),
		scriptSetupStep(
			"cadvisor-"+CadvisorVersion+"-docker-attribution-v3",
			"Install localhost-only cAdvisor resource accounting",
			"cadvisor.sh",
			nil,
		),
		scriptSetupStep(
			"clickhouse-25-8-28-1-observability-v2",
			"Start pinned ClickHouse telemetry storage",
			"clickhouse.sh",
			func(cfg Config) map[string]string {
				return map[string]string{"CLICKHOUSE_PASSWORD": cfg.Secrets.ClickHousePassword}
			},
		),
		scriptSetupStep(
			"otel-collector-"+OpenTelemetryCollectorVersion+"-workload-ansi-v3",
			"Install durable logs, traces, and metrics collection",
			"otel-collector.sh",
			func(cfg Config) map[string]string {
				return map[string]string{
					"CLICKHOUSE_PASSWORD": cfg.Secrets.ClickHousePassword,
					"INSTANCE_ID":         cfg.InstanceID,
					"OTELCOL_VERSION":     OpenTelemetryCollectorVersion,
				}
			},
		),
		scriptSetupStep(
			"prometheus-3-13-1-service-metrics-v3",
			"Install Prometheus with 24-hour raw retention",
			"prometheus.sh",
			nil,
		),
		scriptSetupStep(
			"buildpacks-0-40-6-cache-v1",
			"Install and warm the Cloud Native Buildpacks tooling",
			"buildpacks.sh",
			func(cfg Config) map[string]string {
				builder, _ := buildpacksclient.PinnedBuilder()
				buildpack, _ := buildpacksclient.PinnedGoBuildpack()
				runImage, _ := buildpacksclient.PinnedRunImage()
				return map[string]string{
					"BUILDER_REFERENCE":      builder,
					"GO_BUILDPACK_REFERENCE": buildpack,
					"PACK_VERSION":           BuildpacksPackVersion,
					"RUN_IMAGE_REFERENCE":    runImage,
					"SERVICE_USER":           cfg.ServiceUser,
				}
			},
		),
		databaseStep(operations.ValidateDatabase),
		applicationBinaryStep(),
		applicationConfigStep(),
		migrationStep(operations.RunMigrations),
		adminStep(operations.EnsureAdmin),
		scriptSetupStep(
			"application-service-caddy-2-11-4",
			"Configure blue-green systemd slots and start pinned Caddy",
			"service.sh",
			func(cfg Config) map[string]string {
				return map[string]string{
					"CADDY_VERSION": CaddyVersion,
					"SERVICE_USER":  cfg.ServiceUser,
				}
			},
		),
		healthStep(),
		controlPlaneBootstrapStep(operations.BootstrapControlPlane, operations.VerifyControlPlaneRoute),
		scriptSetupStep(
			"ssh-hardening",
			"Harden SSH and configure the firewall",
			"ssh.sh",
			func(cfg Config) map[string]string {
				return map[string]string{
					"ADMIN_USER": cfg.AdminUser,
					"SSH_PORT":   fmt.Sprint(cfg.SSHPort),
				}
			},
		),
		scriptSetupStep(
			"service-health",
			"Verify control-plane services, listeners, and active slot",
			"service-health.sh",
			func(cfg Config) map[string]string {
				return map[string]string{
					"DATABASE_EXTERNAL":   fmt.Sprint(cfg.Database.External),
					"CLICKHOUSE_PASSWORD": cfg.Secrets.ClickHousePassword,
					"ADMIN_USER":          cfg.AdminUser,
					"SERVICE_USER":        cfg.ServiceUser,
				}
			},
		),
	}
}

func sshCASetupStep() Step {
	return setupStep{
		id:          "ssh-ca-recovery-v1",
		description: "Create SSH CAs and verify encrypted recovery bundle",
		apply: func(ctx context.Context, cfg Config, runtime Runtime, report Reporter) error {
			script, err := loadScript("ssh-ca.sh")
			if err != nil {
				return err
			}
			if err := runtime.Shell.Run(ctx, "ssh-ca-recovery-v1", script, map[string]string{
				"SERVICE_USER": cfg.ServiceUser,
				"DOMAIN":       cfg.Domain,
			}, report); err != nil {
				return err
			}
			if runtime.DryRun {
				return nil
			}
			if err := CreateSSHCARecoveryBundle(cfg.Secrets.SSHCARecoveryPassphrase); err != nil {
				return err
			}
			checksum, err := SSHCARecoveryBundleChecksum()
			if err != nil {
				return err
			}
			report(Event{
				Kind:   EventLog,
				StepID: "ssh-ca-recovery-v1",
				Line:   "SSH CA recovery bundle verified: " + checksum,
			})
			return nil
		},
	}
}

func databaseStep(validateDatabase func(context.Context, string) error) Step {
	return setupStep{
		id: "database-observability-v1", description: "Configure local or external PostgreSQL",
		apply: func(ctx context.Context, cfg Config, runtime Runtime, report Reporter) error {
			if runtime.DryRun {
				report(
					Event{
						Kind:   EventLog,
						StepID: "database-observability-v1",
						Line:   "dry run: database setup skipped",
					},
				)
				return nil
			}
			if !cfg.Database.External {
				script, err := loadScript("database.sh")
				if err != nil {
					return err
				}
				return runtime.Shell.Run(ctx, "database-observability-v1", script, map[string]string{
					"DB_NAME":     cfg.Database.Name,
					"DB_USER":     cfg.Database.User,
					"DB_PASSWORD": cfg.Secrets.DatabasePassword,
				}, report)
			}
			if validateDatabase == nil {
				return errors.New("external database validation is unavailable")
			}
			if err := validateDatabase(ctx, cfg.DatabaseURL()); err != nil {
				return fmt.Errorf("connect to external PostgreSQL: %w", err)
			}
			return nil
		},
	}
}

func applicationBinaryStep() Step {
	return setupStep{
		id: "application-binary", description: "Install the DeployCrate CE release binary",
		check: func(_ context.Context, cfg Config, runtime Runtime) (CheckResult, error) {
			if runtime.DryRun {
				return CheckResult{}, nil
			}
			releaseInfo, releaseErr := os.Stat(ApplicationReleaseBinaryPath(cfg.Version))
			slotInfo, slotErr := os.Stat(ApplicationSlotBinaryPath("blue"))
			complete := releaseErr == nil && slotErr == nil && releaseInfo.Mode().IsRegular() &&
				slotInfo.Mode().IsRegular()
			return CheckResult{
				Complete: complete,
				Detail:   "release binary and blue slot already installed",
			}, nil
		},
		apply: func(ctx context.Context, cfg Config, runtime Runtime, report Reporter) error {
			if runtime.DryRun {
				report(
					Event{
						Kind:   EventLog,
						StepID: "application-binary",
						Line:   "dry run: release binary placement skipped",
					},
				)
				return nil
			}
			return InstallApplicationReleaseBinary(ctx, "", cfg.Version)
		},
	}
}

func controlPlaneBootstrapStep(
	bootstrapControlPlane func(context.Context, BootstrapInput) (string, error),
	verifyControlPlaneRoute func(context.Context, string, string) error,
) Step {
	const stepID = "control-plane-topology-capabilities-v1"
	return setupStep{
		id:          stepID,
		description: "Create the control-plane topology and apply the Caddy route",
		apply: func(ctx context.Context, cfg Config, runtimeState Runtime, report Reporter) error {
			if runtimeState.DryRun {
				report(
					Event{
						Kind:   EventLog,
						StepID: stepID,
						Line:   "dry run: topology persistence and Caddy API configuration skipped",
					},
				)
				return nil
			}

			digest, err := ApplicationArtifactDigest(cfg.Version)
			if err != nil {
				return err
			}
			metadata, err := parseOSRelease()
			if err != nil {
				return err
			}
			wireGuardIdentity, err := LoadWireGuardIdentity(cfg.Secrets.SessionEncryptionKey)
			if err != nil {
				return err
			}
			if bootstrapControlPlane == nil || verifyControlPlaneRoute == nil {
				return errors.New("control-plane bootstrap operations are unavailable")
			}
			backupInput, err := encryptedBootstrapBackupInput(cfg)
			if err != nil {
				return err
			}
			wireGuardToolsVersion, err := installedWireGuardToolsVersion(ctx, runtimeState.Shell)
			if err != nil {
				return err
			}
			externalRouteID, err := bootstrapControlPlane(ctx, BootstrapInput{
				DatabaseURL: cfg.DatabaseURL(),
				Domain:      cfg.Domain,
				Version:     cfg.Version,
				ArtifactReference: ApplicationReleaseBinaryPath(
					cfg.Version,
				),
				ArtifactDigest:       digest,
				Distribution:         metadata["ID"],
				DistributionVersion:  metadata["VERSION_ID"],
				Architecture:         runtime.GOARCH,
				SessionEncryptionKey: cfg.Secrets.SessionEncryptionKey,
				Capabilities: BootstrapCapabilitiesInput{
					BuildpacksPackVersion: BuildpacksPackVersion,
					CaddyVersion:          CaddyVersion,
					DockerEngineVersion:   DockerEngineVersion,
					ResticVersion:         ResticVersion,
					WireGuardToolsVersion: wireGuardToolsVersion,
				},
				DatabaseExternal: cfg.Database.External,
				DatabaseHost:     cfg.Database.Host,
				DatabasePort:     cfg.Database.Port,
				DatabaseName:     cfg.Database.Name,
				DatabaseSSLMode:  cfg.Database.SSLMode,
				Backup:           backupInput,
				WireGuard: BootstrapWireGuardInput{
					Interface:           WireGuardInterface,
					NetworkCIDR:         WireGuardNetworkCIDR,
					PrivateAddress:      WireGuardPrivateAddress,
					PublicKey:           wireGuardIdentity.PublicKey,
					EncryptedPrivateKey: wireGuardIdentity.EncryptedPrivateKey,
					Endpoint: cfg.Domain + ":" + fmt.Sprint(
						WireGuardListenPort,
					),
					ListenPort: WireGuardListenPort,
				},
			})
			if err != nil {
				return err
			}
			report(
				Event{
					Kind:   EventLog,
					StepID: stepID,
					Line:   "bootstrap topology committed and Caddy route applied",
				},
			)

			script, err := loadScript("caddy-resume.sh")
			if err != nil {
				return err
			}
			if err := runtimeState.Shell.Run(
				ctx,
				stepID,
				script,
				nil,
				report,
			); err != nil {
				return err
			}
			if err := verifyControlPlaneRoute(ctx, cfg.DatabaseURL(), externalRouteID); err != nil {
				return fmt.Errorf("verify Caddy route after restart: %w", err)
			}
			return nil
		},
	}
}

func installedWireGuardToolsVersion(ctx context.Context, shell Shell) (string, error) {
	output, err := shell.Output(ctx, "wg", "--version")
	if err != nil {
		return "", fmt.Errorf("read WireGuard tools version: %w", err)
	}
	fields := strings.Fields(output)
	if len(fields) < 2 || fields[0] != "wireguard-tools" {
		return "", fmt.Errorf("unexpected WireGuard tools version output %q", output)
	}
	version := strings.TrimPrefix(fields[1], "v")
	if version == "" {
		return "", fmt.Errorf("unexpected WireGuard tools version output %q", output)
	}
	return version, nil
}

func encryptedBootstrapBackupInput(cfg Config) (BootstrapBackupInput, error) {
	if !cfg.S3.Enabled {
		return BootstrapBackupInput{}, nil
	}
	payload, err := json.Marshal(struct {
		AccessKeyID     string `json:"access_key_id"`
		SecretAccessKey string `json:"secret_access_key"`
		ResticPassword  string `json:"restic_password"`
		AgeIdentity     string `json:"age_identity"`
	}{
		AccessKeyID:     cfg.Secrets.S3AccessKeyID,
		SecretAccessKey: cfg.Secrets.S3SecretAccessKey,
		ResticPassword:  cfg.Secrets.ResticPassword,
		AgeIdentity:     cfg.Secrets.AgeIdentity,
	})
	if err != nil {
		return BootstrapBackupInput{}, fmt.Errorf("encode backup credential: %w", err)
	}
	defer clear(payload)
	encrypted, err := secretcrypto.Encrypt(payload, cfg.Secrets.SessionEncryptionKey)
	if err != nil {
		return BootstrapBackupInput{}, fmt.Errorf("encrypt backup credential: %w", err)
	}
	serverRetention, err := json.Marshal(cfg.S3.ServerPolicy.Retention)
	if err != nil {
		return BootstrapBackupInput{}, err
	}
	databaseRetention, err := json.Marshal(cfg.S3.DatabasePolicy.Retention)
	if err != nil {
		return BootstrapBackupInput{}, err
	}
	return BootstrapBackupInput{
		Enabled:                    true,
		InstanceID:                 cfg.InstanceID,
		Provider:                   cfg.S3.Provider,
		Endpoint:                   cfg.S3.Endpoint,
		Region:                     cfg.S3.Region,
		Bucket:                     cfg.S3.Bucket,
		Prefix:                     cfg.S3.Prefix,
		ForcePathStyle:             cfg.S3.UsePathStyle,
		EncryptedCredentialPayload: encrypted,
		ValidatedAt:                cfg.S3.ValidatedAt,
		ServerSchedule:             cfg.S3.ServerPolicy.Schedule,
		ServerRetention:            serverRetention,
		DatabaseSchedule:           cfg.S3.DatabasePolicy.Schedule,
		DatabaseRetention:          databaseRetention,
	}, nil
}

func applicationConfigStep() Step {
	return setupStep{
		id:          "application-config-backups-v1",
		description: "Write protected application secrets and configuration",
		apply: func(_ context.Context, cfg Config, runtime Runtime, report Reporter) error {
			if runtime.DryRun {
				report(
					Event{
						Kind:   EventLog,
						StepID: "application-config-backups-v1",
						Line:   "dry run: application configuration skipped",
					},
				)
				return nil
			}
			return WriteApplicationEnvironment(cfg)
		},
	}
}

func migrationStep(runMigrations func(context.Context, string) error) Step {
	return setupStep{
		id: "database-migrations", description: "Apply embedded database migrations",
		apply: func(ctx context.Context, cfg Config, runtime Runtime, report Reporter) error {
			if runtime.DryRun {
				report(
					Event{
						Kind:   EventLog,
						StepID: "database-migrations",
						Line:   "dry run: migrations skipped",
					},
				)
				return nil
			}
			if runMigrations == nil {
				return errors.New("database migration operation is unavailable")
			}
			return runMigrations(ctx, cfg.DatabaseURL())
		},
	}
}

func adminStep(ensureAdmin func(context.Context, AdminInput) error) Step {
	return setupStep{
		id: "application-admin", description: "Create or update the application administrator",
		apply: func(ctx context.Context, cfg Config, runtime Runtime, report Reporter) error {
			if runtime.DryRun {
				report(
					Event{
						Kind:   EventLog,
						StepID: "application-admin",
						Line:   "dry run: administrator setup skipped",
					},
				)
				return nil
			}
			if ensureAdmin == nil {
				return errors.New("administrator setup operation is unavailable")
			}
			return ensureAdmin(ctx, AdminInput{DatabaseURL: cfg.DatabaseURL(), Email: cfg.AdminEmail, Password: cfg.Secrets.AdminPassword, Pepper: cfg.Secrets.Pepper})
		},
	}
}

func healthStep() Step {
	return setupStep{
		id: "health-check", description: "Verify the application health endpoint",
		apply: func(ctx context.Context, _ Config, runtime Runtime, report Reporter) error {
			if runtime.DryRun {
				report(
					Event{
						Kind:   EventLog,
						StepID: "health-check",
						Line:   "dry run: health check skipped",
					},
				)
				return nil
			}
			client := &http.Client{Timeout: 5 * time.Second}
			for attempt := 1; attempt <= 30; attempt++ {
				request, err := http.NewRequestWithContext(
					ctx,
					http.MethodGet,
					"http://127.0.0.1:8080/api/health",
					nil,
				)
				if err != nil {
					return err
				}
				response, err := client.Do(request)
				if err == nil {
					response.Body.Close()
					if response.StatusCode == http.StatusOK {
						return nil
					}
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(2 * time.Second):
				}
			}
			return errors.New("application health endpoint did not become ready")
		},
	}
}

func NewRunner(cfg Config, dryRun bool, operations Operations) Runner {
	return Runner{
		Steps: DefaultSteps(operations),
		Store: NewStateStore(),
		Run:   Runtime{DryRun: dryRun, Shell: NewShell(dryRun, cfg.SecretValues())},
	}
}
