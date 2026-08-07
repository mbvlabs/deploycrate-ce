package buildpacks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"deploycrate-ce/internal/buildpacks/nodeassets"
	"deploycrate-ce/internal/sudo"

	"github.com/google/uuid"
)

const (
	packExecutable         = "/usr/local/bin/pack"
	dockerExecutable       = "/usr/bin/docker"
	NobleBuilderAMD64      = "docker.io/paketobuildpacks/ubuntu-noble-builder@sha256:44aa874655716b237568f3c8b98fea1eae9984bf0e85bffe6847c286b610c767"
	NobleBuilderARM64      = "docker.io/paketobuildpacks/ubuntu-noble-builder@sha256:29a6861e7a6cac353f34478fcf5cfd2fe606f9bcb2e431bc73033e05fb4d722b"
	NodeEngineBuildpack    = "paketo-buildpacks/node-engine@8.4.1"
	GoBuildpackAMD64       = "docker.io/paketobuildpacks/go@sha256:506156f52901969a3dcca288793f86800a57b540c1d5f63329bd3f22b677e659"
	GoBuildpackARM64       = "docker.io/paketobuildpacks/go@sha256:123f547694d9e3ae99e332e9388612c604d6fdfb72dbdfe0f0be674389519bd6"
	NobleRunImageAMD64     = "docker.io/paketobuildpacks/ubuntu-noble-run@sha256:7b47badbf2e31f418d56204bcc25ef8d7e5dd8e8f824fcf87119031bd36db882"
	NobleRunImageARM64     = "docker.io/paketobuildpacks/ubuntu-noble-run@sha256:0f3957568f45877ccbc7cfe6df8b4662037759bb0b543a55e24d35d7cb275c52"
	CacheSchemaVersion     = 1
	PullPolicyIfNotPresent = "if-not-present"
	maxOutputBytes         = 32 << 10
)

type BuildSpec struct {
	Image              string
	Path               string
	ReportDirectory    string
	TemporaryDirectory string
	BuildCache         string
	LaunchCache        string
	PreviousImage      string
	PullPolicy         string
	DockerEnvironment  []string
	BPGOTargets        string
	FrontendScript     string
	Output             io.Writer
}

type CacheNames struct {
	Build  string
	Launch string
}

type Result struct {
	Output string
}

type Client struct{}

func New() Client { return Client{} }

func (Client) Build(ctx context.Context, spec BuildSpec) (Result, error) {
	if strings.TrimSpace(spec.Image) == "" || strings.TrimSpace(spec.Path) == "" ||
		strings.TrimSpace(spec.ReportDirectory) == "" ||
		strings.TrimSpace(spec.TemporaryDirectory) == "" ||
		strings.TrimSpace(spec.BuildCache) == "" ||
		strings.TrimSpace(spec.LaunchCache) == "" {
		return Result{}, errors.New(
			"Pack image, source path, report directory, temporary directory, and caches are required",
		)
	}
	if !validCacheName(spec.BuildCache, "build") || !validCacheName(spec.LaunchCache, "launch") {
		return Result{}, errors.New("Pack caches must use Environment-owned DeployCrate names")
	}
	if !slices.Contains([]string{"always", "never", PullPolicyIfNotPresent}, spec.PullPolicy) {
		return Result{}, errors.New("Pack pull policy is invalid")
	}
	if spec.PreviousImage != "" && !immutableImageReference(spec.PreviousImage) {
		return Result{}, errors.New("Pack previous image must use an immutable SHA-256 reference")
	}
	builder, err := PinnedBuilder()
	if err != nil {
		return Result{}, err
	}
	goBuildpack, err := PinnedGoBuildpack()
	if err != nil {
		return Result{}, err
	}
	runImage, err := PinnedRunImage()
	if err != nil {
		return Result{}, err
	}
	arguments := []string{"build", spec.Image, "--path", spec.Path, "--builder", builder}
	if spec.FrontendScript != "" {
		assetsBuildpack, materializeErr := nodeassets.Materialize(
			filepath.Join(spec.TemporaryDirectory, "buildpacks"),
		)
		if materializeErr != nil {
			return Result{}, materializeErr
		}
		arguments = append(arguments,
			"--buildpack", NodeEngineBuildpack,
			"--buildpack", assetsBuildpack,
			"--env", "BP_DEPLOYCRATE_FRONTEND_SCRIPT="+spec.FrontendScript,
		)
	}
	arguments = append(
		arguments,
		"--buildpack",
		goBuildpack,
		"--trust-extra-buildpacks",
		"--run-image",
		runImage,
		"--publish",
		"--cache",
		cacheArgument("build", spec.BuildCache),
		"--cache",
		cacheArgument("launch", spec.LaunchCache),
		"--pull-policy",
		spec.PullPolicy,
		"--timestamps",
		"--report-output-dir",
		spec.ReportDirectory,
	)
	if spec.PreviousImage != "" {
		arguments = append(arguments, "--previous-image", spec.PreviousImage)
	}
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
		return Result{
				Output: output.String(),
			}, fmt.Errorf(
				"Pack build failed: %w: %s",
				err,
				output.String(),
			)
	}
	return Result{Output: output.String()}, nil
}

func EnvironmentCacheNames(environmentID uuid.UUID) (CacheNames, error) {
	if environmentID == uuid.Nil {
		return CacheNames{}, errors.New("Environment cache owner is required")
	}
	prefix := fmt.Sprintf("deploycrate-pack-%s-v%d", environmentID, CacheSchemaVersion)
	return CacheNames{Build: prefix + "-build", Launch: prefix + "-launch"}, nil
}

func (Client) DeleteEnvironmentCaches(ctx context.Context, environmentID uuid.UUID) error {
	caches, err := EnvironmentCacheNames(environmentID)
	if err != nil {
		return err
	}
	for _, cache := range []string{caches.Build, caches.Launch} {
		output, removeErr := sudo.CommandContext(ctx, dockerExecutable, "volume", "rm", cache).
			CombinedOutput()
		if removeErr == nil {
			continue
		}
		message := boundedCommandOutput(output)
		lower := strings.ToLower(message)
		if strings.Contains(lower, "no such volume") || strings.Contains(lower, "not found") {
			continue
		}
		return fmt.Errorf("remove Pack cache volume %s: %w: %s", cache, removeErr, message)
	}
	return nil
}

func cacheArgument(kind, name string) string {
	return "type=" + kind + ";format=volume;name=" + name
}

func validCacheName(name, kind string) bool {
	prefix := "deploycrate-pack-"
	suffix := fmt.Sprintf("-v%d-%s", CacheSchemaVersion, kind)
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return false
	}
	owner := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	parsed, err := uuid.Parse(owner)
	return err == nil && parsed != uuid.Nil && parsed.String() == owner
}

func immutableImageReference(reference string) bool {
	const marker = "@sha256:"
	position := strings.LastIndex(reference, marker)
	if position < 1 || len(reference[position+len(marker):]) != 64 {
		return false
	}
	for _, character := range reference[position+len(marker):] {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func boundedCommandOutput(output []byte) string {
	message := strings.TrimSpace(string(output))
	if len(message) > 2048 {
		message = message[:2048]
	}
	return message
}

func PinnedBuilder() (string, error) {
	return PinnedBuilderForArchitecture(runtime.GOARCH)
}

func PinnedBuilderForArchitecture(architecture string) (string, error) {
	switch architecture {
	case "amd64":
		return NobleBuilderAMD64, nil
	case "arm64":
		return NobleBuilderARM64, nil
	default:
		return "", fmt.Errorf(
			"Go Buildpacks are not approved for host architecture %s",
			architecture,
		)
	}
}

func PinnedGoBuildpack() (string, error) {
	return PinnedGoBuildpackForArchitecture(runtime.GOARCH)
}

func PinnedGoBuildpackForArchitecture(architecture string) (string, error) {
	switch architecture {
	case "amd64":
		return GoBuildpackAMD64, nil
	case "arm64":
		return GoBuildpackARM64, nil
	default:
		return "", fmt.Errorf(
			"Go Buildpacks are not approved for host architecture %s",
			architecture,
		)
	}
}

func PinnedRunImage() (string, error) {
	return PinnedRunImageForArchitecture(runtime.GOARCH)
}

func PinnedRunImageForArchitecture(architecture string) (string, error) {
	switch architecture {
	case "amd64":
		return NobleRunImageAMD64, nil
	case "arm64":
		return NobleRunImageARM64, nil
	default:
		return "", fmt.Errorf(
			"Go Buildpacks are not approved for host architecture %s",
			architecture,
		)
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
