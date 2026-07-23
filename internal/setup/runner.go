package setup

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	state, err := r.Store.Load(cfg.Version)
	if err != nil {
		return err
	}

	for index, step := range r.Steps {
		if current, ok := state.Steps[step.ID()]; ok && current.Status == StepCompleted {
			report(Event{Kind: EventSkipped, StepID: step.ID(), Description: step.Describe(cfg), Index: index + 1, Total: len(r.Steps)})
			continue
		}

		check, err := step.Check(ctx, cfg, r.Run)
		if err != nil {
			return fmt.Errorf("check step %s: %w", step.ID(), err)
		}
		if check.Complete {
			state.Steps[step.ID()] = StepState{ID: step.ID(), Status: StepCompleted, CompletedAt: time.Now()}
			if !r.Run.DryRun {
				if err := r.Store.Save(state); err != nil {
					return err
				}
			}
			report(Event{Kind: EventSkipped, StepID: step.ID(), Description: step.Describe(cfg), Line: check.Detail, Index: index + 1, Total: len(r.Steps)})
			continue
		}

		stepState := StepState{ID: step.ID(), Status: StepRunning, StartedAt: time.Now()}
		state.Steps[step.ID()] = stepState
		if !r.Run.DryRun {
			if err := r.Store.Save(state); err != nil {
				return err
			}
		}
		report(Event{Kind: EventStarted, StepID: step.ID(), Description: step.Describe(cfg), Index: index + 1, Total: len(r.Steps)})

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
			report(Event{Kind: EventFailed, StepID: step.ID(), Description: step.Describe(cfg), Err: err, Index: index + 1, Total: len(r.Steps)})
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
		report(Event{Kind: EventCompleted, StepID: step.ID(), Description: step.Describe(cfg), Index: index + 1, Total: len(r.Steps)})
	}

	report(Event{Kind: EventFinished, Index: len(r.Steps), Total: len(r.Steps)})
	return nil
}

type Shell struct {
	DryRun  bool
	Secrets []string
}

func NewShell(dryRun bool, secrets []string) Shell {
	return Shell{
		DryRun:  dryRun,
		Secrets: secrets,
	}
}

func (Shell) Run(_ context.Context, stepID string, _ string, _ map[string]string, report Reporter) error {
	report(Event{Kind: EventLog, StepID: stepID, Line: "stub only: shell execution is not implemented"})
	return nil
}

func (Shell) Output(_ context.Context, name string, _ ...string) (string, error) {
	return "", fmt.Errorf("%s output is a stub and is not implemented", name)
}

func redact(value string, secrets []string) string {
	for _, secret := range secrets {
		if len(secret) >= 4 {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}
