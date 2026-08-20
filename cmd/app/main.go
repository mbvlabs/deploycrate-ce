package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"deploycrate-ce/assets"
	mailclients "deploycrate-ce/clients/email"
	"deploycrate-ce/config"
	"deploycrate-ce/controllers"
	"deploycrate-ce/database"
	"deploycrate-ce/email"
	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/resourceaccess"
	"deploycrate-ce/internal/server"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/queue"
	"deploycrate-ce/router"
	"deploycrate-ce/router/routes"
	"deploycrate-ce/services"
	"deploycrate-ce/telemetry"

	"go.uber.org/fx"
)

var appVersion = "dev"

const exitFail = 1

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(exitFail)
	}
}

func run(arguments []string, stdout io.Writer) error {
	if len(arguments) == 0 {
		return errors.New("program arguments are required")
	}
	if len(arguments) > 1 {
		if len(arguments) == 2 && arguments[1] == "jobs" {
			return runJobRunner()
		}
		return runCommand(context.Background(), arguments[1:], stdout)
	}

	rootHTML, err := assets.Files.ReadFile("inertia/root.go.html")
	if err != nil {
		return fmt.Errorf("read Inertia root template: %w", err)
	}
	viteManifest, err := assets.Files.ReadFile("dist/vite/manifest.json")
	if err != nil {
		return fmt.Errorf("read Vite manifest: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := inertia.Init(
		config.ProjectName,
		config.Env,
		routes.ViteBuild.Path(),
		rootHTML,
		viteManifest,
		inertia.WithSharedProp("appVersion", appVersion),
	); err != nil {
		return fmt.Errorf("initialize inertia: %w", err)
	}
	app := fx.New(appOptions(ctx)...)
	if err := app.Start(ctx); err != nil {
		return err
	}

	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer shutdownCancel()
	return app.Stop(shutdownCtx)
}

func appOptions(ctx context.Context) []fx.Option {
	return []fx.Option{
		sharedOptions(ctx),
		queue.WorkersModule,
		controllers.Module,
		router.Module,

		fx.Invoke(runMigrationsOnStartup),
		fx.Invoke(reconcileWorkloadsOnStartup),
		fx.Invoke(startServer),
	}
}

func jobRunnerOptions(ctx context.Context) []fx.Option {
	return []fx.Option{
		sharedOptions(ctx),
		queue.WorkersModule,

		fx.Invoke(runMigrationsOnStartup),
		fx.Invoke(startQueueProcessor),
		fx.Invoke(ensureInitialBackupsOnStartup),
		fx.Invoke(services.StartResourceCaddyReconciler),
	}
}

func sharedOptions(ctx context.Context) fx.Option {
	return fx.Options(
		fx.Provide(
			func() context.Context { return ctx },
			func() services.CurrentVersion { return services.CurrentVersion(appVersion) },
			func() telemetry.ServiceVersion { return telemetry.ServiceVersion(appVersion) },
			func(service services.MetricRollupService) queue.MetricRollupCollector { return service },
			func(cfg config.Config) (email.TransactionalSender, email.MarketingSender) {
				if config.Env == server.ProdEnvironment {
					return mailclients.NewAwsSes(cfg), mailclients.NewAwsSes(cfg)
				}

				return mailclients.NewMailpit(cfg), mailclients.NewMailpit(cfg)
			},
		),

		config.Module,
		database.Module,
		telemetry.Module,
		queue.Module,
		services.Module,
	)
}

func runJobRunner() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	app := fx.New(jobRunnerOptions(ctx)...)
	if err := app.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer shutdownCancel()
	return app.Stop(shutdownCtx)
}

func runCommand(ctx context.Context, arguments []string, stdout io.Writer) error {
	if len(arguments) == 0 {
		return errors.New("command is required")
	}
	switch arguments[0] {
	case "version":
		if len(arguments) != 1 {
			return errors.New("usage: deploycrate-ce version")
		}
		_, err := fmt.Fprintln(stdout, appVersion)
		return err
	case "migrate":
		if len(arguments) != 1 {
			return errors.New("usage: deploycrate-ce migrate")
		}
		cfg := config.NewConfig()
		db, err := database.NewPostgres(ctx, cfg)
		if err != nil {
			return err
		}
		defer db.Close()
		clickHouse, err := database.NewClickHouse(ctx, cfg)
		if err != nil {
			return err
		}
		defer clickHouse.Close()
		if err := database.ApplyAllMigrations(ctx, db, clickHouse); err != nil {
			return fmt.Errorf("run database migrations: %w", err)
		}
		return nil
	case "backups":
		if len(arguments) != 2 || arguments[1] != "activate" {
			return errors.New("usage: deploycrate-ce backups activate")
		}
		cfg := config.NewConfig()
		db, err := database.NewPostgres(ctx, cfg)
		if err != nil {
			return err
		}
		defer db.Close()
		activated, err := services.NewBackupPolicyActivator(db).Activate(
			ctx,
			cfg.App.InstanceID,
		)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "activated %d backup policies\n", activated)
		return err
	case "host-resource-access":
		return resourceaccess.RunHostCommand(arguments[1:])
	default:
		return fmt.Errorf("unknown deploycrate-ce command %q", arguments[0])
	}
}

func runMigrationsOnStartup(
	lc fx.Lifecycle,
	db storage.Pool,
	clickHouse *database.ClickHouse,
) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := database.ApplyAllMigrations(ctx, db, clickHouse); err != nil {
				return fmt.Errorf("run startup database migrations: %w", err)
			}
			return nil
		},
	})
}

func reconcileWorkloadsOnStartup(lc fx.Lifecycle, reconciliation *services.WorkloadReconciliation) {
	lc.Append(fx.Hook{OnStart: func(ctx context.Context) error {
		if err := reconciliation.Reconcile(ctx); err != nil {
			return fmt.Errorf("reconcile workload state on startup: %w", err)
		}
		return nil
	}})
}

func startQueueProcessor(lc fx.Lifecycle, appCtx context.Context, p queue.Processor) {
	var done <-chan struct{}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := p.Seed(ctx); err != nil {
				return fmt.Errorf("seed backup schedules: %w", err)
			}
			done = startInBackground(appCtx, "queue processor", p.Start)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return stopAndWait(ctx, p.Stop, done)
		},
	})
}

func startServer(lc fx.Lifecycle, appCtx context.Context, r *router.Router, cfg config.Config) {
	requestBaseCtx := context.WithoutCancel(appCtx)
	srv := server.New(
		requestBaseCtx,
		cfg.App.Host,
		cfg.App.Port,
		config.Env,
		r.Handler,
		nil,
	)
	var done <-chan struct{}

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			slog.InfoContext(
				appCtx,
				"starting server",
				"host",
				cfg.App.Host,
				"port",
				cfg.App.Port,
			)
			done = startInBackground(appCtx, "server", func(ctx context.Context) error {
				return srv.Start(ctx, config.Env)
			})
			return nil
		},
		OnStop: func(ctx context.Context) error {
			slog.InfoContext(ctx, "initiating graceful shutdown")
			return stopAndWait(ctx, func(ctx context.Context) error {
				var shutdownErr error
				for _, shutdowner := range srv.Shutdowners {
					if err := shutdowner.Shutdown(ctx); err != nil {
						shutdownErr = errors.Join(
							shutdownErr,
							fmt.Errorf("server: shutdown component %T: %w", shutdowner, err),
						)
					}
				}
				return shutdownErr
			}, done)
		},
	})
}

func ensureInitialBackupsOnStartup(
	lc fx.Lifecycle,
	db storage.Pool,
	scheduler *services.BackupScheduler,
) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			policies, err := models.BackupPolicy.ActiveSchedules(ctx, db.Executor())
			if err != nil {
				return fmt.Errorf("load configured backup policies: %w", err)
			}

			for _, policy := range policies {
				if err := scheduler.EnsureInitial(ctx, policy.ID); err != nil {
					return fmt.Errorf("ensure initial backup for policy %s: %w", policy.ID, err)
				}
			}

			return nil
		},
	})
}

func startInBackground(
	ctx context.Context,
	name string,
	start func(context.Context) error,
) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := start(ctx); err != nil {
			slog.Error(name+" error", "error", err)
		}
	}()
	return done
}

func stopAndWait(
	ctx context.Context,
	stop func(context.Context) error,
	done <-chan struct{},
) error {
	stopErr := stop(ctx)
	select {
	case <-done:
		return stopErr
	case <-ctx.Done():
		return errors.Join(stopErr, ctx.Err())
	}
}
