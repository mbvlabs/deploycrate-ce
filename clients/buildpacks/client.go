package buildpacks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

const (
	packExecutable    = "/usr/local/bin/pack"
	NobleBuilderAMD64 = "docker.io/paketobuildpacks/ubuntu-noble-builder@sha256:44aa874655716b237568f3c8b98fea1eae9984bf0e85bffe6847c286b610c767"
	NobleBuilderARM64 = "docker.io/paketobuildpacks/ubuntu-noble-builder@sha256:29a6861e7a6cac353f34478fcf5cfd2fe606f9bcb2e431bc73033e05fb4d722b"
	GoBuildpackAMD64  = "docker.io/paketobuildpacks/go@sha256:506156f52901969a3dcca288793f86800a57b540c1d5f63329bd3f22b677e659"
	GoBuildpackARM64  = "docker.io/paketobuildpacks/go@sha256:123f547694d9e3ae99e332e9388612c604d6fdfb72dbdfe0f0be674389519bd6"
	maxOutputBytes    = 32 << 10
)

type BuildSpec struct {
	Image              string
	Path               string
	ReportDirectory    string
	TemporaryDirectory string
	DockerEnvironment  []string
	BPGOTargets        string
	Output             io.Writer
}

type Result struct {
	Output string
}

type Client struct{}

func New() Client { return Client{} }

func (Client) Build(ctx context.Context, spec BuildSpec) (Result, error) {
	if strings.TrimSpace(spec.Image) == "" || strings.TrimSpace(spec.Path) == "" || strings.TrimSpace(spec.ReportDirectory) == "" || strings.TrimSpace(spec.TemporaryDirectory) == "" {
		return Result{}, errors.New("Pack image, source path, report directory, and temporary directory are required")
	}
	builder, err := PinnedBuilder()
	if err != nil {
		return Result{}, err
	}
	buildpack, err := PinnedGoBuildpack()
	if err != nil {
		return Result{}, err
	}
	arguments := []string{"build", spec.Image, "--path", spec.Path, "--builder", builder,
		"--buildpack", buildpack, "--publish",
		"--report-output-dir", spec.ReportDirectory}
	if spec.BPGOTargets != "" {
		arguments = append(arguments, "--env", "BP_GO_TARGETS="+spec.BPGOTargets)
	}
	command := exec.CommandContext(ctx, packExecutable, arguments...)
	command.Env = append([]string{}, spec.DockerEnvironment...)
	command.Env = append(command.Env, "TMPDIR="+spec.TemporaryDirectory)
	output := &tailBuffer{limit: maxOutputBytes}
	commandOutput := io.Writer(output)
	if spec.Output != nil {
		commandOutput = io.MultiWriter(output, spec.Output)
	}
	command.Stdout = commandOutput
	command.Stderr = commandOutput
	if err := command.Run(); err != nil {
		return Result{Output: output.String()}, fmt.Errorf("Pack build failed: %w: %s", err, output.String())
	}
	return Result{Output: output.String()}, nil
}

func PinnedBuilder() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return NobleBuilderAMD64, nil
	case "arm64":
		return NobleBuilderARM64, nil
	default:
		return "", fmt.Errorf("Go Buildpacks are not approved for host architecture %s", runtime.GOARCH)
	}
}

func PinnedGoBuildpack() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return GoBuildpackAMD64, nil
	case "arm64":
		return GoBuildpackARM64, nil
	default:
		return "", fmt.Errorf("Go Buildpacks are not approved for host architecture %s", runtime.GOARCH)
	}
}

type tailBuffer struct {
	mutex     sync.Mutex
	value     []byte
	limit     int
	truncated bool
}

func (writer *tailBuffer) Write(value []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	original := len(value)
	if writer.limit < 1 {
		return original, nil
	}
	if len(value) >= writer.limit {
		writer.value = append(writer.value[:0], value[len(value)-writer.limit:]...)
		writer.truncated = true
		return original, nil
	}
	if overflow := len(writer.value) + len(value) - writer.limit; overflow > 0 {
		copy(writer.value, writer.value[overflow:])
		writer.value = writer.value[:len(writer.value)-overflow]
		writer.truncated = true
	}
	writer.value = append(writer.value, value...)
	return original, nil
}

func (writer *tailBuffer) String() string {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	value := strings.TrimSpace(string(writer.value))
	if writer.truncated {
		value = "[earlier output truncated]\n" + value
	}
	return value
}

var _ io.Writer = (*tailBuffer)(nil)
