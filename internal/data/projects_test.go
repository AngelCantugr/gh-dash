package data

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
)

// boolPtr is a test helper that returns a pointer to a bool literal.
func boolPtr(b bool) *bool { return &b }

func TestFetchProjects(t *testing.T) {
	tests := []struct {
		name      string
		owners    []OwnerRef
		filters   ProjectFilters
		wantCount int
		wantIDs   []string
	}{
		{
			name:      "viewer fallback with nil cache returns all fixture projects",
			owners:    nil,
			filters:   ProjectFilters{},
			wantCount: 3,
		},
		{
			name:      "filter open projects only",
			owners:    nil,
			filters:   ProjectFilters{Closed: boolPtr(false)},
			wantCount: 2,
			wantIDs:   []string{"PVT_001", "PVT_003"},
		},
		{
			name:      "filter closed projects only",
			owners:    nil,
			filters:   ProjectFilters{Closed: boolPtr(true)},
			wantCount: 1,
			wantIDs:   []string{"PVT_002"},
		},
		{
			name:      "filter by title is case-insensitive",
			owners:    nil,
			filters:   ProjectFilters{TitleContains: "roadmap"},
			wantCount: 1,
			wantIDs:   []string{"PVT_001"},
		},
		{
			name:      "filter by title partial match",
			owners:    nil,
			filters:   ProjectFilters{TitleContains: "Q"},
			wantCount: 2,
			wantIDs:   []string{"PVT_001", "PVT_002"},
		},
		{
			name:      "filter with no match returns empty slice",
			owners:    nil,
			filters:   ProjectFilters{TitleContains: "nonexistent-zzz"},
			wantCount: 0,
		},
		{
			name: "deduplication by node ID across multiple owners",
			owners: []OwnerRef{
				{Kind: OwnerOrg, Login: "test-org"},
				{Kind: OwnerUser, Login: "test-user"},
			},
			filters:   ProjectFilters{},
			wantCount: 3, // PVT_001 (shared), PVT_004, PVT_005 — PVT_001 deduplicated
			wantIDs:   []string{"PVT_001", "PVT_004", "PVT_005"},
		},
		{
			name: "missing fixture file is silently skipped",
			owners: []OwnerRef{
				{Kind: OwnerOrg, Login: "no-such-org"},
			},
			filters:   ProjectFilters{},
			wantCount: 0,
		},
		{
			name:      "combined closed and title filter",
			owners:    nil,
			filters:   ProjectFilters{Closed: boolPtr(false), TitleContains: "backlog"},
			wantCount: 1,
			wantIDs:   []string{"PVT_003"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use file-based mock fixtures; nil cache must not cause a panic.
			t.Setenv(config.FF_MOCK_DATA, "1")

			got, err := FetchProjects(nil, tt.owners, tt.filters)

			require.NoError(t, err)
			require.Len(t, got, tt.wantCount)

			if len(tt.wantIDs) > 0 {
				gotIDs := make([]string, len(got))
				for i, p := range got {
					gotIDs[i] = p.ID
				}
				require.ElementsMatch(t, tt.wantIDs, gotIDs)
			}
		})
	}
}
