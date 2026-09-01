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

const BuildpackSettingsSchemaVersion = 4

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

type BuildpackFrontendSSRSettings struct {
	Enabled bool   `json:"enabled"`
	Script  string `json:"script"`
}

type BuildpackFrontendSettings struct {
	Runtime   string                        `json:"runtime"`
	Directory string                        `json:"directory"`
	Script    string                        `json:"script"`
	SSR       *BuildpackFrontendSSRSettings `json:"ssr,omitempty"`
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

func DefaultBuildpackSettings() json.RawMessage {
	return json.RawMessage(`{"schema_version":4,"runtime":"go"}`)
}

func IsSupportedBuildpackRuntime(runtime BuildpackRuntime) bool {
	return slices.Contains(SupportedBuildpackRuntimes, runtime)
}

func ParseBuildpackSettings(value json.RawMessage) (BuildpackSettings, error) {
	var settings BuildpackSettings
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		return settings, errors.New("Buildpacks settings must use the supported schema")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return settings, errors.New("Buildpacks settings must contain one JSON object")
	}
	if settings.SchemaVersion != 2 &&
		settings.SchemaVersion != 3 &&
		settings.SchemaVersion != BuildpackSettingsSchemaVersion {
		return settings, errors.New("Buildpacks settings schema version is unsupported")
	}
	if settings.SchemaVersion == 2 && settings.Runtime == "" {
		settings.Runtime = BuildpackRuntimeGo
	}
	settings.SchemaVersion = BuildpackSettingsSchemaVersion
	settings.Runtime = BuildpackRuntime(
		strings.ToLower(strings.TrimSpace(string(settings.Runtime))),
	)
	if !IsSupportedBuildpackRuntime(settings.Runtime) {
		return settings, errors.New("Buildpacks runtime must be go, rails, laravel, or django")
	}
	if settings.Frontend == nil {
		if settings.Advanced == nil {
			return settings, nil
		}
		return validateAdvancedSettings(settings)
	}
	settings.Frontend.Runtime = strings.TrimSpace(settings.Frontend.Runtime)
	settings.Frontend.Directory = strings.TrimSpace(settings.Frontend.Directory)
	settings.Frontend.Script = strings.TrimSpace(settings.Frontend.Script)
	if settings.Frontend.Runtime != "node" {
		return settings, errors.New("frontend runtime must be node")
	}
	if settings.Frontend.Directory == "" {
		settings.Frontend.Directory = "."
	}
	if strings.HasPrefix(settings.Frontend.Directory, "/") {
		return settings, errors.New("frontend directory must be relative")
	}
	settings.Frontend.Directory = path.Clean(settings.Frontend.Directory)
	if settings.Frontend.Directory == ".." ||
		strings.HasPrefix(settings.Frontend.Directory, "../") {
		return settings, errors.New("frontend directory cannot leave the build context")
	}
	if !buildpackScriptPattern.MatchString(settings.Frontend.Script) {
		return settings, errors.New("frontend script must be a package script name")
	}
	if settings.Frontend.SSR != nil {
		settings.Frontend.SSR.Script = strings.TrimSpace(settings.Frontend.SSR.Script)
		if settings.Frontend.SSR.Enabled {
			if settings.Frontend.SSR.Script == "" {
				settings.Frontend.SSR.Script = "build:ssr"
			}
			if !buildpackScriptPattern.MatchString(settings.Frontend.SSR.Script) {
				return settings, errors.New("SSR build script must be a package script name")
			}
		} else {
			settings.Frontend.SSR = nil
		}
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
	return settings.Frontend != nil
}

func (settings BuildpackSettings) SSREnabled() bool {
	return settings.Frontend != nil &&
		settings.Frontend.SSR != nil &&
		settings.Frontend.SSR.Enabled
}

func (settings BuildpackSettings) PackEnvironment() []string {
	environment := make([]string, 0, 8)
	if settings.Frontend != nil {
		environment = append(
			environment,
			"BP_DEPLOYCRATE_FRONTEND_SCRIPT="+settings.Frontend.Script,
			"BP_DEPLOYCRATE_FRONTEND_DIRECTORY="+settings.Frontend.Directory,
		)
		if settings.SSREnabled() {
			environment = append(
				environment,
				"BP_DEPLOYCRATE_FRONTEND_SSR=true",
				"BP_DEPLOYCRATE_FRONTEND_SSR_SCRIPT="+settings.Frontend.SSR.Script,
			)
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
