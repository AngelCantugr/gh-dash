package fuzzyselect

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/data"
)

// Sections that aren't bound to a single repository (notifications, or PR/issue
// filters without a repo: qualifier) reach LoadSuggestions with an empty owner and
// name. Fetching anyway asks GitHub to resolve the repository "/", whose error is
// rendered verbatim under the search box.
func TestSearchQuerySourceLoadSuggestionsWithoutRepo(t *testing.T) {
	testCases := []struct {
		name string
		ctx  LoaderContext
	}{
		{
			name: "no owner and no name",
			ctx:  LoaderContext{},
		},
		{
			name: "owner without name",
			ctx:  LoaderContext{RepoOwner: "dlvhdr"},
		},
		{
			name: "name without owner",
			ctx:  LoaderContext{RepoName: "gh-dash"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			src := &SearchQuerySource{}

			require.NoError(t, src.LoadSuggestions(tc.ctx))

			require.Empty(t, src.Users)
			require.Empty(t, src.Labels)
			require.NoError(t, src.UsersErr)
			require.NoError(t, src.LabelsErr)
		})
	}
}

// A source outlives a single focus, so results loaded while a repo was in scope
// must not survive into a section that spans repositories.
func TestSearchQuerySourceLoadSuggestionsWithoutRepoClearsPreviousResults(t *testing.T) {
	src := &SearchQuerySource{
		Users:  []data.User{{Login: "dlvhdr", Name: "Dolev"}},
		Labels: []data.Label{{Name: "bug"}},
	}

	require.NoError(t, src.LoadSuggestions(LoaderContext{}))

	require.Empty(t, src.Users)
	require.Empty(t, src.Labels)
}

// @me is not repo-scoped, so author: completion stays useful without a repository
// even though no user names are available.
func TestSearchQuerySourceSuggestsMeWithoutRepo(t *testing.T) {
	src := &SearchQuerySource{}
	require.NoError(t, src.LoadSuggestions(LoaderContext{}))

	input := "author:"
	got := src.Suggestions(input, tea.Position{X: len(input)})

	require.Equal(t, []Suggestion{{Value: "@me", Detail: "Signed-in user"}}, got)
}

func TestSearchQuerySourceSuggestsNoLabelsWithoutRepo(t *testing.T) {
	src := &SearchQuerySource{}
	require.NoError(t, src.LoadSuggestions(LoaderContext{}))

	input := "label:"
	require.Empty(t, src.Suggestions(input, tea.Position{X: len(input)}))
}

func TestUserMentionSourceLoadSuggestionsWithoutRepo(t *testing.T) {
	src := &UserMentionSource{Users: []data.User{{Login: "dlvhdr"}}}

	require.NoError(t, src.LoadSuggestions(LoaderContext{}))

	require.Empty(t, src.Users)
	require.NoError(t, src.Err)
}

func TestLabelSourceLoadSuggestionsWithoutRepo(t *testing.T) {
	src := &LabelSource{Labels: []data.Label{{Name: "bug"}}}

	require.NoError(t, src.LoadSuggestions(LoaderContext{}))

	require.Empty(t, src.Labels)
}

func TestLoaderContextHasRepo(t *testing.T) {
	require.True(t, LoaderContext{RepoOwner: "dlvhdr", RepoName: "gh-dash"}.HasRepo())
	require.False(t, LoaderContext{}.HasRepo())
	require.False(t, LoaderContext{RepoOwner: "dlvhdr"}.HasRepo())
	require.False(t, LoaderContext{RepoName: "gh-dash"}.HasRepo())
}

// A focus that doesn't fetch — an unscoped search — never overwrites the fetch
// status, and ClearFetchStatusMsg only reaches the currently selected section. If
// Reset left the status alone, a failed lookup for a previously scoped repo would
// keep painting its error under an unscoped search box.
func TestResetClearsFetchStatus(t *testing.T) {
	var m Model

	m.SetFetchError(errors.New("could not resolve to a Repository with the name '/'"))
	m.Reset()

	require.Equal(t, FetchStateIdle, m.fetchState)
	require.NoError(t, m.fetchError)

	m.SetFetchLoading()
	m.Reset()

	require.Equal(t, FetchStateIdle, m.fetchState)
}
