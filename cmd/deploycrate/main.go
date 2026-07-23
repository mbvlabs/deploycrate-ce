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
		return stubCommand(args[1:], "logs")
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
	host, err := setup.Preflight(ctx, true)
	if err != nil {
		return err
	}

	cfg, err := setup.NewConfig(version)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(setupui.NewModel(cfg, host, *dryRun)).Run()
	return err
}

func resume(_ context.Context, args []string, _ io.Writer) error {
	flags := flag.NewFlagSet("resume", flag.ContinueOnError)
	_ = flags.Bool("dry-run", false, "inspect resume behavior without mutating the host")
	if err := flags.Parse(args); err != nil {
		return err
	}
	return errors.New("resume is a stub until installer state persistence is implemented")
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
