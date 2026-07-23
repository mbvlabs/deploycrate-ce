package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	mailclients "deploycrate-ce/clients/email"
	"deploycrate-ce/config"
	"deploycrate-ce/controllers"
	"deploycrate-ce/database"
	"deploycrate-ce/email"
	"deploycrate-ce/internal/inertia"
	"deploycrate-ce/internal/server"
	"deploycrate-ce/queue"
	"deploycrate-ce/router"
	"deploycrate-ce/services"
	"deploycrate-ce/telemetry"

	"go.uber.org/fx"
)

var appVersion = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := inertia.Init("inertia/root.go.html"); err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize inertia: %s\n", err)
		os.Exit(1)
	}
	app := fx.New(
		fx.Provide(
			func() context.Context { return ctx },
			func() services.CurrentVersion { return services.CurrentVersion(appVersion) },
			func(cfg config.Config) (email.TransactionalSender, email.MarketingSender) {
				if config.Env == server.ProdEnvironment {
					log.Fatal("provide real email sender")
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

		fx.Invoke(startQueueProcessor),
		fx.Invoke(startServer),
	)

	if err := app.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.Stop(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func startQueueProcessor(lc fx.Lifecycle, appCtx context.Context, p queue.Processor) {
	var done <-chan struct{}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			done = startInBackground(appCtx, "queue processor", p.Start)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return stopAndWait(ctx, p.Stop, done)
		},
	})
}

func startServer(lc fx.Lifecycle, appCtx context.Context, r *router.Router, cfg config.Config) {
	srv := server.New(
		appCtx,
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
						shutdownErr = errors.Join(shutdownErr, fmt.Errorf("server: shutdown component %T: %w", shutdowner, err))
					}
				}
				return shutdownErr
			}, done)
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
