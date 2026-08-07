package container

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"deploycrate-ce/internal/sudo"

	"github.com/google/uuid"
)

const (
	workloadDockerExecutable    = "/usr/bin/docker"
	cnbLifecycleLauncher        = "/cnb/lifecycle/launcher"
	labelApplication            = "com.deploycrate.application"
	labelEnvironment            = "com.deploycrate.environment"
	labelTarget                 = "com.deploycrate.target"
	labelDeployment             = "com.deploycrate.deployment"
	labelInstance               = "com.deploycrate.instance"
	labelRelease                = "com.deploycrate.release"
	labelProcessName            = "com.deploycrate.process.name"
	labelProcessKind            = "com.deploycrate.process.kind"
	labelProcessReplica         = "com.deploycrate.process.replica"
	labelReleaseCommand         = "com.deploycrate.release-command"
	labelNetworkOwner           = "com.deploycrate.environment"
	labelResourceInstallation   = "com.deploycrate.resource-installation"
	WorkloadLabelEnvironment    = labelEnvironment
	WorkloadLabelTarget         = labelTarget
	WorkloadLabelDeployment     = labelDeployment
	WorkloadLabelInstance       = labelInstance
	WorkloadLabelResource       = labelResourceInstallation
	WorkloadLabelNetworkOwner   = labelNetworkOwner
	WorkloadLabelProcessName    = labelProcessName
	WorkloadLabelProcessKind    = labelProcessKind
	WorkloadLabelProcessReplica = labelProcessReplica
	WorkloadLabelReleaseCommand = labelReleaseCommand
)

type WorkloadRunSpec struct {
	ApplicationID     uuid.UUID
	EnvironmentID     uuid.UUID
	TargetID          uuid.UUID
	DeploymentID      uuid.UUID
	InstanceID        uuid.UUID
	ReleaseID         uuid.UUID
	ProcessName       string
	ProcessKind       string
	ProcessReplica    string
	ContainerName     string
	ImageReference    string
	NetworkName       string
	RestartPolicy     string
	PublishAddress    string
	ContainerPort     int32
	Environment       map[string]string
	Command           []string
	DockerEnvironment []string
}

type WorkloadState struct {
	Exists         bool
	ID             string
	Name           string
	ImageReference string
	ImageID        string
	Running        bool
	Status         string
	HostAddress    string
	HostPort       int32
	Labels         map[string]string
	ExitCode       int32
	Error          string
}

type OneOffRunSpec struct {
	ApplicationID             uuid.UUID
	EnvironmentID             uuid.UUID
	TargetID                  uuid.UUID
	ReleaseID                 uuid.UUID
	ReleaseCommandExecutionID uuid.UUID
	Attempt                   int32
	ContainerName             string
	ImageReference            string
	NetworkName               string
	Environment               map[string]string
	Command                   []string
	DockerEnvironment         []string
	Stdout                    io.Writer
	Stderr                    io.Writer
	Created                   func(string) error
}

type OneOffRunResult struct {
	State            WorkloadState
	ContainerCreated bool
}

type ResourceContainerAttachment struct {
	InstallationID uuid.UUID
	ContainerName  string
}

type PreparedWorkloadRun struct {
	Arguments   []string
	Environment []string
}

type WorkloadClient struct{}

func NewWorkload() WorkloadClient { return WorkloadClient{} }

func (WorkloadClient) ReconcileNetwork(
	ctx context.Context,
	environmentID uuid.UUID,
) (string, error) {
	if environmentID == uuid.Nil {
		return "", errors.New("environment network owner is required")
	}

	name := "dc-env-" + environmentID.String()
	command := sudo.CommandContext(ctx, workloadDockerExecutable, "network", "inspect", name)
	output, err := command.Output()
	if err == nil {
		var networks []struct {
			Driver string            `json:"Driver"`
			Labels map[string]string `json:"Labels"`
		}
		if json.Unmarshal(output, &networks) != nil || len(networks) != 1 ||
			networks[0].Driver != "bridge" ||
			networks[0].Labels[labelNetworkOwner] != environmentID.String() {
			return "", errors.New("existing Environment network has invalid ownership")
		}
		return name, nil
	}

	create := sudo.CommandContext(
		ctx,
		workloadDockerExecutable,
		"network",
		"create",
		"--driver",
		"bridge",
		"--label",
		labelNetworkOwner+"="+environmentID.String(),
		name,
	)
	if output, err := create.CombinedOutput(); err != nil {
		return "", fmt.Errorf(
			"create Environment Docker network: %w: %s",
			err,
			boundedDockerMessage(output),
		)
	}

	return name, nil
}

func (WorkloadClient) ConnectResourceContainer(
	ctx context.Context,
	environmentID uuid.UUID,
	attachment ResourceContainerAttachment,
) error {
	if environmentID == uuid.Nil || attachment.InstallationID == uuid.Nil ||
		strings.TrimSpace(attachment.ContainerName) == "" ||
		strings.HasPrefix(attachment.ContainerName, "-") ||
		strings.ContainsAny(attachment.ContainerName, " \t\r\n") {
		return errors.New("Resource container attachment is invalid")
	}
	networkName := "dc-env-" + environmentID.String()
	command := sudo.CommandContext(
		ctx,
		workloadDockerExecutable,
		"container",
		"inspect",
		attachment.ContainerName,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect Resource container: %w: %s", err, boundedDockerMessage(output))
	}
	var containers []struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
		NetworkSettings struct {
			Networks map[string]json.RawMessage `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	if json.Unmarshal(output, &containers) != nil || len(containers) != 1 {
		return errors.New("Docker returned invalid Resource container state")
	}
	container := containers[0]
	if container.Config.Labels[labelResourceInstallation] != attachment.InstallationID.String() {
		return errors.New("Resource container has invalid installation ownership")
	}
	if !container.State.Running {
		return errors.New("Resource container is not running")
	}
	if _, connected := container.NetworkSettings.Networks[networkName]; connected {
		return nil
	}
	connect := sudo.CommandContext(
		ctx,
		workloadDockerExecutable,
		"network",
		"connect",
		networkName,
		attachment.ContainerName,
	)
	if output, err := connect.CombinedOutput(); err != nil {
		return fmt.Errorf(
			"connect Resource container to Environment network: %w: %s",
			err,
			boundedDockerMessage(output),
		)
	}
	return nil
}

func (WorkloadClient) PruneResourceContainers(
	ctx context.Context,
	environmentID uuid.UUID,
	keep []ResourceContainerAttachment,
) error {
	if environmentID == uuid.Nil {
		return errors.New("Environment is required")
	}
	networkName := "dc-env-" + environmentID.String()
	command := sudo.CommandContext(ctx, workloadDockerExecutable, "network", "inspect", networkName)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.ToLower(boundedDockerMessage(output))
		if strings.Contains(message, "not found") || strings.Contains(message, "no such network") {
			return nil
		}
		return fmt.Errorf("inspect Environment network: %w: %s", err, boundedDockerMessage(output))
	}
	var networks []struct {
		Containers map[string]struct {
			Name string `json:"Name"`
		} `json:"Containers"`
	}
	if json.Unmarshal(output, &networks) != nil || len(networks) != 1 {
		return errors.New("Docker returned invalid Environment network state")
	}
	retained := make(map[string]struct{}, len(keep))
	for _, attachment := range keep {
		retained[attachment.InstallationID.String()+"\x00"+attachment.ContainerName] = struct{}{}
	}
	for identifier, connected := range networks[0].Containers {
		inspect := sudo.CommandContext(
			ctx,
			workloadDockerExecutable,
			"container",
			"inspect",
			connected.Name,
		)
		containerOutput, inspectErr := inspect.CombinedOutput()
		if inspectErr != nil {
			return fmt.Errorf(
				"inspect connected Environment container: %w: %s",
				inspectErr,
				boundedDockerMessage(containerOutput),
			)
		}
		var containers []struct {
			Config struct {
				Labels map[string]string `json:"Labels"`
			} `json:"Config"`
		}
		if json.Unmarshal(containerOutput, &containers) != nil || len(containers) != 1 {
			return errors.New("Docker returned invalid connected container state")
		}
		installationID, parseErr := uuid.Parse(
			containers[0].Config.Labels[labelResourceInstallation],
		)
		if parseErr != nil {
			continue
		}
		if _, exists := retained[installationID.String()+"\x00"+connected.Name]; exists {
			continue
		}
		disconnect := sudo.CommandContext(
			ctx,
			workloadDockerExecutable,
			"network",
			"disconnect",
			"--force",
			networkName,
			identifier,
		)
		if disconnectOutput, disconnectErr := disconnect.CombinedOutput(); disconnectErr != nil {
			return fmt.Errorf(
				"disconnect stale Resource container from Environment network: %w: %s",
				disconnectErr,
				boundedDockerMessage(disconnectOutput),
			)
		}
	}
	return nil
}

func (WorkloadClient) Find(
	ctx context.Context,
	deploymentID, instanceID uuid.UUID,
) (WorkloadState, error) {
	command := sudo.CommandContext(ctx, workloadDockerExecutable, "ps", "--all", "--quiet",
		"--filter", "label="+labelDeployment+"="+deploymentID.String(),
		"--filter", "label="+labelInstance+"="+instanceID.String())
	output, err := command.CombinedOutput()
	if err != nil {
		return WorkloadState{}, fmt.Errorf(
			"find workload container: %w: %s",
			err,
			boundedDockerMessage(output),
		)
	}
	identifiers := strings.Fields(string(output))
	if len(identifiers) == 0 {
		return WorkloadState{}, nil
	}
	if len(identifiers) != 1 {
		return WorkloadState{}, errors.New(
			"multiple Docker containers claim the same Deployment and Instance",
		)
	}
	return inspectWorkload(ctx, identifiers[0])
}

func (WorkloadClient) Run(ctx context.Context, spec WorkloadRunSpec) (WorkloadState, error) {
	if existing, err := (WorkloadClient{}).Find(
		ctx,
		spec.DeploymentID,
		spec.InstanceID,
	); err != nil ||
		existing.Exists {
		return existing, err
	}
	prepared, err := PrepareWorkloadRun(spec)
	if err != nil {
		return WorkloadState{}, err
	}
	command := sudo.CommandContextPreserveEnvironment(
		ctx,
		workloadDockerExecutable,
		prepared.Arguments...)
	command.Env = prepared.Environment
	if output, err := command.CombinedOutput(); err != nil {
		return WorkloadState{}, fmt.Errorf(
			"run workload container: %w: %s",
			err,
			boundedDockerMessage(output),
		)
	}
	return (WorkloadClient{}).Find(ctx, spec.DeploymentID, spec.InstanceID)
}

func PrepareWorkloadRun(spec WorkloadRunSpec) (PreparedWorkloadRun, error) {
	publishAddress := strings.TrimSpace(spec.PublishAddress)
	if publishAddress == "" {
		publishAddress = "127.0.0.1"
	}
	if !validWorkloadPublishAddress(publishAddress) {
		return PreparedWorkloadRun{}, errors.New("workload publish address is invalid")
	}
	if spec.ApplicationID == uuid.Nil || spec.EnvironmentID == uuid.Nil ||
		spec.TargetID == uuid.Nil ||
		spec.DeploymentID == uuid.Nil ||
		spec.InstanceID == uuid.Nil ||
		spec.ReleaseID == uuid.Nil ||
		strings.TrimSpace(spec.ProcessName) == "" ||
		strings.TrimSpace(spec.ProcessReplica) == "" ||
		!strings.Contains(spec.ImageReference, "@sha256:") ||
		spec.RestartPolicy != "unless-stopped" ||
		!validWorkloadObjectName(spec.ContainerName) ||
		!validWorkloadObjectName(spec.NetworkName) ||
		!validWorkloadCommand(spec.Command, false) {
		return PreparedWorkloadRun{}, errors.New("workload Docker specification is invalid")
	}
	if spec.ProcessKind == "web" && (spec.ContainerPort < 1 || spec.ContainerPort > 65535) ||
		spec.ProcessKind == "worker" && spec.ContainerPort != 0 ||
		spec.ProcessKind != "web" && spec.ProcessKind != "worker" {
		return PreparedWorkloadRun{}, errors.New("workload process specification is invalid")
	}
	arguments := []string{"run", "--detach", "--name", spec.ContainerName,
		"--label", labelApplication + "=" + spec.ApplicationID.String(),
		"--label", labelEnvironment + "=" + spec.EnvironmentID.String(),
		"--label", labelTarget + "=" + spec.TargetID.String(),
		"--label", labelDeployment + "=" + spec.DeploymentID.String(),
		"--label", labelInstance + "=" + spec.InstanceID.String(),
		"--label", labelRelease + "=" + spec.ReleaseID.String(),
		"--label", labelProcessName + "=" + spec.ProcessName,
		"--label", labelProcessKind + "=" + spec.ProcessKind,
		"--label", labelProcessReplica + "=" + spec.ProcessReplica,
		"--network", spec.NetworkName, "--restart", spec.RestartPolicy}
	if spec.ProcessKind == "web" {
		arguments = append(
			arguments,
			"--publish",
			net.JoinHostPort(publishAddress, "")+":"+strconv.Itoa(int(spec.ContainerPort)),
		)
	}
	keys := make([]string, 0, len(spec.Environment))
	for key := range spec.Environment {
		if key == "" || strings.ContainsAny(key, "=\x00") {
			return PreparedWorkloadRun{}, errors.New("workload environment contains an invalid key")
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	childEnvironment := append([]string{}, spec.DockerEnvironment...)
	if len(childEnvironment) == 0 {
		childEnvironment = []string{
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		}
	}
	for _, key := range keys {
		if strings.ContainsRune(spec.Environment[key], '\x00') {
			return PreparedWorkloadRun{}, errors.New(
				"workload environment contains an invalid value",
			)
		}
		arguments = append(arguments, "--env", key)
		childEnvironment = append(childEnvironment, key+"="+spec.Environment[key])
	}
	if len(spec.Command) > 0 {
		arguments = append(arguments, "--entrypoint", cnbLifecycleLauncher)
	}
	arguments = append(arguments, spec.ImageReference)
	if len(spec.Command) > 0 {
		arguments = append(arguments, "--")
	}
	arguments = append(arguments, spec.Command...)
	return PreparedWorkloadRun{Arguments: arguments, Environment: childEnvironment}, nil
}

func inspectWorkload(ctx context.Context, identifier string) (WorkloadState, error) {
	command := sudo.CommandContext(ctx, workloadDockerExecutable, "inspect", identifier)
	output, err := command.CombinedOutput()
	if err != nil {
		return WorkloadState{}, fmt.Errorf(
			"inspect workload container: %w: %s",
			err,
			boundedDockerMessage(output),
		)
	}
	return ParseWorkloadInspect(output)
}

func ParseWorkloadInspect(output []byte) (WorkloadState, error) {
	var values []struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Image  string `json:"Image"`
		Config struct {
			Image  string            `json:"Image"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		State struct {
			Running  bool   `json:"Running"`
			Status   string `json:"Status"`
			ExitCode int32  `json:"ExitCode"`
			Error    string `json:"Error"`
		} `json:"State"`
		NetworkSettings struct {
			Ports map[string][]struct {
				HostIP   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			} `json:"Ports"`
		} `json:"NetworkSettings"`
	}
	if json.Unmarshal(output, &values) != nil || len(values) != 1 {
		return WorkloadState{}, errors.New("Docker returned invalid workload state")
	}
	value := values[0]
	state := WorkloadState{
		Exists:         true,
		ID:             value.ID,
		Name:           strings.TrimPrefix(value.Name, "/"),
		ImageReference: value.Config.Image,
		ImageID:        value.Image,
		Running:        value.State.Running,
		Status:         value.State.Status,
		Labels:         value.Config.Labels,
		ExitCode:       value.State.ExitCode,
		Error:          value.State.Error,
	}
	for _, bindings := range value.NetworkSettings.Ports {
		for _, binding := range bindings {
			if !validWorkloadPublishAddress(binding.HostIP) {
				continue
			}
			port, parseErr := strconv.Atoi(binding.HostPort)
			if parseErr == nil && port > 0 && port <= 65535 {
				state.HostAddress = binding.HostIP
				state.HostPort = int32(port)
			}
		}
	}
	return state, nil
}

func (WorkloadClient) RunOneOff(ctx context.Context, spec OneOffRunSpec) (OneOffRunResult, error) {
	prepared, err := PrepareOneOffCreate(spec)
	if err != nil {
		return OneOffRunResult{}, err
	}
	create := sudo.CommandContextPreserveEnvironment(
		ctx,
		workloadDockerExecutable,
		prepared.Arguments...)
	create.Env = prepared.Environment
	output, err := create.CombinedOutput()
	if err != nil {
		return OneOffRunResult{}, fmt.Errorf(
			"create one-off workload container: %w: %s",
			err,
			boundedDockerMessage(output),
		)
	}
	identifier := strings.TrimSpace(string(output))
	result := OneOffRunResult{
		ContainerCreated: true,
		State:            WorkloadState{Exists: true, ID: identifier, Name: spec.ContainerName},
	}
	if spec.Created != nil {
		if err := spec.Created(identifier); err != nil {
			return result, err
		}
	}
	start := sudo.CommandContext(ctx, workloadDockerExecutable, "start", "--attach", identifier)
	start.Stdout = spec.Stdout
	start.Stderr = spec.Stderr
	startErr := start.Run()
	state, inspectErr := inspectWorkload(context.WithoutCancel(ctx), identifier)
	if inspectErr == nil {
		result.State = state
	}
	if startErr != nil {
		return result, fmt.Errorf("run one-off workload container: %w", startErr)
	}
	return result, inspectErr
}

func PrepareOneOffCreate(spec OneOffRunSpec) (PreparedWorkloadRun, error) {
	if spec.ApplicationID == uuid.Nil || spec.EnvironmentID == uuid.Nil ||
		spec.TargetID == uuid.Nil ||
		spec.ReleaseID == uuid.Nil ||
		spec.ReleaseCommandExecutionID == uuid.Nil ||
		spec.Attempt < 1 ||
		!strings.Contains(spec.ImageReference, "@sha256:") ||
		!validWorkloadObjectName(spec.ContainerName) ||
		!validWorkloadObjectName(spec.NetworkName) ||
		!validWorkloadCommand(spec.Command, true) {
		return PreparedWorkloadRun{}, errors.New("one-off workload Docker specification is invalid")
	}
	arguments := []string{
		"create",
		"--name",
		spec.ContainerName,
		"--label",
		labelApplication + "=" + spec.ApplicationID.String(),
		"--label",
		labelEnvironment + "=" + spec.EnvironmentID.String(),
		"--label",
		labelTarget + "=" + spec.TargetID.String(),
		"--label",
		labelRelease + "=" + spec.ReleaseID.String(),
		"--label",
		labelReleaseCommand + "=" + spec.ReleaseCommandExecutionID.String(),
		"--label",
		labelProcessName + "=release",
		"--label",
		labelProcessKind + "=release",
		"--label",
		labelProcessReplica + "=" + strconv.Itoa(int(spec.Attempt)),
		"--network",
		spec.NetworkName,
	}
	keys := make([]string, 0, len(spec.Environment))
	for key := range spec.Environment {
		if key == "" || strings.ContainsAny(key, "=\x00") {
			return PreparedWorkloadRun{}, errors.New(
				"one-off workload environment contains an invalid key",
			)
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	childEnvironment := append([]string{}, spec.DockerEnvironment...)
	if len(childEnvironment) == 0 {
		childEnvironment = []string{
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		}
	}
	for _, key := range keys {
		if strings.ContainsRune(spec.Environment[key], '\x00') {
			return PreparedWorkloadRun{}, errors.New(
				"one-off workload environment contains an invalid value",
			)
		}
		arguments = append(arguments, "--env", key)
		childEnvironment = append(childEnvironment, key+"="+spec.Environment[key])
	}
	if len(spec.Command) > 0 {
		arguments = append(arguments, "--entrypoint", cnbLifecycleLauncher)
	}
	arguments = append(arguments, spec.ImageReference)
	if len(spec.Command) > 0 {
		arguments = append(arguments, "--")
	}
	arguments = append(arguments, spec.Command...)
	return PreparedWorkloadRun{Arguments: arguments, Environment: childEnvironment}, nil
}

func (WorkloadClient) RemoveOneOff(ctx context.Context, identifier string) error {
	if strings.TrimSpace(identifier) == "" || strings.HasPrefix(identifier, "-") {
		return errors.New("one-off workload container identity is invalid")
	}
	command := sudo.CommandContext(ctx, workloadDockerExecutable, "rm", "--force", identifier)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf(
			"remove one-off workload container: %w: %s",
			err,
			boundedDockerMessage(output),
		)
	}
	return nil
}

func (WorkloadClient) FindEnvironment(
	ctx context.Context,
	environmentID uuid.UUID,
) ([]WorkloadState, error) {
	if environmentID == uuid.Nil {
		return nil, errors.New("Environment is required")
	}
	command := sudo.CommandContext(
		ctx,
		workloadDockerExecutable,
		"ps",
		"--all",
		"--quiet",
		"--filter",
		"label="+labelEnvironment+"="+environmentID.String(),
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf(
			"find Environment workload containers: %w: %s",
			err,
			boundedDockerMessage(output),
		)
	}
	identifiers := strings.Fields(string(output))
	states := make([]WorkloadState, 0, len(identifiers))
	for _, identifier := range identifiers {
		state, inspectErr := inspectWorkload(ctx, identifier)
		if inspectErr != nil {
			return nil, inspectErr
		}
		states = append(states, state)
	}
	return states, nil
}

func (WorkloadClient) Remove(ctx context.Context, deploymentID, instanceID uuid.UUID) error {
	state, err := (WorkloadClient{}).Find(ctx, deploymentID, instanceID)
	if err != nil || !state.Exists {
		return err
	}
	command := sudo.CommandContext(ctx, workloadDockerExecutable, "rm", "--force", state.ID)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("remove workload container: %w: %s", err, boundedDockerMessage(output))
	}
	return nil
}

func (WorkloadClient) Start(
	ctx context.Context,
	deploymentID, instanceID uuid.UUID,
) (WorkloadState, error) {
	state, err := (WorkloadClient{}).Find(ctx, deploymentID, instanceID)
	if err != nil || !state.Exists || state.Running {
		return state, err
	}
	command := sudo.CommandContext(ctx, workloadDockerExecutable, "start", state.ID)
	if output, err := command.CombinedOutput(); err != nil {
		return WorkloadState{}, fmt.Errorf(
			"start workload container: %w: %s",
			err,
			boundedDockerMessage(output),
		)
	}
	return (WorkloadClient{}).Find(ctx, deploymentID, instanceID)
}

func (WorkloadClient) Restart(ctx context.Context, deploymentID, instanceID uuid.UUID) (WorkloadState, error) {
	state, err := (WorkloadClient{}).Find(ctx, deploymentID, instanceID)
	if err != nil || !state.Exists {
		return state, err
	}
	command := sudo.CommandContext(ctx, workloadDockerExecutable, "restart", state.ID)
	if output, err := command.CombinedOutput(); err != nil {
		return WorkloadState{}, fmt.Errorf("restart workload container: %w: %s", err, boundedDockerMessage(output))
	}
	return (WorkloadClient{}).Find(ctx, deploymentID, instanceID)
}

func (WorkloadClient) DeleteEnvironment(ctx context.Context, environmentID uuid.UUID) error {
	if environmentID == uuid.Nil {
		return errors.New("Environment is required")
	}
	containers := sudo.CommandContext(
		ctx,
		workloadDockerExecutable,
		"ps",
		"--all",
		"--quiet",
		"--filter",
		"label="+labelEnvironment+"="+environmentID.String(),
	)
	output, err := containers.CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"find Environment workload containers: %w: %s",
			err,
			boundedDockerMessage(output),
		)
	}
	identifiers := strings.Fields(string(output))
	if len(identifiers) > 0 {
		arguments := append([]string{"rm", "--force"}, identifiers...)
		remove := sudo.CommandContext(ctx, workloadDockerExecutable, arguments...)
		if output, err := remove.CombinedOutput(); err != nil {
			return fmt.Errorf(
				"remove Environment workload containers: %w: %s",
				err,
				boundedDockerMessage(output),
			)
		}
	}
	if err := (WorkloadClient{}).PruneResourceContainers(ctx, environmentID, nil); err != nil {
		return err
	}
	networkName := "dc-env-" + environmentID.String()
	removeNetwork := sudo.CommandContext(
		ctx,
		workloadDockerExecutable,
		"network",
		"rm",
		networkName,
	)
	if output, err := removeNetwork.CombinedOutput(); err != nil {
		message := boundedDockerMessage(output)
		if !strings.Contains(strings.ToLower(message), "not found") &&
			!strings.Contains(strings.ToLower(message), "no such network") {
			return fmt.Errorf("remove Environment Docker network: %w: %s", err, message)
		}
	}
	return nil
}

func boundedDockerMessage(output []byte) string {
	message := strings.TrimSpace(string(output))
	if len(message) > 2048 {
		message = message[:2048]
	}
	return message
}

func validWorkloadPublishAddress(value string) bool {
	address, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || !address.Is4() {
		return false
	}
	return address.IsLoopback() || netip.MustParsePrefix("10.99.0.0/16").Contains(address)
}

func validWorkloadObjectName(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.HasPrefix(value, "-") &&
		!strings.ContainsAny(value, " \t\r\n\x00")
}

func validWorkloadCommand(command []string, requireExecutable bool) bool {
	if requireExecutable && (len(command) == 0 || command[0] == "") {
		return false
	}
	for _, argument := range command {
		if strings.ContainsRune(argument, '\x00') {
			return false
		}
	}
	return true
}
