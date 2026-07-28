package container

import (
	"context"
	"encoding/json"
	"fmt"

	"deploycrate-ce/internal/hostcommand"
)

type PortMapping struct {
	HostPort      int32  `json:"hostPort"`
	ContainerPort int32  `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly"`
}

type RunSpec struct {
	InstallationID string            `json:"installationId"`
	ContainerName  string            `json:"containerName"`
	ImageReference string            `json:"imageReference"`
	RestartPolicy  string            `json:"restartPolicy"`
	PortMappings   []PortMapping     `json:"portMappings"`
	VolumeMounts   []VolumeMount     `json:"volumeMounts"`
	Environment    map[string]string `json:"environment"`
}

type State struct {
	Exists         bool   `json:"exists"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	ImageReference string `json:"imageReference"`
	ImageID        string `json:"imageId"`
	Status         string `json:"status"`
	Running        bool   `json:"running"`
	Health         string `json:"health"`
	ExitCode       int    `json:"exitCode"`
	Error          string `json:"error"`
	StartedAt      string `json:"startedAt"`
	FinishedAt     string `json:"finishedAt"`
	RestartCount   int    `json:"restartCount"`
}

type Client struct{}

func New() Client { return Client{} }

func (Client) Run(ctx context.Context, spec RunSpec) error {
	payload, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("encode container run specification: %w", err)
	}
	_, err = hostcommand.RunWithInput(ctx, payload, "container-run")
	return err
}

func (Client) Inspect(ctx context.Context, installationID, containerName string) (State, error) {
	output, err := hostcommand.Run(ctx, "container-inspect", installationID, containerName)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal([]byte(output), &state); err != nil {
		return State{}, fmt.Errorf("decode container inspection: %w", err)
	}
	return state, nil
}

func (Client) Start(ctx context.Context, installationID, containerName string) error {
	_, err := hostcommand.Run(ctx, "container-start", installationID, containerName)
	return err
}

func (Client) Stop(ctx context.Context, installationID, containerName string) error {
	_, err := hostcommand.Run(ctx, "container-stop", installationID, containerName)
	return err
}

func (Client) Restart(ctx context.Context, installationID, containerName string) error {
	_, err := hostcommand.Run(ctx, "container-restart", installationID, containerName)
	return err
}

func (Client) Remove(ctx context.Context, installationID, containerName string) error {
	_, err := hostcommand.Run(ctx, "container-remove", installationID, containerName)
	return err
}
