package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
)

type StepState struct {
	ID          string     `json:"id"`
	Status      StepStatus `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt time.Time  `json:"completed_at"`
	Error       string     `json:"error,omitempty"`
}

type State struct {
	Version   string               `json:"version"`
	UpdatedAt time.Time            `json:"updated_at"`
	Steps     map[string]StepState `json:"steps"`
}

type StateStore struct {
	path string
}

func NewStateStore() StateStore {
	_, _, stateDir := ConfigPaths()
	return StateStore{path: filepath.Join(stateDir, "install-state.json")}
}

func (s StateStore) Path() string {
	return s.path
}

func (s StateStore) Load(version string) (State, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return State{Version: version, Steps: make(map[string]StepState)}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read installer state: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode installer state: %w", err)
	}
	if state.Steps == nil {
		state.Steps = make(map[string]StepState)
	}
	return state, nil
}

func (s StateStore) Save(state State) error {
	state.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode installer state: %w", err)
	}
	return writeProtectedFile(s.path, data, 0o600)
}
