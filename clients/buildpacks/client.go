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
	JammyFullBuilderAMD64  = "docker.io/paketobuildpacks/builder-jammy-buildpackless-full@sha256:aab2231ffe9dd903bed666748de610e21f5428ebc10f887025314c5e14c44266"
	JammyFullBuilderARM64  = "docker.io/paketobuildpacks/builder-jammy-buildpackless-full@sha256:81ddba577a731d3d8d016be9930c21d74909ae3f4c713be5f2649da025072027"
	JammyFullRunAMD64      = "docker.io/paketobuildpacks/run-jammy-full@sha256:6eb3a6192e698b6eea8028e84aad8ab1155bcdd0abf95a1bddc85996c8113305"
	JammyFullRunARM64      = "docker.io/paketobuildpacks/run-jammy-full@sha256:bf98ec091241a6e2f66b5227aa57b33ea7129bb64d068e9dfd6676b91b59f8da"
	RubyBuildpackAMD64     = "docker.io/paketobuildpacks/ruby@sha256:82c668d1ddc4d10715e47fde028d2b7153e33c4bee66a84128222d4a04c6f718"
	PHPBuildpackAMD64      = "docker.io/paketobuildpacks/php@sha256:d1124e6b2c39e083fbdcb7f8d589f8886d106aea7fb95ba5f73b790cc4593236"
	PythonBuildpackAMD64   = "docker.io/paketobuildpacks/python@sha256:0ba65e1e82fc80ef91e509503ae53959d680681baba0e070dc6020c987f1f324"
	PythonBuildpackARM64   = "docker.io/paketobuildpacks/python@sha256:bb1dbe6a1aaccdf6a187e2884b7812a14c82ef3cafb5a2284184aec66b23ba6c"
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
	FrontendDirectory  string
	Runtime            string
	Output             io.Writer
}

type Profile struct {
	Builder     string
	Buildpack   string
	RunImage    string
	Environment []string
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
	profile, err := ProfileForArchitecture(spec.Runtime, runtime.GOARCH)
	if err != nil {
		return Result{}, err
	}
	arguments := []string{"build", spec.Image, "--path", spec.Path, "--builder", profile.Builder}
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
			"--env", "BP_DEPLOYCRATE_FRONTEND_DIRECTORY="+spec.FrontendDirectory,
		)
	}
	arguments = append(
		arguments,
		"--buildpack",
		profile.Buildpack,
		"--trust-extra-buildpacks",
		"--run-image",
		profile.RunImage,
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
	for _, environment := range profile.Environment {
		arguments = append(arguments, "--env", environment)
	}
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

func ProfileForArchitecture(buildpackRuntime, architecture string) (Profile, error) {
	buildpackRuntime = strings.ToLower(strings.TrimSpace(buildpackRuntime))
	if buildpackRuntime == "" {
		buildpackRuntime = "go"
	}
	if buildpackRuntime == "go" {
		builder, err := PinnedBuilderForArchitecture(architecture)
		if err != nil {
			return Profile{}, err
		}
		buildpack, err := PinnedGoBuildpackForArchitecture(architecture)
		if err != nil {
			return Profile{}, err
		}
		runImage, err := PinnedRunImageForArchitecture(architecture)
		if err != nil {
			return Profile{}, err
		}
		return Profile{Builder: builder, Buildpack: buildpack, RunImage: runImage}, nil
	}

	var builder, runImage string
	switch architecture {
	case "amd64":
		builder, runImage = JammyFullBuilderAMD64, JammyFullRunAMD64
	case "arm64":
		builder, runImage = JammyFullBuilderARM64, JammyFullRunARM64
	default:
		return Profile{}, fmt.Errorf(
			"Buildpacks runtime %s is not approved for host architecture %s",
			buildpackRuntime,
			architecture,
		)
	}

	switch buildpackRuntime {
	case "rails":
		if architecture != "amd64" {
			return Profile{}, errors.New(
				"Rails Buildpacks are currently available only on amd64 Servers",
			)
		}
		return Profile{Builder: builder, Buildpack: RubyBuildpackAMD64, RunImage: runImage}, nil
	case "laravel":
		if architecture != "amd64" {
			return Profile{}, errors.New(
				"Laravel Buildpacks are currently available only on amd64 Servers",
			)
		}
		return Profile{
			Builder: builder, Buildpack: PHPBuildpackAMD64, RunImage: runImage,
			Environment: []string{"BP_PHP_SERVER=nginx", "BP_PHP_WEB_DIR=public"},
		}, nil
	case "django":
		buildpack := PythonBuildpackAMD64
		if architecture == "arm64" {
			buildpack = PythonBuildpackARM64
		}
		return Profile{Builder: builder, Buildpack: buildpack, RunImage: runImage}, nil
	default:
		return Profile{}, fmt.Errorf("unsupported Buildpacks runtime %q", buildpackRuntime)
	}
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
