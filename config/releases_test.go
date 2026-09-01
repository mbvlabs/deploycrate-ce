package config

import (
	"errors"
	"testing"
)

func TestReleaseSourceForChannel(t *testing.T) {
	tests := []struct {
		name    string
		channel string
		envURL  string
		envChan string
		want    ReleaseSource
		wantErr error
	}{
		{
			name:    "stable channel",
			channel: ReleaseChannelStable,
			want: ReleaseSource{
				BaseURL: StableReleaseBaseURL,
				Channel: ReleaseChannelStable,
			},
		},
		{
			name:    "edge channel",
			channel: ReleaseChannelEdge,
			want: ReleaseSource{
				BaseURL: EdgeReleaseBaseURL,
				Channel: ReleaseChannelEdge,
			},
		},
		{
			name:    "custom mirror only applies to matching env channel",
			channel: ReleaseChannelEdge,
			envURL:  "https://releases.example.com",
			envChan: ReleaseChannelStable,
			want: ReleaseSource{
				BaseURL: EdgeReleaseBaseURL,
				Channel: ReleaseChannelEdge,
			},
		},
		{
			name:    "invalid channel",
			channel: "preview",
			wantErr: ErrReleaseSourceUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("DEPLOYCRATE_CE_RELEASE_BASE_URL", test.envURL)
			t.Setenv("DEPLOYCRATE_CE_UPDATE_CHANNEL", test.envChan)

			got, err := ReleaseSourceForChannel(test.channel)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ReleaseSourceForChannel() error = %v, want %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("ReleaseSourceForChannel() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestFormatReleaseVersion(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{version: "1.2.3", want: "v1.2.3"},
		{version: "v1.2.3", want: "v1.2.3"},
		{version: "edge-4cff03ac8c01", want: "edge-4cff03ac8c01"},
		{version: "development-deadbeef", want: "development-deadbeef"},
		{version: "dev", want: "dev"},
	}

	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			if got := FormatReleaseVersion(test.version); got != test.want {
				t.Fatalf("FormatReleaseVersion(%q) = %q, want %q", test.version, got, test.want)
			}
		})
	}
}

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
