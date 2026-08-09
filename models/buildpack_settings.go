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

const BuildpackSettingsSchemaVersion = 3

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

type BuildpackFrontendSettings struct {
	Runtime   string `json:"runtime"`
	Directory string `json:"directory"`
	Script    string `json:"script"`
}

type BuildpackSettings struct {
	SchemaVersion int                        `json:"schema_version"`
	Runtime       BuildpackRuntime           `json:"runtime"`
	Frontend      *BuildpackFrontendSettings `json:"frontend,omitempty"`
}

func DefaultBuildpackSettings() json.RawMessage {
	return json.RawMessage(`{"schema_version":3,"runtime":"go"}`)
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
	if settings.SchemaVersion != 2 && settings.SchemaVersion != BuildpackSettingsSchemaVersion {
		return settings, errors.New("Buildpacks settings schema version is unsupported")
	}
	if settings.SchemaVersion == 2 && settings.Runtime == "" {
		settings.Runtime = BuildpackRuntimeGo
	}
	settings.SchemaVersion = BuildpackSettingsSchemaVersion
	settings.Runtime = BuildpackRuntime(strings.ToLower(strings.TrimSpace(string(settings.Runtime))))
	if !IsSupportedBuildpackRuntime(settings.Runtime) {
		return settings, errors.New("Buildpacks runtime must be go, rails, laravel, or django")
	}
	if settings.Frontend == nil {
		return settings, nil
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
	return settings, nil
}

func CanonicalBuildpackSettings(value json.RawMessage) (json.RawMessage, error) {
	settings, err := ParseBuildpackSettings(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(settings)
}
