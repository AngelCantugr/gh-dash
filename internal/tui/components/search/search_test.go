package search

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/cmpcontroller"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

func newTestModel(t *testing.T, value string) Model {
	t.Helper()

	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../../../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	require.NoError(t, err)

	thm := theme.ParseTheme(&cfg)
	ctx := &context.ProgramContext{
		Config: &cfg,
		Theme:  thm,
		Styles: context.InitStyles(thm),
	}

	return NewModel(ctx, SearchOptions{InitialValue: value})
}

// Focus() decides whether to fetch repo-scoped users and labels from this result.
// Reporting a repo that isn't really there sends an unresolvable lookup to GitHub
// and paints its error under the search box.
func TestModelRepo(t *testing.T) {
	testCases := []struct {
		name        string
		value       string
		wantFound   bool
		wantRepoRef cmpcontroller.RepoRef
	}{
		{
			name:      "notifications filter has no repo",
			value:     "is:notification reason:review_requested",
			wantFound: false,
		},
		{
			name:      "unscoped pr filter has no repo",
			value:     "is:open author:@me",
			wantFound: false,
		},
		{
			name:      "empty filter has no repo",
			value:     "",
			wantFound: false,
		},
		{
			name:      "repo without a slash is not a repo ref",
			value:     "repo:dlvhdr is:open",
			wantFound: false,
		},
		{
			name:      "half-typed repo has no name",
			value:     "repo:dlvhdr/ is:open",
			wantFound: false,
		},
		{
			name:      "half-typed repo has no owner",
			value:     "repo:/gh-dash is:open",
			wantFound: false,
		},
		{
			name:      "scoped filter yields the repo",
			value:     "repo:dlvhdr/gh-dash is:open",
			wantFound: true,
			wantRepoRef: cmpcontroller.RepoRef{
				NameWithOwner: "dlvhdr/gh-dash",
				Owner:         "dlvhdr",
				Name:          "gh-dash",
			},
		},
		{
			name:      "repo qualifier is found after other tokens",
			value:     "is:open repo:dlvhdr/gh-dash",
			wantFound: true,
			wantRepoRef: cmpcontroller.RepoRef{
				NameWithOwner: "dlvhdr/gh-dash",
				Owner:         "dlvhdr",
				Name:          "gh-dash",
			},
		},
		{
			// NameWithOwner keys the suggestion caches, and the fetchers store
			// under "owner/name". Carrying extra segments through would produce a
			// key that never matches, so the refresh key would clear nothing.
			name:      "extra path segments do not leak into the cache key",
			value:     "repo:dlvhdr/gh-dash/pulls is:open",
			wantFound: true,
			wantRepoRef: cmpcontroller.RepoRef{
				NameWithOwner: "dlvhdr/gh-dash",
				Owner:         "dlvhdr",
				Name:          "gh-dash",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t, tc.value)

			repo, found := m.Repo()

			require.Equal(t, tc.wantFound, found)
			require.Equal(t, tc.wantRepoRef, repo)
		})
	}
}

// Enter leaves the popup empty on purpose and only a completed fetch filters it.
// An unscoped section has no fetch to wait for, so without an explicit filter the
// popup opens announcing "no results" while @me is sitting right there unoffered.
func TestFocusOnUnscopedSectionOffersRepoIndependentSuggestions(t *testing.T) {
	m := newTestModel(t, "author:")

	m.Focus()

	completions := m.ViewCompletions()
	require.NotEmpty(t, completions)
	require.Contains(t, completions, "@me")
	require.NotContains(t, completions, "no results")
}

// The scoped path still defers to the fetch, so the popup reports progress rather
// than prematurely claiming there is nothing to show.
func TestFocusOnScopedSectionWaitsForTheFetch(t *testing.T) {
	m := newTestModel(t, "repo:dlvhdr/gh-dash author:")

	m.Focus()

	require.NotContains(t, m.ViewCompletions(), "no results")
}
