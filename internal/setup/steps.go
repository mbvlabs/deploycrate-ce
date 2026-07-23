package setup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	caddyclients "deploycrate-ce/clients/caddy"
	"deploycrate-ce/database"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/services"
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

func (step setupStep) Apply(ctx context.Context, cfg Config, runtime Runtime, report Reporter) error {
	return step.apply(ctx, cfg, runtime, report)
}

func scriptSetupStep(id, description, scriptName string, environment func(Config) map[string]string) Step {
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

func DefaultSteps() []Step {
	return []Step{
		scriptSetupStep("host-packages", "Install baseline host packages", "packages.sh", nil),
		scriptSetupStep("deploycrate-user", "Create unrestricted deploycrate user and SSH access", "user.sh", func(cfg Config) map[string]string {
			return map[string]string{"USERNAME": cfg.LinuxUser, "PASSWORD": cfg.Secrets.LinuxPassword, "SSH_PUBLIC_KEY": cfg.SSHPublicKey}
		}),
		scriptSetupStep("host-safety", "Configure journald, fail2ban, and swap", "host.sh", func(cfg Config) map[string]string {
			return map[string]string{"SSH_PORT": fmt.Sprint(cfg.SSHPort)}
		}),
		scriptSetupStep("docker", "Install and configure Docker Engine", "docker.sh", func(cfg Config) map[string]string {
			return map[string]string{"USERNAME": cfg.LinuxUser}
		}),
		scriptSetupStep("buildpacks", "Install the Cloud Native Buildpacks tooling", "buildpacks.sh", func(cfg Config) map[string]string {
			return map[string]string{"USERNAME": cfg.LinuxUser}
		}),
		databaseStep(),
		applicationBinaryStep(),
		applicationConfigStep(),
		migrationStep(),
		adminStep(),
		backupConfigStep(),
		scriptSetupStep("application-service", "Configure blue-green systemd slots and start Caddy", "service.sh", func(cfg Config) map[string]string {
			return map[string]string{"USERNAME": cfg.LinuxUser}
		}),
		healthStep(),
		controlPlaneBootstrapStep(),
		scriptSetupStep("ssh-hardening", "Harden SSH and configure the firewall", "ssh.sh", func(cfg Config) map[string]string {
			return map[string]string{"USERNAME": cfg.LinuxUser, "SSH_PORT": fmt.Sprint(cfg.SSHPort)}
		}),
	}
}

func databaseStep() Step {
	return setupStep{
		id: "database", description: "Configure local or external PostgreSQL",
		apply: func(ctx context.Context, cfg Config, runtime Runtime, report Reporter) error {
			if runtime.DryRun {
				report(Event{Kind: EventLog, StepID: "database", Line: "dry run: database setup skipped"})
				return nil
			}
			if !cfg.Database.External {
				script, err := loadScript("database.sh")
				if err != nil {
					return err
				}
				return runtime.Shell.Run(ctx, "database", script, map[string]string{
					"DB_NAME": cfg.Database.Name, "DB_USER": cfg.Database.User, "DB_PASSWORD": cfg.Secrets.DatabasePassword,
				}, report)
			}
			db, err := storage.NewPostgres(ctx, cfg.DatabaseURL())
			if err != nil {
				return fmt.Errorf("connect to external PostgreSQL: %w", err)
			}
			return db.Close()
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
			complete := releaseErr == nil && slotErr == nil && releaseInfo.Mode().IsRegular() && slotInfo.Mode().IsRegular()
			return CheckResult{Complete: complete, Detail: "release binary and blue slot already installed"}, nil
		},
		apply: func(_ context.Context, cfg Config, runtime Runtime, report Reporter) error {
			if runtime.DryRun {
				report(Event{Kind: EventLog, StepID: "application-binary", Line: "dry run: release binary placement skipped"})
				return nil
			}
			return InstallApplicationReleaseBinary("", cfg.Version)
		},
	}
}

func controlPlaneBootstrapStep() Step {
	return setupStep{
		id: "control-plane-topology", description: "Create the control-plane topology and apply the Caddy route",
		apply: func(ctx context.Context, cfg Config, runtimeState Runtime, report Reporter) error {
			if runtimeState.DryRun {
				report(Event{Kind: EventLog, StepID: "control-plane-topology", Line: "dry run: topology persistence and Caddy API configuration skipped"})
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
			db, err := storage.NewPostgres(ctx, cfg.DatabaseURL())
			if err != nil {
				return err
			}
			defer db.Close()

			routes := services.NewCaddyRouteService(db, caddyclients.New(""))
			bootstrap := services.NewBootstrapService(db, routes)
			result, err := bootstrap.Bootstrap(ctx, services.BootstrapInput{
				Domain: cfg.Domain, Version: cfg.Version,
				ArtifactReference: ApplicationReleaseBinaryPath(cfg.Version), ArtifactDigest: digest,
				Distribution: metadata["ID"], DistributionVersion: metadata["VERSION_ID"], Architecture: runtime.GOARCH,
				DatabaseExternal: cfg.Database.External, DatabaseHost: cfg.Database.Host,
				DatabasePort: cfg.Database.Port, DatabaseName: cfg.Database.Name, DatabaseSSLMode: cfg.Database.SSLMode,
			})
			if err != nil {
				return err
			}
			report(Event{Kind: EventLog, StepID: "control-plane-topology", Line: "bootstrap topology committed and Caddy route applied"})

			script, err := loadScript("caddy-resume.sh")
			if err != nil {
				return err
			}
			if err := runtimeState.Shell.Run(ctx, "control-plane-topology", script, nil, report); err != nil {
				return err
			}
			if err := bootstrap.VerifyRoute(ctx, result.ExternalRouteID); err != nil {
				return fmt.Errorf("verify Caddy route after restart: %w", err)
			}
			return nil
		},
	}
}

func applicationConfigStep() Step {
	return setupStep{
		id: "application-config", description: "Write protected application secrets and configuration",
		apply: func(_ context.Context, cfg Config, runtime Runtime, report Reporter) error {
			if runtime.DryRun {
				report(Event{Kind: EventLog, StepID: "application-config", Line: "dry run: application configuration skipped"})
				return nil
			}
			return WriteApplicationEnvironment(cfg)
		},
	}
}

func migrationStep() Step {
	return setupStep{
		id: "database-migrations", description: "Apply embedded database migrations",
		apply: func(ctx context.Context, cfg Config, runtime Runtime, report Reporter) error {
			if runtime.DryRun {
				report(Event{Kind: EventLog, StepID: "database-migrations", Line: "dry run: migrations skipped"})
				return nil
			}
			db, err := storage.NewPostgres(ctx, cfg.DatabaseURL())
			if err != nil {
				return err
			}
			defer db.Close()
			return storage.RunMigrations(ctx, db.Conn(), database.Migrations, "migrations")
		},
	}
}

func adminStep() Step {
	return setupStep{
		id: "application-admin", description: "Create or update the application administrator",
		apply: func(ctx context.Context, cfg Config, runtime Runtime, report Reporter) error {
			if runtime.DryRun {
				report(Event{Kind: EventLog, StepID: "application-admin", Line: "dry run: administrator setup skipped"})
				return nil
			}
			db, err := storage.NewPostgres(ctx, cfg.DatabaseURL())
			if err != nil {
				return err
			}
			defer db.Close()
			hashedPassword, err := models.HashPassword(cfg.Secrets.AdminPassword, cfg.Secrets.Pepper)
			if err != nil {
				return fmt.Errorf("hash administrator password: %w", err)
			}
			admin, err := models.User.FindByEmail(ctx, db.Executor(), cfg.AdminEmail)
			if errors.Is(err, models.ErrNotFound) {
				admin, err = models.User.Create(ctx, db.Executor(), cfg.Secrets.Pepper, models.CreateUserData{
					Email:        cfg.AdminEmail,
					PasswordPair: models.PasswordPair{Password: cfg.Secrets.AdminPassword, ConfirmPassword: cfg.Secrets.AdminPassword},
				})
			}
			if err != nil {
				return fmt.Errorf("find or create administrator: %w", err)
			}
			_, err = models.User.Update(ctx, db.Executor(), models.UpdateUserData{
				ID: admin.ID, Email: cfg.AdminEmail, Password: []byte(hashedPassword), IsAdmin: true,
				EmailValidatedAt: sql.NullTime{Time: time.Now(), Valid: true},
			})
			return err
		},
	}
}

func backupConfigStep() Step {
	return setupStep{
		id: "backup-destination", description: "Store S3-compatible backup configuration",
		apply: func(_ context.Context, cfg Config, runtime Runtime, report Reporter) error {
			if !cfg.S3.Enabled {
				report(Event{Kind: EventLog, StepID: "backup-destination", Line: "S3-compatible backups were not selected"})
				return nil
			}
			if runtime.DryRun {
				report(Event{Kind: EventLog, StepID: "backup-destination", Line: "dry run: backup configuration skipped"})
				return nil
			}
			return WriteBackupEnvironment(cfg)
		},
	}
}

func healthStep() Step {
	return setupStep{
		id: "health-check", description: "Verify the application health endpoint",
		apply: func(ctx context.Context, _ Config, runtime Runtime, report Reporter) error {
			if runtime.DryRun {
				report(Event{Kind: EventLog, StepID: "health-check", Line: "dry run: health check skipped"})
				return nil
			}
			client := &http.Client{Timeout: 5 * time.Second}
			for attempt := 1; attempt <= 30; attempt++ {
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:8080/api/health", nil)
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

func NewRunner(cfg Config, dryRun bool) Runner {
	return Runner{
		Steps: DefaultSteps(),
		Store: NewStateStore(),
		Run:   Runtime{DryRun: dryRun, Shell: NewShell(dryRun, cfg.SecretValues())},
	}
}
