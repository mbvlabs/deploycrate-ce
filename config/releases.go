package config

import (
	"errors"
	"os"
	"strings"
)

const (
	DevelopmentReleaseBaseURL = "https://get-dev.deploycrate.com"
	ReleaseApplicationPath    = "/dc-ce-app/deploycrate-ce"
	ReleaseChecksumPath       = "/dc-ce-app/deploycrate-ce.sha256"
)

var ErrReleaseSourceUnavailable = errors.New("self-update is unavailable for this build; set DEPLOYCRATE_CE_RELEASE_BASE_URL to a Cloudflare R2 endpoint")

type ReleaseSource struct {
	BaseURL     string
	Development bool
}

func ResolveReleaseSource(version string) (ReleaseSource, error) {
	if override := strings.TrimSpace(os.Getenv("DEPLOYCRATE_CE_RELEASE_BASE_URL")); override != "" {
		return ReleaseSource{BaseURL: strings.TrimRight(override, "/")}, nil
	}

	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if version == "dev" || strings.HasPrefix(version, "development-") {
		return ReleaseSource{BaseURL: DevelopmentReleaseBaseURL, Development: true}, nil
	}

	return ReleaseSource{}, ErrReleaseSourceUnavailable
}
