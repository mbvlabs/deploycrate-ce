package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"
)

const BuildpackSettingsSchemaVersion = 5

type BuildpackRuntime string

const (
	BuildpackRuntimeGo      BuildpackRuntime = "go"
	BuildpackRuntimeRails   BuildpackRuntime = "rails"
	BuildpackRuntimeLaravel BuildpackRuntime = "laravel"
	BuildpackRuntimeDjango  BuildpackRuntime = "django"
)

var SupportedBuildpackRuntimes = []BuildpackRuntime{
	BuildpackRuntimeGo,
	BuildpackRuntimeRails,
	BuildpackRuntimeLaravel,
	BuildpackRuntimeDjango,
}

var buildpackScriptPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:_-]{0,63}$`)
var buildpackVersionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}([-.][0-9A-Za-z]+)?$`)

type buildpackFrontendSSRSettings struct {
	Enabled bool   `json:"enabled"`
	Script  string `json:"script"`
}

type buildpackFrontendSettingsJSON struct {
	Runtime         string                        `json:"runtime"`
	Directory       string                        `json:"directory"`
	Script          string                        `json:"script"`
	Scripts         []string                      `json:"scripts"`
	SSR             *buildpackFrontendSSRSettings `json:"ssr,omitempty"`
	KeepNodeRuntime bool                          `json:"keep_node_runtime"`
}

type BuildpackFrontendSettings struct {
	Runtime         string   `json:"runtime"`
	Directory       string   `json:"directory"`
	Scripts         []string `json:"scripts"`
	KeepNodeRuntime bool     `json:"keep_node_runtime,omitempty"`
}

type BuildpackAdvancedSettings struct {
	GoVersion    string `json:"go_version,omitempty"`
	GoBuildFlags string `json:"go_build_flags,omitempty"`
	NodeVersion  string `json:"node_version,omitempty"`
}

type BuildpackSettings struct {
	SchemaVersion int                        `json:"schema_version"`
	Runtime       BuildpackRuntime           `json:"runtime"`
	Frontend      *BuildpackFrontendSettings `json:"frontend,omitempty"`
	Advanced      *BuildpackAdvancedSettings `json:"advanced,omitempty"`
}

type buildpackSettingsJSON struct {
	SchemaVersion int                            `json:"schema_version"`
	Runtime       BuildpackRuntime               `json:"runtime"`
	Frontend      *buildpackFrontendSettingsJSON `json:"frontend,omitempty"`
	Advanced      *BuildpackAdvancedSettings     `json:"advanced,omitempty"`
}

func DefaultBuildpackSettings() json.RawMessage {
	return json.RawMessage(`{"schema_version":5,"runtime":"go"}`)
}

func IsSupportedBuildpackRuntime(runtime BuildpackRuntime) bool {
	return slices.Contains(SupportedBuildpackRuntimes, runtime)
}

func normalizeFrontendSettings(
	raw *buildpackFrontendSettingsJSON,
) (*BuildpackFrontendSettings, error) {
	if raw == nil {
		return nil, nil
	}
	runtime := strings.TrimSpace(raw.Runtime)
	directory := strings.TrimSpace(raw.Directory)
	if runtime != "node" {
		return nil, errors.New("frontend runtime must be node")
	}
	if directory == "" {
		directory = "."
	}
	if strings.HasPrefix(directory, "/") {
		return nil, errors.New("frontend directory must be relative")
	}
	directory = path.Clean(directory)
	if directory == ".." || strings.HasPrefix(directory, "../") {
		return nil, errors.New("frontend directory cannot leave the build context")
	}

	scripts := make([]string, 0, len(raw.Scripts)+2)
	for _, script := range raw.Scripts {
		script = strings.TrimSpace(script)
		if script == "" {
			continue
		}
		scripts = append(scripts, script)
	}
	if len(scripts) == 0 {
		legacyScript := strings.TrimSpace(raw.Script)
		if legacyScript != "" {
			scripts = append(scripts, legacyScript)
		}
	}
	if raw.SSR != nil && raw.SSR.Enabled {
		ssrScript := strings.TrimSpace(raw.SSR.Script)
		if ssrScript == "" {
			ssrScript = "build:ssr"
		}
		scripts = append(scripts, ssrScript)
	}
	if len(scripts) == 0 {
		scripts = append(scripts, "build")
	}
	for _, script := range scripts {
		if !buildpackScriptPattern.MatchString(script) {
			return nil, errors.New("frontend build script must be a package script name")
		}
	}

	keepNodeRuntime := raw.KeepNodeRuntime
	if !keepNodeRuntime && raw.SSR != nil && raw.SSR.Enabled {
		keepNodeRuntime = true
	}

	return &BuildpackFrontendSettings{
		Runtime:         "node",
		Directory:       directory,
		Scripts:         scripts,
		KeepNodeRuntime: keepNodeRuntime,
	}, nil
}

func ParseBuildpackSettings(value json.RawMessage) (BuildpackSettings, error) {
	var raw buildpackSettingsJSON
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return BuildpackSettings{}, errors.New("Buildpacks settings must use the supported schema")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return BuildpackSettings{}, errors.New("Buildpacks settings must contain one JSON object")
	}
	if raw.SchemaVersion != 2 &&
		raw.SchemaVersion != 3 &&
		raw.SchemaVersion != 4 &&
		raw.SchemaVersion != BuildpackSettingsSchemaVersion {
		return BuildpackSettings{}, errors.New("Buildpacks settings schema version is unsupported")
	}
	if raw.SchemaVersion == 2 && raw.Runtime == "" {
		raw.Runtime = BuildpackRuntimeGo
	}
	settings := BuildpackSettings{
		SchemaVersion: BuildpackSettingsSchemaVersion,
		Runtime: BuildpackRuntime(
			strings.ToLower(strings.TrimSpace(string(raw.Runtime))),
		),
		Advanced: raw.Advanced,
	}
	if !IsSupportedBuildpackRuntime(settings.Runtime) {
		return settings, errors.New("Buildpacks runtime must be go, rails, laravel, or django")
	}
	if raw.Frontend != nil {
		frontend, err := normalizeFrontendSettings(raw.Frontend)
		if err != nil {
			return settings, err
		}
		settings.Frontend = frontend
	}
	if settings.Frontend == nil {
		if settings.Advanced == nil {
			return settings, nil
		}
		return validateAdvancedSettings(settings)
	}
	return validateAdvancedSettings(settings)
}

func validateAdvancedSettings(settings BuildpackSettings) (BuildpackSettings, error) {
	if settings.Advanced == nil {
		return settings, nil
	}
	settings.Advanced.GoVersion = strings.TrimSpace(settings.Advanced.GoVersion)
	settings.Advanced.GoBuildFlags = strings.TrimSpace(settings.Advanced.GoBuildFlags)
	settings.Advanced.NodeVersion = strings.TrimSpace(settings.Advanced.NodeVersion)
	if settings.Advanced.GoVersion != "" &&
		!buildpackVersionPattern.MatchString(settings.Advanced.GoVersion) {
		return settings, errors.New("buildpack Go version must look like 1.23.4")
	}
	if settings.Advanced.NodeVersion != "" &&
		!buildpackVersionPattern.MatchString(settings.Advanced.NodeVersion) {
		return settings, errors.New("buildpack Node version must look like 22.11.0")
	}
	if strings.ContainsAny(settings.Advanced.GoBuildFlags, "\r\n") {
		return settings, errors.New("buildpack Go build flags cannot contain line breaks")
	}
	if len(settings.Advanced.GoBuildFlags) > 512 {
		return settings, errors.New("buildpack Go build flags are too long")
	}
	if settings.Advanced.GoVersion == "" &&
		settings.Advanced.GoBuildFlags == "" &&
		settings.Advanced.NodeVersion == "" {
		settings.Advanced = nil
	}
	return settings, nil
}

func (settings BuildpackSettings) FrontendEnabled() bool {
	return settings.Frontend != nil && len(settings.Frontend.Scripts) > 0
}

func (settings BuildpackSettings) PackEnvironment() []string {
	environment := make([]string, 0, 8)
	if settings.Frontend != nil && len(settings.Frontend.Scripts) > 0 {
		environment = append(
			environment,
			"BP_DEPLOYCRATE_FRONTEND_SCRIPTS="+strings.Join(settings.Frontend.Scripts, ","),
			"BP_DEPLOYCRATE_FRONTEND_DIRECTORY="+settings.Frontend.Directory,
		)
		if settings.Frontend.KeepNodeRuntime {
			environment = append(environment, "BP_DEPLOYCRATE_KEEP_NODE_RUNTIME=true")
		}
	}
	if settings.Advanced != nil {
		if settings.Advanced.GoVersion != "" {
			environment = append(environment, "BP_GO_VERSION="+settings.Advanced.GoVersion)
		}
		if settings.Advanced.GoBuildFlags != "" {
			environment = append(environment, "BP_GO_BUILD_FLAGS="+settings.Advanced.GoBuildFlags)
		}
		if settings.Advanced.NodeVersion != "" {
			environment = append(environment, "BP_NODE_VERSION="+settings.Advanced.NodeVersion)
		}
	}
	return environment
}

func CanonicalBuildpackSettings(value json.RawMessage) (json.RawMessage, error) {
	settings, err := ParseBuildpackSettings(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(settings)
}
