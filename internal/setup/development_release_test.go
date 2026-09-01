package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"deploycrate-ce/internal/releaseauth"
)

type setupReleaseVerifier struct {
	manifestError error
	artifactError error
}

func (v setupReleaseVerifier) VerifyBytes(
	context.Context,
	[]byte,
	string,
	string,
	string,
) error {
	return v.manifestError
}

func (v setupReleaseVerifier) VerifyFile(
	context.Context,
	string,
	string,
	string,
	string,
) error {
	return v.artifactError
}

var _ releaseauth.Verifier = setupReleaseVerifier{}

func TestAcquireReleaseApplicationBinary(t *testing.T) {
	binary := []byte("release-binary")
	digest := sha256.Sum256(binary)
	manifest := releaseManifest{
		SchemaVersion: 1,
		Channel:       UpdateChannelStable,
		Version:       "1.2.3",
		Artifacts:     map[string]releaseArtifact{},
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/manifest.json":
			if err := json.NewEncoder(writer).Encode(manifest); err != nil {
				t.Errorf("encode manifest: %v", err)
			}
		case "/releases/1.2.3/deploycrate-ce":
			if _, err := writer.Write(binary); err != nil {
				t.Errorf("write binary: %v", err)
			}
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	manifest.Artifacts[runtime.GOOS+"/"+runtime.GOARCH] = releaseArtifact{
		URL:    server.URL + "/releases/1.2.3/deploycrate-ce",
		SHA256: hex.EncodeToString(digest[:]),
	}
	t.Setenv("DEPLOYCRATE_STABLE_BASE_URL", server.URL)

	path, cleanup, err := acquireReleaseApplicationBinaryWithVerifier(
		context.Background(),
		UpdateChannelStable,
		manifest.Version,
		setupReleaseVerifier{},
	)
	if err != nil {
		t.Fatalf("acquireReleaseApplicationBinary() error = %v", err)
	}
	t.Cleanup(cleanup)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read acquired binary: %v", err)
	}
	if string(got) != string(binary) {
		t.Fatalf("acquired binary = %q, want %q", got, binary)
	}

	if _, cleanup, err := acquireReleaseApplicationBinaryWithVerifier(
		context.Background(),
		UpdateChannelStable,
		"1.2.2",
		setupReleaseVerifier{},
	); err == nil {
		cleanup()
		t.Fatal("acquireReleaseApplicationBinary() accepted a different manifest version")
	}

	if _, cleanup, err := acquireReleaseApplicationBinaryWithVerifier(
		context.Background(),
		UpdateChannelStable,
		manifest.Version,
		setupReleaseVerifier{manifestError: errors.New("invalid manifest signature")},
	); err == nil {
		cleanup()
		t.Fatal("acquireReleaseApplicationBinary() accepted an unauthenticated manifest")
	}
	if _, cleanup, err := acquireReleaseApplicationBinaryWithVerifier(
		context.Background(),
		UpdateChannelStable,
		manifest.Version,
		setupReleaseVerifier{artifactError: errors.New("invalid artifact signature")},
	); err == nil {
		cleanup()
		t.Fatal("acquireReleaseApplicationBinary() accepted an unauthenticated binary")
	}
}
