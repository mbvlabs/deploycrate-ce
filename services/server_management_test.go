package services

import "testing"

func TestDockerPruneResource(t *testing.T) {
	tests := []struct {
		scope        string
		wantResource string
		wantOK       bool
	}{
		{scope: "containers", wantResource: "container", wantOK: true},
		{scope: "images", wantResource: "image", wantOK: true},
		{scope: "volumes", wantResource: "volume", wantOK: true},
		{scope: "container"},
		{scope: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.scope, func(t *testing.T) {
			resource, ok := dockerPruneResource(test.scope)
			if resource != test.wantResource || ok != test.wantOK {
				t.Fatalf(
					"dockerPruneResource(%q) = (%q, %t); want (%q, %t)",
					test.scope,
					resource,
					ok,
					test.wantResource,
					test.wantOK,
				)
			}
		})
	}
}
