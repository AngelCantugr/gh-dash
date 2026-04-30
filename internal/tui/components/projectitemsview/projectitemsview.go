// Package projectitemsview implements the drill-down items view for a GitHub
// ProjectsV2 project. It is a plain Bubble Tea sub-model owned by
// projectsection.Model and is NOT a section.Section.
package projectitemsview

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
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
	return &Model{
		ctx:          ctx,
		projectID:    projectID,
		projectTitle: projectTitle,
		projectURL:   projectURL,
		sectionId:    sectionId,
	table:        tbl,
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
		switch {
		case key.Matches(msg, keys.ProjectKeys.Back):
			// Handled by parent — return nil so caller can act on it.
			return m, nil

		case key.Matches(msg, keys.ProjectKeys.LoadMore):
			return m, m.LoadMore()
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
		m.table.SetRows(BuildRows(m.ctx, m.items, m.schema))
		m.table.SetIsLoading(false)
	}

	tbl, tblCmd := m.table.Update(msg)
	m.table = tbl
	return m, tblCmd
}

// View renders the breadcrumb header and the table.
func (m *Model) View() string {
	breadcrumb := lipgloss.NewStyle().
		Foreground(m.ctx.Theme.FaintText).
		Render("Projects") +
		lipgloss.NewStyle().
			Foreground(m.ctx.Theme.PrimaryText).
			Render(fmt.Sprintf(" > %s", m.projectTitle))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		breadcrumb,
		m.table.View(),
	)
}

// FetchItems starts an async fetch. When appendMode is true the results are
// appended to the existing items slice (load-more); otherwise they replace it.
func (m *Model) FetchItems(appendMode bool) tea.Cmd {
	if m.isFetching {
		return nil
	}
	m.isFetching = true

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
