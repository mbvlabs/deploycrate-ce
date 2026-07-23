package setup

import (
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
	return State{Version: version, Steps: make(map[string]StepState)}, nil
}

func (StateStore) Save(State) error {
	return nil
}
