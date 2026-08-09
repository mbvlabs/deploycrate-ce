package resourceaccess

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

const maximumImageReferenceLength = 512

//go:embed os-update-check.sh
var osUpdateCheckScript string

//go:embed os-update-apply.sh
var osUpdateApplyScript string

//go:embed provision-capability.sh
var capabilityProvisionScript string

// OSUpdateCheckScript returns the shell script that refreshes the package index
// and reports the available system upgrades as JSON on stdout.
func OSUpdateCheckScript() []byte {
	return []byte(osUpdateCheckScript)
}

// OSUpdateApplyScript returns the shell script that applies system upgrades and
// reports whether a reboot is required as JSON on stdout.
func OSUpdateApplyScript() []byte {
	return []byte(osUpdateApplyScript)
}

// CapabilityProvisionScript returns the shell script that provisions the
// requested server workload capabilities on the host.
func CapabilityProvisionScript() []byte {
	return []byte(capabilityProvisionScript)
}

type dockerContainer struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Image  string `json:"Image"`
	State  string `json:"State"`
	Status string `json:"Status"`
	Ports  string `json:"Ports"`
}

type dockerImage struct {
	ID         string `json:"ID"`
	Repository string `json:"Repository"`
	Tag        string `json:"Tag"`
	Size       string `json:"Size"`
}

func listAllContainers() (string, error) {
	output, err := exec.Command(
		dockerExecutable,
		"ps",
		"-a",
		"--format",
		"{{json .}}",
	).Output()
	if err != nil {
		return "", fmt.Errorf("list Docker containers: %w", err)
	}
	containers := make([]dockerContainer, 0)
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var value dockerContainer
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			return "", errors.New("Docker returned an invalid container listing")
		}
		containers = append(containers, value)
	}
	var builder strings.Builder
	encoder := json.NewEncoder(&builder)
	for _, container := range containers {
		if err := encoder.Encode(container); err != nil {
			return "", fmt.Errorf("encode Docker container listing: %w", err)
		}
	}
	return builder.String(), nil
}

func listAllImages() (string, error) {
	output, err := exec.Command(
		dockerExecutable,
		"images",
		"--format",
		"{{json .}}",
	).Output()
	if err != nil {
		return "", fmt.Errorf("list Docker images: %w", err)
	}
	images := make([]dockerImage, 0)
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var value dockerImage
		if err := json.Unmarshal([]byte(line), &value); err != nil {
			return "", errors.New("Docker returned an invalid image listing")
		}
		images = append(images, value)
	}
	var builder strings.Builder
	encoder := json.NewEncoder(&builder)
	for _, image := range images {
		if err := encoder.Encode(image); err != nil {
			return "", fmt.Errorf("encode Docker image listing: %w", err)
		}
	}
	return builder.String(), nil
}

func controlContainerByName(operation, name string) error {
	if !containerNamePattern.MatchString(name) || len(name) > 128 {
		return errors.New("container name is invalid")
	}
	if !slices.Contains([]string{"start", "stop", "restart", "remove"}, operation) {
		return errors.New("container operation is invalid")
	}
	arguments := []string{operation, name}
	if operation == "remove" {
		arguments = []string{"rm", "--force", name}
	}
	return run(dockerExecutable, arguments...)
}

func printContainerLogsByName(name string, tail int) error {
	if !containerNamePattern.MatchString(name) || len(name) > 128 {
		return errors.New("container name is invalid")
	}
	if tail < 1 || tail > maximumContainerLogTail {
		return fmt.Errorf("container log tail must be between 1 and %d", maximumContainerLogTail)
	}
	output, err := exec.Command(
		dockerExecutable,
		"logs",
		"--tail",
		strconv.Itoa(tail),
		name,
	).CombinedOutput()
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

func removeImage(reference string) error {
	if !validImageReference(reference) {
		return errors.New("image reference is invalid")
	}
	return run(dockerExecutable, "rmi", reference)
}

func validImageReference(value string) bool {
	return value != "" &&
		strings.TrimSpace(value) == value &&
		!strings.HasPrefix(value, "-") &&
		!strings.ContainsAny(value, " \t\r\n") &&
		len(value) <= maximumImageReferenceLength
}

func runScript(script string) (string, error) {
	command := exec.Command("/usr/bin/env", "bash", "-s")
	command.Stdin = strings.NewReader(script)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf(
			"run provisioning script: %w: %s",
			err,
			strings.TrimSpace(string(output)),
		)
	}
	return strings.TrimSpace(string(output)), nil
}

func checkHostUpdates() (string, error) {
	return runScript(osUpdateCheckScript)
}

func applyHostUpdates() (string, error) {
	return runScript(osUpdateApplyScript)
}

func rebootHost() error {
	return run("/usr/bin/systemctl", "reboot", "--no-block")
}

func applyCapabilities(capabilities []string) error {
	if len(capabilities) == 0 {
		return errors.New("at least one server capability is required")
	}
	for _, capability := range capabilities {
		if !slices.Contains(
			[]string{"build", "runtime", "resource", "database", "repository"},
			capability,
		) {
			return fmt.Errorf("unknown server capability %q", capability)
		}
	}
	command := exec.Command("/usr/bin/env", "bash", "-s")
	command.Stdin = strings.NewReader(capabilityProvisionScript)
	command.Env = append(
		os.Environ(),
		"DEPLOYCRATE_CAPABILITIES="+strings.Join(capabilities, ","),
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"provision server capabilities: %w: %s",
			err,
			strings.TrimSpace(string(output)),
		)
	}
	return nil
}
