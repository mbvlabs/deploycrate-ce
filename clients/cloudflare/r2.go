package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const maximumObjectSize = 512 << 20

type R2 struct {
	client *http.Client
}

func NewR2(client *http.Client) *R2 {
	return &R2{client: client}
}

func (r *R2) Download(ctx context.Context, baseURL, objectPath, destination string) error {
	sourceURL, err := joinObjectURL(baseURL, objectPath)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "deploycrate-ce-self-update")

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", sourceURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf(
			"download %s: status %d: %s",
			sourceURL,
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, io.LimitReader(resp.Body, maximumObjectSize+1))
	if copyErr == nil && written > maximumObjectSize {
		copyErr = fmt.Errorf("download %s exceeded the %d-byte limit", sourceURL, maximumObjectSize)
	}
	closeErr := file.Close()
	return errors.Join(copyErr, closeErr)
}

func joinObjectURL(baseURL, objectPath string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("parse Cloudflare R2 base URL: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("Cloudflare R2 release base URL must be an absolute HTTPS URL")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.TrimLeft(objectPath, "/")
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
