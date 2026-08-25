package config

import (
	"errors"
	"os"
	"strings"
)

const (
	StableReleaseBaseURL = "https://get.deploycrate.com"
	EdgeReleaseBaseURL   = "https://get-dev.deploycrate.com"
	ReleaseChannelStable = "stable"
	ReleaseChannelEdge   = "edge"
)

var ErrReleaseSourceUnavailable = errors.New(
	"self-update is unavailable: DEPLOYCRATE_CE_UPDATE_CHANNEL must be stable or edge",
)

type ReleaseSource struct {
	BaseURL string
	Channel string
}

func ResolveReleaseSource(version string) (ReleaseSource, error) {
	channel := strings.ToLower(strings.TrimSpace(os.Getenv("DEPLOYCRATE_CE_UPDATE_CHANNEL")))
	if channel == "" {
		version = strings.TrimPrefix(strings.TrimSpace(version), "v")
		if version == "dev" || strings.HasPrefix(version, "development-") ||
			strings.HasPrefix(version, "edge-") {
			channel = ReleaseChannelEdge
		} else {
			channel = ReleaseChannelStable
		}
	}

	baseURL := strings.TrimSpace(os.Getenv("DEPLOYCRATE_CE_RELEASE_BASE_URL"))
	if baseURL == "" {
		switch channel {
		case ReleaseChannelStable:
			baseURL = StableReleaseBaseURL
		case ReleaseChannelEdge:
			baseURL = EdgeReleaseBaseURL
		default:
			return ReleaseSource{}, ErrReleaseSourceUnavailable
		}
	}
	if channel != ReleaseChannelStable && channel != ReleaseChannelEdge {
		return ReleaseSource{}, ErrReleaseSourceUnavailable
	}
	return ReleaseSource{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Channel: channel,
	}, nil
}
