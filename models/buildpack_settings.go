package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

const BuildpackSettingsSchemaVersion = 1

var buildpackScriptPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:_-]{0,63}$`)

type BuildpackFrontendSettings struct {
	Runtime        string `json:"runtime"`
	PackageManager string `json:"package_manager"`
	Script         string `json:"script"`
}

type BuildpackSettings struct {
	SchemaVersion int                        `json:"schema_version"`
	Frontend      *BuildpackFrontendSettings `json:"frontend,omitempty"`
}

func DefaultBuildpackSettings() json.RawMessage {
	return json.RawMessage(`{"schema_version":1}`)
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
	if settings.SchemaVersion != BuildpackSettingsSchemaVersion {
		return settings, errors.New("Buildpacks settings schema version is unsupported")
	}
	if settings.Frontend == nil {
		return settings, nil
	}
	settings.Frontend.Runtime = strings.TrimSpace(settings.Frontend.Runtime)
	settings.Frontend.PackageManager = strings.TrimSpace(settings.Frontend.PackageManager)
	settings.Frontend.Script = strings.TrimSpace(settings.Frontend.Script)
	if settings.Frontend.Runtime != "node" {
		return settings, errors.New("frontend runtime must be node")
	}
	if settings.Frontend.PackageManager != "pnpm" {
		return settings, errors.New("frontend package manager must be pnpm")
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
