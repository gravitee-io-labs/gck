package cmd

import (
	"slices"
	"testing"
)

func TestLiveOthers(t *testing.T) {
	tests := []struct {
		name         string
		stateNames   []string
		kindClusters []string
		target       string
		want         []string
	}{
		{"only the deleted cluster", []string{"gravitee"}, nil, "gravitee", nil},
		{"another cluster runs", []string{"confidence", "gravitee"}, []string{"confidence"}, "gravitee", []string{"confidence"}},
		{"stale state file", []string{"confidence", "gravitee"}, nil, "gravitee", nil},
		{"kind cluster gck does not manage", []string{"gravitee"}, []string{"other"}, "gravitee", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := liveOthers(tc.stateNames, tc.kindClusters, tc.target)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("liveOthers() = %v, want %v", got, tc.want)
			}
		})
	}
}
