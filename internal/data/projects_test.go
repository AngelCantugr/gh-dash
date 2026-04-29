package data

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/persistcache"
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
				{Kind: OwnerKindOrg, Login: "test-org"},
				{Kind: OwnerKindUser, Login: "test-user"},
			},
			filters:   ProjectFilters{},
			wantCount: 3, // PVT_001 (shared), PVT_004, PVT_005 — PVT_001 deduplicated
			wantIDs:   []string{"PVT_001", "PVT_004", "PVT_005"},
		},
		{
			name: "missing fixture file is silently skipped",
			owners: []OwnerRef{
				{Kind: OwnerKindOrg, Login: "no-such-org"},
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

// TestFetchProjectsCacheHit verifies that fetchViewerProjects and fetchOwnerProjects
// return pre-populated cache data without making a network call (client stays nil).
func TestFetchProjectsCacheHit(t *testing.T) {
	// Redirect the cache to a temp directory so we don't pollute the real cache.
	t.Setenv("HOME", t.TempDir())
	store, err := persistcache.New()
	require.NoError(t, err)

	viewerProjects := []ProjectData{
		{
			ID:    "CACHE_HIT_001",
			Title: "Cached viewer project",
			Owner: OwnerRef{Kind: OwnerKindUser, Login: "viewer"},
		},
	}
	data, err := json.Marshal(viewerProjects)
	require.NoError(t, err)
	require.NoError(t, store.Put("projects/viewer", data, time.Hour))

	// fetchViewerProjects returns cached data; client is nil and must not be called.
	got, err := fetchViewerProjects(store)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "CACHE_HIT_001", got[0].ID)
}

// TestFetchOwnerProjectsCacheHit verifies per-owner cache hit for an org owner.
func TestFetchOwnerProjectsCacheHit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := persistcache.New()
	require.NoError(t, err)

	orgOwner := OwnerRef{Kind: OwnerKindOrg, Login: "cached-org"}
	cachedProjects := []ProjectData{
		{ID: "CACHE_ORG_001", Title: "Org cached", Owner: orgOwner},
		{ID: "CACHE_ORG_002", Title: "Org cached 2", Owner: orgOwner},
	}
	data, err := json.Marshal(cachedProjects)
	require.NoError(t, err)
	require.NoError(t, store.Put("projects/org/cached-org", data, time.Hour))

	// fetchOwnerProjects returns cached data; client is nil and must not be called.
	got, err := fetchOwnerProjects(store, orgOwner)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "CACHE_ORG_001", got[0].ID)
}
