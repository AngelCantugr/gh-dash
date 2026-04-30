// Package projectitemsview implements the drill-down items view for a GitHub
// ProjectsV2 project. It is a plain Bubble Tea sub-model owned by
// projectsection.Model and is NOT a section.Section.
package projectitemsview

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/persistcache"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/table"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/keys"
)

// Model is a plain Bubble Tea sub-model owned by projectsection.Model.
// It renders project items in a table and manages its own fetch/pagination
// lifecycle. It does NOT implement section.Section.
type Model struct {
	ctx          *context.ProgramContext
	projectID    string
	projectTitle string
	projectURL   string
	schema       *data.ProjectSchema
	items        []data.ProjectItemData
	pageInfo     *data.PageInfo
	table        table.Model
	isFetching   bool
	sectionId    int
	// search state
	isSearching bool
	searchQuery string
	searchInput textinput.Model
}

// ProjectItemsFetchedMsg is returned by the async fetch cmd.
type ProjectItemsFetchedMsg struct {
	Schema   data.ProjectSchema
	Items    []data.ProjectItemData
	PageInfo data.PageInfo
	Err      error
	Append   bool // false = initial load, true = load-more
}

// NewModel creates a new projectitemsview.Model for the given project.
func NewModel(
	ctx *context.ProgramContext,
	sectionId int,
	projectID, projectTitle, projectURL string,
) *Model {
	dims := constants.Dimensions{
		Width:  ctx.MainContentWidth,
		Height: ctx.MainContentHeight,
	}
	emptyState := "No items found."
	tbl := table.NewModel(
		*ctx,
		dims,
		time.Now(),
		time.Now(),
		GetColumns(),
		[]table.Row{},
		"item",
		&emptyState,
		"Fetching project items...",
		true,
	)
	si := textinput.New()
	si.Placeholder = "Search by title..."
	si.Prompt = " / "
	si.Blur()
	return &Model{
		ctx:          ctx,
		projectID:    projectID,
		projectTitle: projectTitle,
		projectURL:   projectURL,
		sectionId:    sectionId,
		table:        tbl,
		searchInput:  si,
	}
}

// Init satisfies the tea.Model convention; callers should use FetchItems.
func (m *Model) Init() tea.Cmd {
	return m.FetchItems(false)
}

// Update handles messages for the items view.
func (m *Model) Update(msg tea.Msg) (*Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Route to search input while searching.
		if m.isSearching {
			switch msg.String() {
			case "esc", "ctrl+c":
				m.isSearching = false
				m.searchQuery = ""
				m.searchInput.SetValue("")
				m.searchInput.Blur()
				m.table.SetRows(BuildRows(m.ctx, m.items, m.schema))
				return m, nil
			case "enter":
				m.isSearching = false
				m.searchInput.Blur()
				return m, nil
			default:
				var siCmd tea.Cmd
				m.searchInput, siCmd = m.searchInput.Update(msg)
				m.searchQuery = m.searchInput.Value()
				m.table.SetRows(BuildRows(m.ctx, m.filteredItems(), m.schema))
				return m, siCmd
			}
		}

		switch {
		case key.Matches(msg, keys.ProjectKeys.Back):
			// Handled by parent — return nil so caller can act on it.
			return m, nil

		case key.Matches(msg, keys.ProjectKeys.LoadMore):
			return m, m.LoadMore()

		case key.Matches(msg, keys.ProjectKeys.Refresh):
			// If already fetching, keep the spinner visible as a no-op hint.
			if m.isFetching {
				m.table.SetIsLoading(true)
				return m, nil
			}
			return m, m.FetchItems(false)

		case key.Matches(msg, keys.Keys.Search):
			m.isSearching = true
			return m, m.searchInput.Focus()
		}

	case ProjectItemsFetchedMsg:
		m.isFetching = false
		if msg.Err != nil {
			return m, nil
		}
		schema := msg.Schema
		m.schema = &schema
		pageInfo := msg.PageInfo
		m.pageInfo = &pageInfo
		if msg.Append {
			m.items = append(m.items, msg.Items...)
		} else {
			m.items = msg.Items
		}
		m.table.SetRows(BuildRows(m.ctx, m.filteredItems(), m.schema))
		m.table.SetIsLoading(false)
	}

	tbl, tblCmd := m.table.Update(msg)
	m.table = tbl
	return m, tblCmd
}

// filteredItems returns items filtered by the current search query (case-insensitive title match).
func (m *Model) filteredItems() []data.ProjectItemData {
	if m.searchQuery == "" {
		return m.items
	}
	q := strings.ToLower(m.searchQuery)
	out := make([]data.ProjectItemData, 0, len(m.items))
	for _, item := range m.items {
		if strings.Contains(strings.ToLower(item.Title), q) {
			out = append(out, item)
		}
	}
	return out
}

// View renders the breadcrumb header, optional search bar, and the table.
func (m *Model) View() string {
	breadcrumb := lipgloss.NewStyle().
		Foreground(m.ctx.Theme.FaintText).
		Render("Projects") +
		lipgloss.NewStyle().
			Foreground(m.ctx.Theme.PrimaryText).
			Render(fmt.Sprintf(" > %s", m.projectTitle))

	parts := []string{breadcrumb}
	if m.isSearching {
		parts = append(parts, m.searchInput.View())
	}
	parts = append(parts, m.table.View())
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// FetchItems starts an async fetch. When appendMode is true the results are
// appended to the existing items slice (load-more); otherwise they replace it.
func (m *Model) FetchItems(appendMode bool) tea.Cmd {
	if m.isFetching {
		m.table.SetIsLoading(true) // keep spinner visible as no-op hint
		return nil
	}
	m.isFetching = true
	m.table.SetIsLoading(true)

	after := ""
	if appendMode && m.pageInfo != nil {
		after = m.pageInfo.EndCursor
	}

	// Capture values for the closure — avoid capturing m directly.
	projectID := m.projectID
	var cache *persistcache.Store
	if m.ctx != nil {
		cache = m.ctx.ProjectsCache
	}

	return func() tea.Msg {
		schema, items, pageInfo, err := data.FetchProjectItems(
			cache,
			projectID,
			after,
			100,
			nil,
		)
		return ProjectItemsFetchedMsg{
			Schema:   schema,
			Items:    items,
			PageInfo: pageInfo,
			Err:      err,
			Append:   appendMode,
		}
	}
}

// LoadMore fetches the next page of items if there is one.
func (m *Model) LoadMore() tea.Cmd {
	if m.pageInfo == nil || !m.pageInfo.HasNextPage {
		return nil
	}
	return m.FetchItems(true)
}

// GetCurrItem returns a pointer to the currently highlighted item.
// Returns nil when the table is empty or has no selection.
func (m *Model) GetCurrItem() *data.ProjectItemData {
	idx := m.table.GetCurrItem()
	if idx < 0 || idx >= len(m.items) {
		return nil
	}
	item := m.items[idx]
	return &item
}

// GetProjectURL returns the URL of the parent project.
func (m *Model) GetProjectURL() string {
	return m.projectURL
}

// UpdateProgramContext syncs the shared context pointer and propagates
// updated dimensions/styles to the table.
func (m *Model) UpdateProgramContext(ctx *context.ProgramContext) {
	m.ctx = ctx
	tblCtx := *ctx
	m.table.SetDimensions(constants.Dimensions{
		Width:  ctx.MainContentWidth,
		Height: ctx.MainContentHeight,
	})
	// table.Model stores a value copy of context; update it via the update helper.
	// We do this by re-setting rows so the internal ctx is also refreshed via Update.
	_ = tblCtx // used to avoid shadowing warning; dimension update above suffices.
}
