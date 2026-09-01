package releaseauth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	CosignVersion        = "3.0.6"
	CosignInstallPath    = "/usr/local/lib/deploycrate-ce/cosign"
	SigstoreOIDCIssuer   = "https://token.actions.githubusercontent.com"
	maxBundleSize        = 1 << 20
	maxCosignBinarySize  = 256 << 20
	maximumVerifierError = 2048
)

var (
	stableVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	edgeVersionPattern   = regexp.MustCompile(`^edge-[0-9a-f]{12}$`)
	cosignSHA256         = map[string]string{
		"amd64": "c956e5dfcac53d52bcf058360d579472f0c1d2d9b69f55209e256fe7783f4c74",
		"arm64": "bedac92e8c3729864e13d4a17048007cfafa79d5deca993a43a90ffe018ef2b8",
	}
)

type Verifier interface {
	VerifyBytes(
		context.Context,
		[]byte,
		string,
		string,
		string,
	) error
	VerifyFile(
		context.Context,
		string,
		string,
		string,
		string,
	) error
}

type SigstoreVerifier struct {
	client *http.Client
}

func New(client *http.Client) *SigstoreVerifier {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &SigstoreVerifier{client: client}
}

func TrustedWorkflowIdentity(channel, version string) (string, error) {
	switch channel {
	case "stable":
		if !stableVersionPattern.MatchString(version) {
			return "", errors.New("stable release version is invalid")
		}
		return "https://github.com/mbvlabs/deploycrate-ce/.github/workflows/" +
			"publish-stable.yml@refs/tags/v" + version, nil
	case "edge":
		if !edgeVersionPattern.MatchString(version) {
			return "", errors.New("edge release version is invalid")
		}
		return "https://github.com/mbvlabs/deploycrate-ce/.github/workflows/" +
			"publish-edge.yml@refs/heads/master", nil
	default:
		return "", errors.New("release channel is invalid")
	}
}

func (v *SigstoreVerifier) VerifyBytes(
	ctx context.Context,
	content []byte,
	bundleURL, channel, version string,
) error {
	temporary, err := os.CreateTemp("", "deploycrate-release-manifest-*")
	if err != nil {
		return fmt.Errorf("create release verification input: %w", err)
	}
	path := temporary.Name()
	defer os.Remove(path)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect release verification input: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write release verification input: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close release verification input: %w", err)
	}
	return v.VerifyFile(ctx, path, bundleURL, channel, version)
}

func (v *SigstoreVerifier) VerifyFile(
	ctx context.Context,
	artifactPath, bundleURL, channel, version string,
) error {
	identity, err := TrustedWorkflowIdentity(channel, version)
	if err != nil {
		return err
	}
	bundlePath, removeBundle, err := v.downloadBundle(ctx, bundleURL)
	if err != nil {
		return err
	}
	defer removeBundle()

	cosignPath, removeCosign, err := v.acquireCosign(ctx)
	if err != nil {
		return err
	}
	defer removeCosign()

	command := exec.CommandContext(
		ctx,
		cosignPath,
		"verify-blob",
		artifactPath,
		"--bundle",
		bundlePath,
		"--certificate-identity",
		identity,
		"--certificate-oidc-issuer",
		SigstoreOIDCIssuer,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if len(detail) > maximumVerifierError {
			detail = detail[:maximumVerifierError]
		}
		if detail == "" {
			return fmt.Errorf("verify release signature: %w", err)
		}
		return fmt.Errorf("verify release signature: %w: %s", err, detail)
	}
	return nil
}

func (v *SigstoreVerifier) downloadBundle(
	ctx context.Context,
	bundleURL string,
) (string, func(), error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, bundleURL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("create release signature request: %w", err)
	}
	response, err := v.client.Do(request)
	if err != nil {
		return "", nil, fmt.Errorf("download release signature: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf(
			"download release signature: status %d",
			response.StatusCode,
		)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxBundleSize+1))
	if err != nil {
		return "", nil, fmt.Errorf("read release signature: %w", err)
	}
	if len(content) > maxBundleSize {
		return "", nil, errors.New("release signature exceeds the size limit")
	}

	temporary, err := os.CreateTemp("", "deploycrate-release-signature-*.sigstore.json")
	if err != nil {
		return "", nil, fmt.Errorf("create release signature file: %w", err)
	}
	path := temporary.Name()
	cleanup := func() { _ = os.Remove(path) }
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		cleanup()
		return "", nil, fmt.Errorf("protect release signature file: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		cleanup()
		return "", nil, fmt.Errorf("write release signature file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close release signature file: %w", err)
	}
	return path, cleanup, nil
}

func (v *SigstoreVerifier) acquireCosign(
	ctx context.Context,
) (string, func(), error) {
	expectedDigest, found := cosignSHA256[runtime.GOARCH]
	if runtime.GOOS != "linux" || !found {
		return "", nil, fmt.Errorf(
			"release signature verification is unsupported on %s/%s",
			runtime.GOOS,
			runtime.GOARCH,
		)
	}
	if _, err := os.Lstat(CosignInstallPath); err == nil {
		if err := verifyFileSHA256(CosignInstallPath, expectedDigest); err != nil {
			return "", nil, fmt.Errorf("verify installed cosign: %w", err)
		}
		return CosignInstallPath, func() {}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", nil, fmt.Errorf("inspect installed cosign: %w", err)
	}

	temporary, err := os.CreateTemp("", "deploycrate-cosign-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temporary cosign binary: %w", err)
	}
	path := temporary.Name()
	cleanup := func() { _ = os.Remove(path) }
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"https://github.com/sigstore/cosign/releases/download/v"+CosignVersion+
			"/cosign-linux-"+runtime.GOARCH,
		nil,
	)
	if err != nil {
		_ = temporary.Close()
		cleanup()
		return "", nil, fmt.Errorf("create cosign download request: %w", err)
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		_ = temporary.Close()
		cleanup()
		return "", nil, fmt.Errorf("download cosign: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_ = temporary.Close()
		cleanup()
		return "", nil, fmt.Errorf("download cosign: status %d", response.StatusCode)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(
		io.MultiWriter(temporary, hash),
		io.LimitReader(response.Body, maxCosignBinarySize+1),
	)
	closeErr := temporary.Close()
	if copyErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("download cosign: %w", copyErr)
	}
	if closeErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("close cosign binary: %w", closeErr)
	}
	if written > maxCosignBinarySize {
		cleanup()
		return "", nil, errors.New("cosign binary exceeds the size limit")
	}
	expected, err := decodeDigest(expectedDigest)
	if err != nil {
		cleanup()
		return "", nil, errors.New("cosign checksum verification failed")
	}
	if !bytes.Equal(hash.Sum(nil), expected) {
		cleanup()
		return "", nil, errors.New("cosign checksum verification failed")
	}
	if err := os.Chmod(path, 0o700); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("make cosign executable: %w", err)
	}
	return path, cleanup, nil
}

func verifyFileSHA256(path, expectedDigest string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("cosign path is not a regular file")
	}
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	expected, err := decodeDigest(expectedDigest)
	if err != nil {
		return errors.New("cosign checksum verification failed")
	}
	if !bytes.Equal(hash.Sum(nil), expected) {
		return errors.New("cosign checksum verification failed")
	}
	return nil
}

func decodeDigest(value string) ([]byte, error) {
	digest, err := hex.DecodeString(value)
	if err != nil {
		return nil, err
	}
	if len(digest) != sha256.Size {
		return nil, errors.New("digest has invalid length")
	}
	return digest, nil
}
