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

	"deploycrate-ce/internal/resourceaccess"
	"deploycrate-ce/internal/setup"
	setupui "deploycrate-ce/internal/setup/ui"
)

var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "bootstrap:", err)
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
	case "logs":
		return logs(args[1:], stdout)
	case "ssh-ca":
		return sshCA(args[1:], stdout)
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version)
		return nil
	case "help", "--help", "-h":
		printHelp(stdout)
		return nil
	case "host-resource-access":
		return resourceaccess.RunHostCommand(args[1:])
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printHelp(stderr)
		return errors.New("unknown command")
	}
}

func sshCA(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "recover" {
		return errors.New("usage: bootstrap ssh-ca recover --bundle PATH --passphrase-file PATH")
	}
	flags := flag.NewFlagSet("ssh-ca recover", flag.ContinueOnError)
	bundlePath := flags.String("bundle", "", "path to deploycrate-ssh-ca-recovery-v1.age")
	passphrasePath := flags.String("passphrase-file", "", "path to a file containing the age passphrase")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *bundlePath == "" || *passphrasePath == "" || flags.NArg() != 0 {
		return errors.New("usage: bootstrap ssh-ca recover --bundle PATH --passphrase-file PATH")
	}
	passphrase, err := os.ReadFile(*passphrasePath)
	if err != nil {
		return fmt.Errorf("read SSH CA recovery passphrase: %w", err)
	}
	if err := setup.RecoverSSHCA(*bundlePath, string(passphrase)); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "SSH user and host CA keys restored and verified")
	return nil
}

func install(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	dryRun := flags.Bool("dry-run", false, "show the complete flow without mutating the host")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !isTerminal(os.Stdin) {
		return errors.New(
			"interactive installation requires a TTY; run sudo bootstrap install from a terminal",
		)
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
			return errors.New("an installation is already configured; run sudo bootstrap resume")
		case setup.InstallationComplete:
			return errors.New("DeployCrate CE is already installed")
		default:
			return errors.New(
				"installer state is inconsistent; inspect /etc/deploycrate-ce and /var/lib/deploycrate-ce before continuing",
			)
		}
	}

	cfg, err := setup.NewConfig(version)
	if err != nil {
		return err
	}
	cfg.PublicIPv4 = host.PublicIPv4
	_, err = tea.NewProgram(setupui.NewModel(cfg, host, *dryRun, newSetupOperations())).Run()
	return err
}

func resume(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("resume", flag.ContinueOnError)
	dryRun := flags.Bool("dry-run", false, "inspect resume behavior without mutating the host")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !isTerminal(os.Stdin) {
		return errors.New(
			"interactive resume requires a TTY; run sudo bootstrap resume from a terminal",
		)
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
		return errors.New(
			"installer state is inconsistent; restore the saved installer configuration before resuming",
		)
	}
	cfg, err := setup.LoadConfig()
	if err != nil {
		return err
	}
	if err := setup.NewRunner(cfg, *dryRun, newSetupOperations()).Execute(ctx, cfg, func(event setup.Event) {
		switch event.Kind {
		case setup.EventStarted:
			fmt.Fprintf(stdout, "[%d/%d] %s\n", event.Index, event.Total, event.Description)
		case setup.EventLog:
			fmt.Fprintln(stdout, "  "+event.Line)
		case setup.EventCompleted:
			fmt.Fprintln(stdout, "  complete")
		case setup.EventSkipped:
			fmt.Fprintf(
				stdout,
				"[%d/%d] %s: already complete\n",
				event.Index,
				event.Total,
				event.Description,
			)
		}
	}); err != nil {
		return err
	}
	_, err = tea.NewProgram(setupui.NewHandoffModel(cfg, *dryRun, status == setup.InstallationCleanupPending)).
		Run()
	return err
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
  bootstrap install [--dry-run]
  bootstrap resume [--dry-run]
  bootstrap ssh-ca recover --bundle PATH --passphrase-file PATH
  bootstrap logs
  bootstrap version`)
}
