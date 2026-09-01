package releaseauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTrustedWorkflowIdentity(t *testing.T) {
	tests := []struct {
		name    string
		channel string
		version string
		want    string
		wantErr bool
	}{
		{
			name:    "stable",
			channel: "stable",
			version: "1.2.3",
			want: "https://github.com/mbvlabs/deploycrate-ce/.github/workflows/" +
				"publish-stable.yml@refs/tags/v1.2.3",
		},
		{
			name:    "edge",
			channel: "edge",
			version: "edge-012345abcdef",
			want: "https://github.com/mbvlabs/deploycrate-ce/.github/workflows/" +
				"publish-edge.yml@refs/heads/master",
		},
		{name: "invalid stable", channel: "stable", version: "latest", wantErr: true},
		{name: "invalid edge", channel: "edge", version: "edge-main", wantErr: true},
		{name: "invalid channel", channel: "preview", version: "1.2.3", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := TrustedWorkflowIdentity(test.channel, test.version)
			if (err != nil) != test.wantErr {
				t.Fatalf("TrustedWorkflowIdentity() error = %v, wantErr %t", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("TrustedWorkflowIdentity() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestVerifyFileRejectsInvalidIdentityBeforeDownloadingBundle(t *testing.T) {
	requested := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requested = true
	}))
	defer server.Close()

	artifact := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(artifact, []byte("artifact"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	verifier := New(server.Client())
	if err := verifier.VerifyFile(
		context.Background(),
		artifact,
		server.URL+"/bundle",
		"stable",
		"latest",
	); err == nil {
		t.Fatal("VerifyFile() accepted an invalid stable identity")
	}
	if requested {
		t.Fatal("VerifyFile() downloaded a bundle before validating the trusted identity")
	}
}

func TestDownloadBundleRejectsOversizedContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(make([]byte, maxBundleSize+1))
	}))
	defer server.Close()

	verifier := New(server.Client())
	if _, cleanup, err := verifier.downloadBundle(
		context.Background(),
		server.URL,
	); err == nil {
		cleanup()
		t.Fatal("downloadBundle() accepted oversized content")
	}
}

func TestVerifyFileSHA256RejectsWrongDigestAndSymlink(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "cosign")
	content := []byte("pinned verifier")
	if err := os.WriteFile(path, content, 0o700); err != nil {
		t.Fatalf("write verifier: %v", err)
	}
	digest := sha256.Sum256(content)
	if err := verifyFileSHA256(path, hex.EncodeToString(digest[:])); err != nil {
		t.Fatalf("verifyFileSHA256() error = %v", err)
	}
	if err := verifyFileSHA256(path, string(make([]byte, sha256.Size*2))); err == nil {
		t.Fatal("verifyFileSHA256() accepted the wrong digest")
	}

	symlink := filepath.Join(directory, "cosign-link")
	if err := os.Symlink(path, symlink); err != nil {
		t.Fatalf("create verifier symlink: %v", err)
	}
	if err := verifyFileSHA256(symlink, hex.EncodeToString(digest[:])); err == nil {
		t.Fatal("verifyFileSHA256() accepted a symlink")
	}
}
