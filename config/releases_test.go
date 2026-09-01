package config

import (
	"errors"
	"testing"
)

func TestResolveReleaseSource(t *testing.T) {
	tests := []struct {
		name    string
		version string
		channel string
		baseURL string
		want    ReleaseSource
		wantErr error
	}{
		{
			name:    "stable version defaults to stable",
			version: "v1.2.3",
			want: ReleaseSource{
				BaseURL: StableReleaseBaseURL,
				Channel: ReleaseChannelStable,
			},
		},
		{
			name:    "development version defaults to edge",
			version: "development-deadbeef",
			want: ReleaseSource{
				BaseURL: EdgeReleaseBaseURL,
				Channel: ReleaseChannelEdge,
			},
		},
		{
			name:    "edge version defaults to edge",
			version: "edge-deadbeef",
			want: ReleaseSource{
				BaseURL: EdgeReleaseBaseURL,
				Channel: ReleaseChannelEdge,
			},
		},
		{
			name:    "explicit channel and mirror are normalized",
			version: "dev",
			channel: " STABLE ",
			baseURL: "https://releases.example.com/",
			want: ReleaseSource{
				BaseURL: "https://releases.example.com",
				Channel: ReleaseChannelStable,
			},
		},
		{
			name:    "invalid channel is rejected",
			version: "1.2.3",
			channel: "preview",
			baseURL: "https://releases.example.com",
			wantErr: ErrReleaseSourceUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("DEPLOYCRATE_CE_UPDATE_CHANNEL", test.channel)
			t.Setenv("DEPLOYCRATE_CE_RELEASE_BASE_URL", test.baseURL)

			got, err := ResolveReleaseSource(test.version)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ResolveReleaseSource() error = %v, want %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("ResolveReleaseSource() = %#v, want %#v", got, test.want)
			}
		})
	}
}
