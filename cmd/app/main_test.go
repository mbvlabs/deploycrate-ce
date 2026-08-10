package main

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/fx"
)

func TestApplicationDependencyGraph(t *testing.T) {
	options := append(appOptions(t.Context()), fx.NopLogger)
	if err := fx.ValidateApp(options...); err != nil {
		t.Fatalf("validate application dependency graph: %v", err)
	}
}

func TestRunVersion(t *testing.T) {
	originalVersion := appVersion
	appVersion = "test-version"
	t.Cleanup(func() { appVersion = originalVersion })

	var stdout bytes.Buffer
	if err := run([]string{"deploycrate-ce", "version"}, &stdout); err != nil {
		t.Fatalf("run version: %v", err)
	}
	if got, want := stdout.String(), "test-version\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestBackgroundLifecycleStopsOnceAndWaitsForExit(t *testing.T) {
	appCtx := t.Context()
	started := make(chan struct{})
	release := make(chan struct{})
	done := startInBackground(appCtx, "test worker", func(ctx context.Context) error {
		if ctx != appCtx {
			t.Errorf("worker context does not match application context")
		}
		close(started)
		<-release
		return nil
	})
	<-started

	var stopCalls atomic.Int32
	stop := func(context.Context) error {
		if stopCalls.Add(1) == 1 {
			close(release)
		}
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := stopAndWait(shutdownCtx, stop, done); err != nil {
		t.Fatalf("stopAndWait: %v", err)
	}
	if got := stopCalls.Load(); got != 1 {
		t.Fatalf("stop calls = %d, want 1", got)
	}
	select {
	case <-done:
	default:
		t.Fatal("background worker still running after shutdown")
	}
}
