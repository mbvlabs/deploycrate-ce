package main

import (
	"context"
	"errors"
	"fmt"
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

func main() {
	if len(os.Args) > 1 {
		if err := runCommand(context.Background(), os.Args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			os.Exit(1)
		}
		return
	}

	rootHTML, err := assets.Files.ReadFile("inertia/root.go.html")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read Inertia root template: %s\n", err)
		os.Exit(1)
	}
	viteManifest, err := assets.Files.ReadFile("dist/vite/manifest.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read Vite manifest: %s\n", err)
		os.Exit(1)
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
		fmt.Fprintf(os.Stderr, "failed to initialize inertia: %s\n", err)
		os.Exit(1)
	}
	app := fx.New(
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
		queue.WorkersModule,
		services.Module,
		controllers.Module,
		router.Module,

		fx.Invoke(runMigrationsOnStartup),
		fx.Invoke(reconcileWorkloadsOnStartup),
		fx.Invoke(startQueueProcessor),
		fx.Invoke(startServer),
		fx.Invoke(ensureInitialBackupsOnStartup),
	)

	if err := app.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	if err := app.Stop(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func runCommand(ctx context.Context, arguments []string) error {
	switch arguments[0] {
	case "version":
		if len(arguments) != 1 {
			return errors.New("usage: deploycrate-ce version")
		}
		fmt.Println(appVersion)
		return nil
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
		fmt.Printf("activated %d backup policies\n", activated)
		return nil
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
