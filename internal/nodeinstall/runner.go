package nodeinstall

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

const stateDirectory = "/var/lib/deploycrate-node"

//go:embed scripts/node-install.sh
var installationScript string

func Install(ctx context.Context, manifest Manifest) (Result, error) {
	if os.Geteuid() != 0 {
		return Result{}, errors.New("node installation must run as root")
	}
	if err := os.MkdirAll(stateDirectory, 0o700); err != nil {
		return Result{}, fmt.Errorf("create node installation state directory: %w", err)
	}
	lock, err := os.OpenFile(stateDirectory+"/install.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return Result{}, fmt.Errorf("open node installation lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return Result{}, errors.New("another node installation is already running")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	nodeNameYAML, err := json.Marshal(manifest.NodeName)
	if err != nil {
		return Result{}, fmt.Errorf("encode node name for telemetry configuration: %w", err)
	}
	peerConfiguration, err := manifest.PeerConfiguration()
	if err != nil {
		return Result{}, fmt.Errorf("build Node WireGuard peers: %w", err)
	}

	command := exec.CommandContext(ctx, "/usr/bin/env", "bash", "-s")
	command.Stdin = strings.NewReader(installationScript)
	command.Env = append(os.Environ(),
		"DEPLOYCRATE_SERVER_ID="+manifest.ServerID,
		"DEPLOYCRATE_NODE_NAME="+manifest.NodeName,
		"DEPLOYCRATE_NODE_NAME_YAML="+string(nodeNameYAML),
		"DEPLOYCRATE_PRIVATE_ADDRESS="+manifest.PrivateAddress,
		fmt.Sprintf("DEPLOYCRATE_WIREGUARD_PORT=%d", manifest.ListenPort),
		fmt.Sprintf("DEPLOYCRATE_SSH_PORT=%d", manifest.SSHPort),
		"DEPLOYCRATE_CONTROL_PUBLIC_KEY="+manifest.ControlPlanePublicKey,
		"DEPLOYCRATE_CONTROL_ADDRESS="+manifest.ControlPlaneAddress,
		"DEPLOYCRATE_CONTROL_ENDPOINT="+manifest.ControlPlaneEndpoint,
		"DEPLOYCRATE_WIREGUARD_PEERS="+base64.StdEncoding.EncodeToString([]byte(peerConfiguration)),
		"DEPLOYCRATE_SSH_USER_CA="+manifest.SSHUserCAPublicKey,
		"DEPLOYCRATE_OTLP_ENDPOINT="+manifest.OTLPEndpoint,
		fmt.Sprintf("DEPLOYCRATE_CAPABILITY_BUILD=%t", manifest.Capabilities.Build),
		fmt.Sprintf("DEPLOYCRATE_CAPABILITY_RUNTIME=%t", manifest.Capabilities.Runtime),
		fmt.Sprintf("DEPLOYCRATE_CAPABILITY_RESOURCE=%t", manifest.Capabilities.Resource),
		fmt.Sprintf("DEPLOYCRATE_CAPABILITY_DATABASE=%t", manifest.Capabilities.Database),
		fmt.Sprintf("DEPLOYCRATE_CAPABILITY_REPOSITORY=%t", manifest.Capabilities.Repository),
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return Result{}, fmt.Errorf("install worker node: %s", message)
	}
	var scriptResult struct {
		WireGuardPublicKey  string `json:"wireguard_public_key"`
		SSHHostPublicKey    string `json:"ssh_host_public_key"`
		OperatingSystem     string `json:"operating_system"`
		Distribution        string `json:"distribution"`
		DistributionVersion string `json:"distribution_version"`
		Architecture        string `json:"architecture"`
	}
	lines := bytes.Split(bytes.TrimSpace(stdout.Bytes()), []byte("\n"))
	if len(lines) == 0 {
		return Result{}, errors.New("node installation did not return a result")
	}
	if err := json.Unmarshal(bytes.TrimSpace(lines[len(lines)-1]), &scriptResult); err != nil {
		return Result{}, fmt.Errorf("decode node installation result: %w", err)
	}
	result := Result{
		ServerID: manifest.ServerID, WireGuardPublicKey: scriptResult.WireGuardPublicKey,
		SSHHostPublicKey: scriptResult.SSHHostPublicKey, OperatingSystem: scriptResult.OperatingSystem,
		Distribution: scriptResult.Distribution, DistributionVersion: scriptResult.DistributionVersion,
		Architecture: scriptResult.Architecture, Capabilities: manifest.Capabilities,
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(stateDirectory+"/install-state.json", encoded, 0o600); err != nil {
		return Result{}, fmt.Errorf("save node installation state: %w", err)
	}
	return result, nil
}
