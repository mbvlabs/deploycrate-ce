package setup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type EventKind string

const (
	EventStarted   EventKind = "started"
	EventLog       EventKind = "log"
	EventSkipped   EventKind = "skipped"
	EventCompleted EventKind = "completed"
	EventFailed    EventKind = "failed"
	EventFinished  EventKind = "finished"
)

type Event struct {
	Kind        EventKind
	StepID      string
	Description string
	Line        string
	Index       int
	Total       int
	Err         error
}

type Reporter func(Event)

type CheckResult struct {
	Complete bool
	Detail   string
}

type Step interface {
	ID() string
	Describe(Config) string
	Check(context.Context, Config, Runtime) (CheckResult, error)
	Apply(context.Context, Config, Runtime, Reporter) error
}

type Runtime struct {
	DryRun bool
	Shell  Shell
}

type Runner struct {
	Steps []Step
	Store StateStore
	Run   Runtime
}

func (r Runner) Execute(ctx context.Context, cfg Config, report Reporter) error {
	if report == nil {
		report = func(Event) {}
	}
	state := State{Version: cfg.Version, Steps: make(map[string]StepState)}
	if !r.Run.DryRun {
		var err error
		state, err = r.Store.Load(cfg.Version)
		if err != nil {
			return err
		}
	}

	for index, step := range r.Steps {
		if current, ok := state.Steps[step.ID()]; ok && current.Status == StepCompleted {
			report(
				Event{
					Kind:        EventSkipped,
					StepID:      step.ID(),
					Description: step.Describe(cfg),
					Index:       index + 1,
					Total:       len(r.Steps),
				},
			)
			continue
		}

		check, err := step.Check(ctx, cfg, r.Run)
		if err != nil {
			return fmt.Errorf("check step %s: %w", step.ID(), err)
		}
		if check.Complete {
			state.Steps[step.ID()] = StepState{
				ID:          step.ID(),
				Status:      StepCompleted,
				CompletedAt: time.Now(),
			}
			if !r.Run.DryRun {
				if err := r.Store.Save(state); err != nil {
					return err
				}
			}
			report(
				Event{
					Kind:        EventSkipped,
					StepID:      step.ID(),
					Description: step.Describe(cfg),
					Line:        check.Detail,
					Index:       index + 1,
					Total:       len(r.Steps),
				},
			)
			continue
		}

		stepState := StepState{ID: step.ID(), Status: StepRunning, StartedAt: time.Now()}
		state.Steps[step.ID()] = stepState
		if !r.Run.DryRun {
			if err := r.Store.Save(state); err != nil {
				return err
			}
		}
		report(
			Event{
				Kind:        EventStarted,
				StepID:      step.ID(),
				Description: step.Describe(cfg),
				Index:       index + 1,
				Total:       len(r.Steps),
			},
		)

		err = step.Apply(ctx, cfg, r.Run, report)
		if err != nil {
			stepState.Status = StepFailed
			stepState.Error = redact(err.Error(), cfg.SecretValues())
			state.Steps[step.ID()] = stepState
			if !r.Run.DryRun {
				if saveErr := r.Store.Save(state); saveErr != nil {
					err = errors.Join(err, saveErr)
				}
			}
			report(
				Event{
					Kind:        EventFailed,
					StepID:      step.ID(),
					Description: step.Describe(cfg),
					Err:         err,
					Index:       index + 1,
					Total:       len(r.Steps),
				},
			)
			return fmt.Errorf("step %s failed: %w", step.ID(), err)
		}

		stepState.Status = StepCompleted
		stepState.Error = ""
		stepState.CompletedAt = time.Now()
		state.Steps[step.ID()] = stepState
		if !r.Run.DryRun {
			if err := r.Store.Save(state); err != nil {
				return err
			}
		}
		report(
			Event{
				Kind:        EventCompleted,
				StepID:      step.ID(),
				Description: step.Describe(cfg),
				Index:       index + 1,
				Total:       len(r.Steps),
			},
		)
	}

	report(Event{Kind: EventFinished, Index: len(r.Steps), Total: len(r.Steps)})
	return nil
}

type Shell struct {
	DryRun  bool
	Secrets []string
	LogPath string
	mu      *sync.Mutex
}

func NewShell(dryRun bool, secrets []string) Shell {
	_, _, stateDir := ConfigPaths()
	return Shell{
		DryRun:  dryRun,
		Secrets: secrets,
		LogPath: filepath.Join(stateDir, "install.log"),
		mu:      &sync.Mutex{},
	}
}

func (s Shell) Run(
	ctx context.Context,
	stepID string,
	script string,
	environment map[string]string,
	report Reporter,
) error {
	if s.DryRun {
		report(Event{Kind: EventLog, StepID: stepID, Line: "dry run: script execution skipped"})
		return nil
	}

	command := exec.CommandContext(ctx, "/bin/bash", "-s")
	command.Stdin = strings.NewReader(script)
	command.Env = os.Environ()
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	output, err := command.CombinedOutput()
	s.writeLog(stepID, output)

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		report(Event{Kind: EventLog, StepID: stepID, Line: redact(scanner.Text(), s.Secrets)})
	}
	if err != nil {
		return fmt.Errorf("setup script failed: %w", err)
	}
	return scanner.Err()
}

func (s Shell) Output(ctx context.Context, name string, args ...string) (string, error) {
	if s.DryRun {
		return "", nil
	}
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf(
			"%s failed: %w: %s",
			name,
			err,
			redact(strings.TrimSpace(string(output)), s.Secrets),
		)
	}
	return strings.TrimSpace(string(output)), nil
}

func (s Shell) writeLog(stepID string, output []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.LogPath), 0o700); err != nil {
		return
	}
	file, err := os.OpenFile(s.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = fmt.Fprintf(
		file,
		"\n[%s] %s\n%s",
		time.Now().Format(time.RFC3339),
		stepID,
		redact(string(output), s.Secrets),
	)
}

func redact(value string, secrets []string) string {
	for _, secret := range secrets {
		if len(secret) >= 4 {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}
