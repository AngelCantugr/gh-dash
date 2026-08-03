package fuzzyselect

import tea "charm.land/bubbletea/v2"

type Context struct {
	Start   tea.Position
	End     tea.Position
	Content string
}

type LoaderContext struct {
	RepoOwner string
	RepoName  string
}

// HasRepo reports whether the context is scoped to a single repository.
// Sections that span repositories — notifications, or PR/issue filters without a
// repo: qualifier — leave both fields empty. Repo-scoped lookups are meaningless
// there and resolve to the literal "/", so sources must skip them.
func (c LoaderContext) HasRepo() bool {
	return c.RepoOwner != "" && c.RepoName != ""
}

// Sources can load suggestions, return them based on the cursor position and insert them.
type Source interface {
	// ExtractContext returns a context with the current word under the cursor, where
	// it starts and ends.
	// This helps with knowing how to autocomplete the current word.
	ExtractContext(input string, cursorPos tea.Position) Context
	// TODO: use Inserter and remove it from Source
	InsertSuggestion(
		input string,
		suggestion string,
		contextStart tea.Position,
		contextEnd tea.Position,
	) (newInput string, newCursorPos tea.Position)
	ItemsToExclude(input string, cursorPos tea.Position) []string
	Suggestions(input string, cursorPos tea.Position) []Suggestion
	LoadSuggestions(ctx LoaderContext) error
}
