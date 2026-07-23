package setup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultDevelopmentReleaseBaseURL = "https://get-dev.deploycrate.com"
	developmentApplicationPath       = "dc-ce-app/deploycrate-ce"
	developmentChecksumLimit         = 4096
)

func acquireDevelopmentApplicationBinary(ctx context.Context) (string, func(), error) {
	baseURL := strings.TrimRight(os.Getenv("DEPLOYCRATE_DEVELOPMENT_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = defaultDevelopmentReleaseBaseURL
	}
	binaryURL := baseURL + "/" + developmentApplicationPath
	checksumURL := binaryURL + ".sha256"
	client := &http.Client{Timeout: 10 * time.Minute}

	expectedChecksum, err := fetchDevelopmentChecksum(ctx, client, checksumURL)
	if err != nil {
		return "", nil, err
	}

	temporary, err := os.CreateTemp("", "deploycrate-ce-development-*")
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
		return "", nil, fmt.Errorf("download development application binary: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_ = temporary.Close()
		cleanup()
		return "", nil, fmt.Errorf("download development application binary: status %d", response.StatusCode)
	}

	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(temporary, hash), response.Body)
	closeErr := temporary.Close()
	if copyErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("download development application binary: %w", copyErr)
	}
	if closeErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("close development application binary: %w", closeErr)
	}
	if !bytes.Equal(hash.Sum(nil), expectedChecksum) {
		cleanup()
		return "", nil, fmt.Errorf("development application checksum verification failed")
	}

	return temporaryPath, cleanup, nil
}

func fetchDevelopmentChecksum(ctx context.Context, client *http.Client, checksumURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create application checksum request: %w", err)
	}
	request.Header.Set("User-Agent", "deploycrate-development-installer")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download development application checksum: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download development application checksum: status %d", response.StatusCode)
	}

	content, err := io.ReadAll(io.LimitReader(response.Body, developmentChecksumLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read development application checksum: %w", err)
	}
	if len(content) > developmentChecksumLimit {
		return nil, fmt.Errorf("development application checksum exceeds %d bytes", developmentChecksumLimit)
	}
	fields := strings.Fields(string(content))
	if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != "deploycrate-ce" {
		return nil, fmt.Errorf("invalid development application checksum file")
	}
	checksum, err := hex.DecodeString(fields[0])
	if err != nil || len(checksum) != sha256.Size {
		return nil, fmt.Errorf("invalid development application SHA-256 checksum")
	}
	return checksum, nil
}
