package services

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"deploycrate-ce/config"
	"deploycrate-ce/internal/releaseauth"
)

type selfUpdateReleaseVerifier struct {
	manifestError error
}

func (v selfUpdateReleaseVerifier) VerifyBytes(
	context.Context,
	[]byte,
	string,
	string,
	string,
) error {
	return v.manifestError
}

func (selfUpdateReleaseVerifier) VerifyFile(
	context.Context,
	string,
	string,
	string,
	string,
) error {
	return nil
}

var _ releaseauth.Verifier = selfUpdateReleaseVerifier{}

func TestStableVersionNewer(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		current   string
		want      bool
	}{
		{name: "patch", candidate: "1.2.4", current: "1.2.3", want: true},
		{name: "minor", candidate: "1.3.0", current: "1.2.9", want: true},
		{name: "major", candidate: "2.0.0", current: "1.9.9", want: true},
		{name: "same", candidate: "1.2.3", current: "1.2.3"},
		{name: "older", candidate: "1.2.2", current: "1.2.3"},
		{name: "v prefix", candidate: "v1.2.4", current: "v1.2.3", want: true},
		{name: "invalid candidate", candidate: "latest", current: "1.2.3"},
		{name: "invalid current", candidate: "1.2.3", current: "dev"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := stableVersionNewer(test.candidate, test.current); got != test.want {
				t.Fatalf(
					"stableVersionNewer(%q, %q) = %t, want %t",
					test.candidate,
					test.current,
					got,
					test.want,
				)
			}
		})
	}
}

func TestFetchReleaseManifest(t *testing.T) {
	platform := runtime.GOOS + "/" + runtime.GOARCH
	manifest := releaseManifest{
		SchemaVersion: 1,
		Channel:       config.ReleaseChannelStable,
		Version:       "1.2.3",
		Artifacts: map[string]releaseArtifact{
			platform: {SHA256: strings.Repeat("a", sha256.Size*2)},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(writer).Encode(manifest); err != nil {
			t.Errorf("encode manifest: %v", err)
		}
	}))
	defer server.Close()

	manifest.Artifacts[platform] = releaseArtifact{
		URL:    server.URL + "/releases/1.2.3/linux/" + runtime.GOARCH + "/deploycrate-ce",
		SHA256: strings.Repeat("a", sha256.Size*2),
	}
	service := &SelfUpdate{
		client:          &http.Client{Timeout: time.Second},
		releaseVerifier: selfUpdateReleaseVerifier{},
	}
	source := config.ReleaseSource{BaseURL: server.URL, Channel: config.ReleaseChannelStable}

	got, err := service.fetchReleaseManifest(context.Background(), source)
	if err != nil {
		t.Fatalf("fetchReleaseManifest() error = %v", err)
	}
	if got.Version != manifest.Version {
		t.Fatalf("fetchReleaseManifest() version = %q, want %q", got.Version, manifest.Version)
	}

	manifest.Artifacts[platform] = releaseArtifact{
		URL:    server.URL + "/releases/1.2.3/linux/" + runtime.GOARCH + "/deploycrate-ce",
		SHA256: strings.Repeat("z", sha256.Size*2),
	}
	if _, err := service.fetchReleaseManifest(context.Background(), source); err == nil {
		t.Fatal("fetchReleaseManifest() accepted a non-hex checksum")
	}

	manifest.Artifacts[platform] = releaseArtifact{
		URL:    "https://releases.example.com/deploycrate-ce",
		SHA256: strings.Repeat("a", sha256.Size*2),
	}
	if _, err := service.fetchReleaseManifest(context.Background(), source); err == nil {
		t.Fatal("fetchReleaseManifest() accepted a cross-origin artifact")
	}

	manifest.Artifacts[platform] = releaseArtifact{
		URL:    server.URL + "/releases/1.2.3/linux/" + runtime.GOARCH + "/deploycrate-ce",
		SHA256: strings.Repeat("a", sha256.Size*2),
	}
	service.releaseVerifier = selfUpdateReleaseVerifier{
		manifestError: errors.New("invalid signature"),
	}
	if _, err := service.fetchReleaseManifest(context.Background(), source); err == nil {
		t.Fatal("fetchReleaseManifest() accepted an unauthenticated manifest")
	}
}
