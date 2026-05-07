package data

import (
	"testing"

	gh "github.com/cli/go-gh/v2/pkg/api"
	"github.com/stretchr/testify/require"
)

func TestClearEnrichmentCache(t *testing.T) {
	// Save original state
	originalCachedClient := cachedClient
	defer func() {
		cachedClient = originalCachedClient
	}()

	t.Run("clears nil cache without panic", func(t *testing.T) {
		cachedClient = nil
		require.True(t, IsEnrichmentCacheCleared(), "cache should be cleared initially")

		ClearEnrichmentCache()
		require.True(t, IsEnrichmentCacheCleared(), "cache should remain cleared")
	})

	t.Run("clears non-nil cache", func(t *testing.T) {
		// Simulate having a cached client (we use an empty struct pointer
		// since we can't create a real GraphQL client without credentials)
		cachedClient = &gh.GraphQLClient{}
		require.False(
			t,
			IsEnrichmentCacheCleared(),
			"cache should not be cleared when client is set",
		)

		ClearEnrichmentCache()
		require.True(
			t,
			IsEnrichmentCacheCleared(),
			"cache should be cleared after ClearEnrichmentCache",
		)
	})
}

func TestIsEnrichmentCacheCleared(t *testing.T) {
	// Save original state
	originalCachedClient := cachedClient
	defer func() {
		cachedClient = originalCachedClient
	}()

	t.Run("returns true when cache is nil", func(t *testing.T) {
		cachedClient = nil
		require.True(t, IsEnrichmentCacheCleared())
	})

	t.Run("returns false when cache is set", func(t *testing.T) {
		cachedClient = &gh.GraphQLClient{}
		require.False(t, IsEnrichmentCacheCleared())
	})
}

func TestSetClient(t *testing.T) {
	// Save original state
	originalClient := client
	originalCachedClient := cachedClient
	defer func() {
		client = originalClient
		cachedClient = originalCachedClient
	}()

	t.Run("sets both client and cachedClient", func(t *testing.T) {
		client = nil
		cachedClient = nil

		// SetClient with nil should set both to nil
		SetClient(nil)
		require.Nil(t, client)
		require.True(t, IsEnrichmentCacheCleared())
	})
}

func TestExtractQueuedFilter(t *testing.T) {
	tests := []struct {
		name      string
		filter    string
		wantClean string
		wantMode  QueuedMode
	}{
		{
			name:      "empty filter",
			filter:    "",
			wantClean: "",
			wantMode:  QueuedAny,
		},
		{
			name:      "no queued token",
			filter:    "is:open author:@me",
			wantClean: "is:open author:@me",
			wantMode:  QueuedAny,
		},
		{
			name:      "is:queued only",
			filter:    "is:queued",
			wantClean: "",
			wantMode:  QueuedOnly,
		},
		{
			name:      "negated is:queued only",
			filter:    "-is:queued",
			wantClean: "",
			wantMode:  QueuedExcluded,
		},
		{
			name:      "is:queued mixed with other tokens",
			filter:    "author:@me is:queued label:bug",
			wantClean: "author:@me label:bug",
			wantMode:  QueuedOnly,
		},
		{
			name:      "negated mixed with other tokens",
			filter:    "is:open -is:queued label:wip",
			wantClean: "is:open label:wip",
			wantMode:  QueuedExcluded,
		},
		{
			name:      "both tokens last wins (only)",
			filter:    "-is:queued is:queued",
			wantClean: "",
			wantMode:  QueuedOnly,
		},
		{
			name:      "both tokens last wins (excluded)",
			filter:    "is:queued -is:queued",
			wantClean: "",
			wantMode:  QueuedExcluded,
		},
		{
			name:      "multiple is:queued occurrences",
			filter:    "is:queued repo:foo/bar is:queued",
			wantClean: "repo:foo/bar",
			wantMode:  QueuedOnly,
		},
		{
			name:      "extra whitespace is normalized",
			filter:    "  is:open   is:queued  ",
			wantClean: "is:open",
			wantMode:  QueuedOnly,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotClean, gotMode := ExtractQueuedFilter(tt.filter)
			require.Equal(t, tt.wantClean, gotClean)
			require.Equal(t, tt.wantMode, gotMode)
		})
	}
}
