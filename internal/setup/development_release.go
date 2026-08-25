package setup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

type releaseArtifact struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

type releaseManifest struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Channel       string                     `json:"channel"`
	Version       string                     `json:"version"`
	Artifacts     map[string]releaseArtifact `json:"artifacts"`
}

func ResolveReleaseVersion(ctx context.Context, channel string) (string, error) {
	baseURL := releaseBaseURL(channel)
	overrideName := "DEPLOYCRATE_STABLE_BASE_URL"
	if channel == UpdateChannelEdge {
		overrideName = "DEPLOYCRATE_EDGE_BASE_URL"
	}
	if override := strings.TrimRight(os.Getenv(overrideName), "/"); override != "" {
		baseURL = override
	}
	manifest, err := loadReleaseManifest(ctx, channel, baseURL)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(manifest.Version), nil
}

func loadReleaseManifest(
	ctx context.Context,
	channel, baseURL string,
) (releaseManifest, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/manifest.json", nil)
	if err != nil {
		return releaseManifest{}, err
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return releaseManifest{}, fmt.Errorf("load %s release manifest: %w", channel, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return releaseManifest{}, fmt.Errorf("load %s release manifest: status %d", channel, response.StatusCode)
	}
	var manifest releaseManifest
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&manifest); err != nil {
		return releaseManifest{}, fmt.Errorf("decode %s release manifest: %w", channel, err)
	}
	if manifest.SchemaVersion != 1 || manifest.Channel != channel ||
		strings.TrimSpace(manifest.Version) == "" {
		return releaseManifest{}, fmt.Errorf("invalid %s release manifest", channel)
	}
	return manifest, nil
}

func acquireReleaseApplicationBinary(
	ctx context.Context,
	channel, expectedVersion string,
) (string, func(), error) {
	overrideName := "DEPLOYCRATE_STABLE_BASE_URL"
	if channel == UpdateChannelEdge {
		overrideName = "DEPLOYCRATE_EDGE_BASE_URL"
	}
	baseURL := strings.TrimRight(os.Getenv(overrideName), "/")
	if baseURL == "" {
		baseURL = releaseBaseURL(channel)
	}
	manifest, err := loadReleaseManifest(ctx, channel, baseURL)
	if err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(manifest.Version) != strings.TrimSpace(expectedVersion) {
		return "", nil, errors.New("selected channel advanced; restart bootstrap to use the new release")
	}
	artifact, found := manifest.Artifacts[runtime.GOOS+"/"+runtime.GOARCH]
	if !found || !strings.HasPrefix(artifact.URL, baseURL+"/") {
		return "", nil, fmt.Errorf("release manifest does not contain this platform")
	}
	expectedChecksum, err := hex.DecodeString(artifact.SHA256)
	if err != nil || len(expectedChecksum) != sha256.Size {
		return "", nil, fmt.Errorf("release manifest contains an invalid checksum")
	}
	binaryURL := artifact.URL
	client := &http.Client{Timeout: 10 * time.Minute}

	temporary, err := os.CreateTemp("", "deploycrate-ce-release-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temporary application binary: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() { _ = os.Remove(temporaryPath) }

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, binaryURL, nil)
	if err != nil {
		_ = temporary.Close()
		cleanup()
		return "", nil, fmt.Errorf("create application binary request: %w", err)
	}
	request.Header.Set("User-Agent", "deploycrate-development-installer")
	response, err := client.Do(request)
	if err != nil {
		_ = temporary.Close()
		cleanup()
		return "", nil, fmt.Errorf("download release application binary: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_ = temporary.Close()
		cleanup()
		return "", nil, fmt.Errorf(
			"download release application binary: status %d",
			response.StatusCode,
		)
	}

	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(temporary, hash), response.Body)
	closeErr := temporary.Close()
	if copyErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("download release application binary: %w", copyErr)
	}
	if closeErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("close release application binary: %w", closeErr)
	}
	if !bytes.Equal(hash.Sum(nil), expectedChecksum) {
		cleanup()
		return "", nil, fmt.Errorf("release application checksum verification failed")
	}

	return temporaryPath, cleanup, nil
}
