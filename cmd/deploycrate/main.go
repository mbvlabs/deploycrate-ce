package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"deploycrate-ce/internal/setup"
	setupui "deploycrate-ce/internal/setup/ui"
)

var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "deploycrate:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printHelp(stdout)
		return nil
	}

	switch args[0] {
	case "install":
		return install(ctx, args[1:])
	case "resume":
		return resume(ctx, args[1:], stdout)
	case "doctor":
		return doctor(ctx, args[1:], stdout)
	case "logs":
		return logs(args[1:], stdout)
	case "backup":
		return stubCommand(args[1:], "backup")
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version)
		return nil
	case "help", "--help", "-h":
		printHelp(stdout)
		return nil
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printHelp(stderr)
		return errors.New("unknown command")
	}
}

func install(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	dryRun := flags.Bool("dry-run", false, "show the complete flow without mutating the host")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !isTerminal(os.Stdin) {
		return errors.New("interactive installation requires a TTY; run sudo deploycrate install from a terminal")
	}
	host, err := setup.Preflight(ctx, *dryRun)
	if err != nil {
		return err
	}
	lock, err := setup.AcquireInstallLock(*dryRun)
	if err != nil {
		return err
	}
	defer lock.Close()
	if !*dryRun {
		status, err := setup.InspectInstallation()
		if err != nil {
			return err
		}
		switch status {
		case setup.InstallationFresh:
		case setup.InstallationResumable, setup.InstallationCleanupPending:
			return errors.New("an installation is already configured; run sudo deploycrate resume")
		case setup.InstallationComplete:
			return errors.New("DeployCrate CE is already installed")
		default:
			return errors.New("installer state is inconsistent; inspect /etc/deploycrate-ce and /var/lib/deploycrate-ce before continuing")
		}
	}

	cfg, err := setup.NewConfig(version)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(setupui.NewModel(cfg, host, *dryRun)).Run()
	return err
}

func resume(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("resume", flag.ContinueOnError)
	dryRun := flags.Bool("dry-run", false, "inspect resume behavior without mutating the host")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !isTerminal(os.Stdin) {
		return errors.New("interactive resume requires a TTY; run sudo deploycrate resume from a terminal")
	}
	if _, err := setup.Preflight(ctx, *dryRun); err != nil {
		return err
	}
	lock, err := setup.AcquireInstallLock(*dryRun)
	if err != nil {
		return err
	}
	defer lock.Close()
	status, err := setup.InspectInstallation()
	if err != nil {
		return err
	}
	switch status {
	case setup.InstallationResumable, setup.InstallationCleanupPending:
	case setup.InstallationFresh:
		return errors.New("no saved installation is available to resume")
	case setup.InstallationComplete:
		return errors.New("DeployCrate CE installation is already complete")
	default:
		return errors.New("installer state is inconsistent; restore the saved installer configuration before resuming")
	}
	cfg, err := setup.LoadConfig()
	if err != nil {
		return err
	}
	if err := setup.NewRunner(cfg, *dryRun).Execute(ctx, cfg, func(event setup.Event) {
		switch event.Kind {
		case setup.EventStarted:
			fmt.Fprintf(stdout, "[%d/%d] %s\n", event.Index, event.Total, event.Description)
		case setup.EventLog:
			fmt.Fprintln(stdout, "  "+event.Line)
		case setup.EventCompleted:
			fmt.Fprintln(stdout, "  complete")
		case setup.EventSkipped:
			fmt.Fprintf(stdout, "[%d/%d] %s: already complete\n", event.Index, event.Total, event.Description)
		}
	}); err != nil {
		return err
	}
	_, err = tea.NewProgram(setupui.NewHandoffModel(cfg, *dryRun, status == setup.InstallationCleanupPending)).Run()
	return err
}

func doctor(_ context.Context, args []string, _ io.Writer) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	_ = flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	return errors.New("doctor is a stub until runtime health checks are implemented")
}

func stubCommand(args []string, name string) error {
	if len(args) != 0 {
		return fmt.Errorf("%s does not accept arguments yet", name)
	}
	return fmt.Errorf("%s is a stub and is not implemented yet", name)
}

func logs(args []string, stdout io.Writer) error {
	if len(args) != 0 {
		return errors.New("logs does not accept arguments")
	}
	file, err := os.Open(setup.NewShell(false, nil).LogPath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(stdout, file)
	return err
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func printHelp(writer io.Writer) {
	fmt.Fprintln(writer, `DeployCrate CE server bootstrap

Usage:
  deploycrate install [--dry-run]
  deploycrate resume [--dry-run]
  deploycrate doctor [--json]
  deploycrate logs
  deploycrate backup
  deploycrate version`)
}
