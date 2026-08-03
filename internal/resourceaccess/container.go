package resourceaccess

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	dockerExecutable            = "/usr/bin/docker"
	installationLabel           = "com.deploycrate.resource-installation"
	maximumContainerInputLength = 64 * 1024
	maximumContainerLogLength   = 64 * 1024
	maximumContainerLogTail     = 500
)

var (
	containerNamePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	volumeNamePattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	environmentKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

type containerRunSpec struct {
	InstallationID string                 `json:"installationId"`
	ContainerName  string                 `json:"containerName"`
	ImageReference string                 `json:"imageReference"`
	RestartPolicy  string                 `json:"restartPolicy"`
	PortMappings   []containerPortMapping `json:"portMappings"`
	VolumeMounts   []containerVolumeMount `json:"volumeMounts"`
	Environment    map[string]string      `json:"environment"`
}

type containerExecSpec struct {
	InstallationID string            `json:"installationId"`
	ContainerName  string            `json:"containerName"`
	Executable     string            `json:"executable"`
	Arguments      []string          `json:"arguments"`
	Environment    map[string]string `json:"environment"`
}

type containerPortMapping struct {
	HostPort      int32  `json:"hostPort"`
	ContainerPort int32  `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

type containerVolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly"`
}

type containerInspection struct {
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

type dockerInspection struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	Image        string `json:"Image"`
	RestartCount int    `json:"RestartCount"`
	Config       struct {
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Status     string `json:"Status"`
		Running    bool   `json:"Running"`
		ExitCode   int    `json:"ExitCode"`
		Error      string `json:"Error"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
		Health     *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
}

func runContainer(input io.Reader) error {
	decoder := json.NewDecoder(io.LimitReader(input, maximumContainerInputLength))
	decoder.DisallowUnknownFields()
	var spec containerRunSpec
	if err := decoder.Decode(&spec); err != nil {
		return fmt.Errorf("decode container run specification: %w", err)
	}
	if err := validateContainerRunSpec(spec); err != nil {
		return err
	}

	installationID := uuid.MustParse(spec.InstallationID).String()
	label, exists, err := inspectContainerLabel(spec.ContainerName)
	if err != nil {
		return err
	}
	if exists {
		if label != installationID {
			return fmt.Errorf("container %q already exists and is not owned by this Resource installation", spec.ContainerName)
		}
		running, err := inspectContainerRunning(spec.ContainerName)
		if err != nil {
			return err
		}
		if running {
			return nil
		}
		return run(dockerExecutable, "start", spec.ContainerName)
	}

	for _, mount := range spec.VolumeMounts {
		if err := run(dockerExecutable, "volume", "create", mount.Name); err != nil {
			return err
		}
	}

	arguments := []string{
		"run", "--detach",
		"--name", spec.ContainerName,
		"--label", installationLabel + "=" + installationID,
		"--restart", spec.RestartPolicy,
	}
	for _, mapping := range spec.PortMappings {
		published := "127.0.0.1:" + strconv.Itoa(int(mapping.HostPort)) + ":" + strconv.Itoa(int(mapping.ContainerPort)) + "/" + mapping.Protocol
		arguments = append(arguments, "--publish", published)
	}
	for _, mount := range spec.VolumeMounts {
		value := "type=volume,source=" + mount.Name + ",target=" + mount.MountPath
		if mount.ReadOnly {
			value += ",readonly"
		}
		arguments = append(arguments, "--mount", value)
	}
	environmentKeys := make([]string, 0, len(spec.Environment))
	for key := range spec.Environment {
		environmentKeys = append(environmentKeys, key)
	}
	sort.Strings(environmentKeys)
	for _, key := range environmentKeys {
		arguments = append(arguments, "--env", key)
	}
	arguments = append(arguments, spec.ImageReference)

	command := exec.Command(dockerExecutable, arguments...)
	command.Env = os.Environ()
	for _, key := range environmentKeys {
		command.Env = append(command.Env, key+"="+spec.Environment[key])
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run %s: %w: %s", dockerExecutable, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func validateContainerRunSpec(spec containerRunSpec) error {
	if _, err := uuid.Parse(spec.InstallationID); err != nil {
		return errors.New("installation ID must be a UUID")
	}
	if !containerNamePattern.MatchString(spec.ContainerName) || len(spec.ContainerName) > 128 {
		return errors.New("container name is invalid")
	}
	if strings.TrimSpace(spec.ImageReference) != spec.ImageReference || spec.ImageReference == "" || strings.HasPrefix(spec.ImageReference, "-") || strings.ContainsAny(spec.ImageReference, " \t\r\n") {
		return errors.New("image reference is invalid")
	}
	if !slices.Contains([]string{"no", "always", "on-failure", "unless-stopped"}, spec.RestartPolicy) {
		return errors.New("restart policy is invalid")
	}
	if len(spec.PortMappings) > 32 {
		return errors.New("too many port mappings")
	}
	for _, mapping := range spec.PortMappings {
		if mapping.HostPort < 1 || mapping.HostPort > 65535 || mapping.ContainerPort < 1 || mapping.ContainerPort > 65535 {
			return errors.New("container port mapping is invalid")
		}
		if mapping.Protocol != "tcp" && mapping.Protocol != "udp" {
			return errors.New("container port protocol is invalid")
		}
	}
	if len(spec.VolumeMounts) > 32 {
		return errors.New("too many volume mounts")
	}
	for _, mount := range spec.VolumeMounts {
		if !volumeNamePattern.MatchString(mount.Name) || len(mount.Name) > 255 {
			return errors.New("Docker volume name is invalid")
		}
		if !filepath.IsAbs(mount.MountPath) || filepath.Clean(mount.MountPath) != mount.MountPath || strings.Contains(mount.MountPath, ",") {
			return errors.New("container mount path is invalid")
		}
	}
	if len(spec.Environment) > 64 {
		return errors.New("too many container environment variables")
	}
	for key := range spec.Environment {
		if !environmentKeyPattern.MatchString(key) {
			return errors.New("container environment variable name is invalid")
		}
	}
	return nil
}

func execContainer(input io.Reader, output io.Writer) error {
	reader := bufio.NewReaderSize(input, maximumContainerInputLength+1)
	header, err := reader.ReadBytes('\n')
	if err != nil {
		return errors.New("read container execution specification")
	}
	if len(header) > maximumContainerInputLength {
		return errors.New("container execution specification is too large")
	}
	defer clear(header)
	decoder := json.NewDecoder(bytes.NewReader(header))
	decoder.DisallowUnknownFields()
	var spec containerExecSpec
	if err := decoder.Decode(&spec); err != nil {
		return fmt.Errorf("decode container execution specification: %w", err)
	}
	if err := validateContainerExecSpec(spec); err != nil {
		return err
	}
	inspection, err := inspectOwnedContainer(spec.InstallationID, spec.ContainerName)
	if err != nil {
		return err
	}
	if !inspection.Exists {
		return fmt.Errorf("container %q does not exist", spec.ContainerName)
	}
	if inspection.Name != spec.ContainerName {
		return errors.New("container name does not match the persisted Resource installation")
	}
	if !inspection.Running {
		return fmt.Errorf("container %q is not running", spec.ContainerName)
	}

	arguments := []string{"exec", "--interactive"}
	keys := make([]string, 0, len(spec.Environment))
	for key := range spec.Environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		arguments = append(arguments, "--env", key)
	}
	arguments = append(arguments, spec.ContainerName, spec.Executable)
	arguments = append(arguments, spec.Arguments...)
	command := exec.Command(dockerExecutable, arguments...)
	command.Env = make([]string, 0, len(keys))
	for _, key := range keys {
		command.Env = append(command.Env, key+"="+spec.Environment[key])
	}
	defer func() {
		for key := range spec.Environment {
			spec.Environment[key] = ""
		}
		clear(command.Env)
	}()
	command.Stdin = reader
	command.Stdout = output
	stderr := &boundedContainerError{remaining: 800}
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.value.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("execute %s in owned container: %s", spec.Executable, message)
	}
	return nil
}

func validateContainerExecSpec(spec containerExecSpec) error {
	if _, err := uuid.Parse(spec.InstallationID); err != nil {
		return errors.New("installation ID must be a UUID")
	}
	if !containerNamePattern.MatchString(spec.ContainerName) || len(spec.ContainerName) > 128 {
		return errors.New("container name is invalid")
	}
	if !slices.Contains([]string{"pg_dump", "pg_dumpall", "pg_restore", "psql"}, spec.Executable) {
		return errors.New("container executable is not allowed")
	}
	if len(spec.Arguments) > 32 {
		return errors.New("too many container execution arguments")
	}
	for _, argument := range spec.Arguments {
		if strings.ContainsRune(argument, '\x00') || len(argument) > 1024 {
			return errors.New("container execution argument is invalid")
		}
	}
	if len(spec.Environment) > 1 {
		return errors.New("too many container execution environment variables")
	}
	for key := range spec.Environment {
		if key != "PGPASSWORD" {
			return errors.New("container execution environment variable is not allowed")
		}
	}
	return nil
}

type boundedContainerError struct {
	value     strings.Builder
	remaining int
}

func (writer *boundedContainerError) Write(value []byte) (int, error) {
	written := len(value)
	if writer.remaining > 0 {
		kept := min(len(value), writer.remaining)
		_, _ = writer.value.Write(value[:kept])
		writer.remaining -= kept
	}
	return written, nil
}

func inspectContainerLabel(name string) (string, bool, error) {
	output, err := exec.Command(dockerExecutable, "container", "inspect", "--format", "{{ index .Config.Labels \""+installationLabel+"\" }}", name).CombinedOutput()
	if err == nil {
		return strings.TrimSpace(string(output)), true, nil
	}
	message := strings.TrimSpace(string(output))
	if strings.Contains(message, "No such container") || strings.Contains(message, "No such object") {
		return "", false, nil
	}
	return "", false, fmt.Errorf("inspect Docker container %q: %w: %s", name, err, message)
}

func inspectContainerRunning(name string) (bool, error) {
	output, err := exec.Command(dockerExecutable, "container", "inspect", "--format", "{{.State.Running}}", name).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("inspect Docker container %q: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)) == "true", nil
}

func printContainerInspection(installationIDValue, name string) error {
	inspection, err := inspectOwnedContainer(installationIDValue, name)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(inspection)
	if err != nil {
		return fmt.Errorf("encode Docker container inspection: %w", err)
	}
	fmt.Println(string(encoded))
	return nil
}

func printContainerLogs(installationIDValue, name string, tail int) error {
	if tail < 1 || tail > maximumContainerLogTail {
		return fmt.Errorf("container log tail must be between 1 and %d", maximumContainerLogTail)
	}
	inspection, err := inspectOwnedContainer(installationIDValue, name)
	if err != nil {
		return err
	}
	if !inspection.Exists {
		return fmt.Errorf("container %q does not exist", name)
	}
	output, err := exec.Command(dockerExecutable, "logs", "--tail", strconv.Itoa(tail), name).CombinedOutput()
	if len(output) > maximumContainerLogLength {
		output = output[len(output)-maximumContainerLogLength:]
	}
	if _, writeErr := os.Stdout.Write(output); writeErr != nil {
		return fmt.Errorf("write Docker container logs: %w", writeErr)
	}
	if err != nil {
		return fmt.Errorf("read Docker container logs: %w", err)
	}
	return nil
}

func inspectOwnedContainer(installationIDValue, name string) (containerInspection, error) {
	installationID, err := uuid.Parse(installationIDValue)
	if err != nil {
		return containerInspection{}, errors.New("installation ID must be a UUID")
	}
	if !containerNamePattern.MatchString(name) || len(name) > 128 {
		return containerInspection{}, errors.New("container name is invalid")
	}
	output, err := exec.Command(dockerExecutable, "container", "inspect", name).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if strings.Contains(message, "No such container") || strings.Contains(message, "No such object") {
			return containerInspection{Exists: false, Name: name}, nil
		}
		return containerInspection{}, fmt.Errorf("inspect Docker container %q: %w: %s", name, err, message)
	}
	var values []dockerInspection
	if err := json.Unmarshal(output, &values); err != nil || len(values) != 1 {
		return containerInspection{}, errors.New("Docker returned an invalid container inspection")
	}
	value := values[0]
	if value.Config.Labels[installationLabel] != installationID.String() {
		return containerInspection{}, fmt.Errorf("container %q is not owned by this Resource installation", name)
	}
	health := ""
	if value.State.Health != nil {
		health = value.State.Health.Status
	}
	return containerInspection{
		Exists: true, ID: value.ID, Name: strings.TrimPrefix(value.Name, "/"),
		ImageReference: value.Config.Image, ImageID: value.Image, Status: value.State.Status,
		Running: value.State.Running, Health: health, ExitCode: value.State.ExitCode,
		Error: value.State.Error, StartedAt: value.State.StartedAt,
		FinishedAt: value.State.FinishedAt, RestartCount: value.RestartCount,
	}, nil
}

func controlContainer(operation, installationID, name string) error {
	inspection, err := inspectOwnedContainer(installationID, name)
	if err != nil {
		return err
	}
	if !inspection.Exists {
		return fmt.Errorf("container %q does not exist", name)
	}
	arguments := []string{operation, name}
	if operation == "remove" {
		arguments = []string{"rm", "--force", name}
	}
	return run(dockerExecutable, arguments...)
}

func removeContainerVolume(name string) error {
	if !volumeNamePattern.MatchString(name) || len(name) > 128 {
		return errors.New("container volume name is invalid")
	}
	output, err := exec.Command(dockerExecutable, "volume", "inspect", name).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if strings.Contains(message, "No such volume") {
			return nil
		}
		return fmt.Errorf("inspect Docker volume %q: %w: %s", name, err, message)
	}
	return run(dockerExecutable, "volume", "rm", name)
}
