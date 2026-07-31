package nativeinstall

import (
	"context"
	"encoding/json"
	"fmt"

	"deploycrate-ce/internal/hostcommand"
)

type InstallSpec struct {
	InstallationID        string          `json:"installationId"`
	Engine                string          `json:"engine"`
	EngineVersion         string          `json:"engineVersion"`
	PackageName           string          `json:"packageName"`
	PackageVersion        string          `json:"packageVersion,omitempty"`
	ServiceName           string          `json:"serviceName"`
	ConfigPath            string          `json:"configPath"`
	DataPath              string          `json:"dataPath"`
	Port                  int32           `json:"port"`
	AdministratorUsername string          `json:"administratorUsername"`
	AdministratorPassword string          `json:"administratorPassword"`
	Settings              json.RawMessage `json:"settings"`
}

type State struct {
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installedVersion"`
	ServiceState     string `json:"serviceState"`
	Running          bool   `json:"running"`
	Error            string `json:"error"`
}

type Client struct{}

func New() Client { return Client{} }

func (Client) Install(ctx context.Context, spec InstallSpec) error {
	payload, err := json.Marshal(spec)
	spec.AdministratorPassword = ""
	if err != nil {
		return fmt.Errorf("encode native database installation: %w", err)
	}
	defer clear(payload)
	_, err = hostcommand.RunWithInput(ctx, payload, "native-database-install")
	return err
}

func (Client) Inspect(ctx context.Context, installationID, packageName, serviceName string) (State, error) {
	output, err := hostcommand.Run(ctx, "native-database-inspect", installationID, packageName, serviceName)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal([]byte(output), &state); err != nil {
		return State{}, fmt.Errorf("decode native database state: %w", err)
	}
	return state, nil
}

func (Client) Start(ctx context.Context, installationID, serviceName string) error {
	_, err := hostcommand.Run(ctx, "native-database-start", installationID, serviceName)
	return err
}

func (Client) Stop(ctx context.Context, installationID, serviceName string) error {
	_, err := hostcommand.Run(ctx, "native-database-stop", installationID, serviceName)
	return err
}

func (Client) Restart(ctx context.Context, installationID, serviceName string) error {
	_, err := hostcommand.Run(ctx, "native-database-restart", installationID, serviceName)
	return err
}
