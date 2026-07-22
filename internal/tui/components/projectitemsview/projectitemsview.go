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
	"charm.land/log/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/persistcache"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/table"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/keys"
)

// sectionTypeProject is the section type string for projectsection.
// Defined here to avoid an import cycle with projectsection.
const sectionTypeProject = "project"

// Model is a plain Bubble Tea sub-model owned by projectsection.Model.
// It renders project items in a table and manages its own fetch/pagination
// lifecycle. It does NOT implement section.Section.
type Model struct {
	ctx             *context.ProgramContext
	projectID       string
	projectTitle    string
	projectURL      string
	extraFieldNames []string
	schema          *data.ProjectSchema
	items           []data.ProjectItemData
	pageInfo        *data.PageInfo
	table           table.Model
	isFetching      bool
	sectionId       int
	// search state
	isSearching bool
	searchQuery string
	searchInput textinput.Model
	// status picker state
	statusPicker   statuspicker
	pickerItemID   string          // ID of the item whose status is being edited
	previousStatus data.FieldValue // saved status value for rollback on error
}

// ProjectItemsFetchedMsg is returned by the async fetch cmd.
type ProjectItemsFetchedMsg struct {
	Schema   data.ProjectSchema
	Items    []data.ProjectItemData
	PageInfo data.PageInfo
	Err      error
	Append   bool // false = initial load, true = load-more
}

// StatusUpdatedMsg is the inner tea.Msg carried by constants.TaskFinishedMsg
// on a successful status mutation. The UpdatedItem holds the server-authoritative
// item state for reconciliation.
type StatusUpdatedMsg struct {
	ItemID           string
	IntendedOptionID string // option ID that was optimistically applied
	UpdatedItem      data.ProjectItemData
}

// StatusUpdateErrMsg is the inner tea.Msg carried by constants.TaskFinishedMsg
// when a status mutation fails. The view reverts the optimistic change on receipt.
type StatusUpdateErrMsg struct {
	ItemID         string
	PreviousStatus data.FieldValue // value to restore
	Err            error
}

// NewModel creates a new projectitemsview.Model for the given project.
func NewModel(
	ctx *context.ProgramContext,
	sectionId int,
	projectID, projectTitle, projectURL string,
	extraFieldNames []string,
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

	m := &Model{
		ctx:             ctx,
		projectID:       projectID,
		projectTitle:    projectTitle,
		projectURL:      projectURL,
		extraFieldNames: extraFieldNames,
		sectionId:       sectionId,
		table:           tbl,
		searchInput:     si,
	}
	// statusPicker callbacks reference m so they must be set after m is created.
	m.statusPicker = newStatusPicker(ctx, m.onStatusPicked, m.onStatusCancelled)
	return m
}

// Init satisfies the tea.Model convention; callers should use FetchItems.
func (m *Model) Init() tea.Cmd {
	return m.FetchItems(false)
}

// Update handles messages for the items view.
func (m *Model) Update(msg tea.Msg) (*Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// While the status picker is open, route all key events to it.
		if m.statusPicker.IsOpen() {
			updated, cmd := m.statusPicker.Update(msg)
			m.statusPicker = updated
			return m, cmd
		}

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

		case key.Matches(msg, keys.ProjectKeys.EditStatus):
			return m, m.handleEditStatus()
		}

	case StatusUpdatedMsg:
		return m, m.handleStatusUpdated(msg)

	case StatusUpdateErrMsg:
		return m, m.handleStatusUpdateErr(msg)

	case clearStatusErrorMsg:
		// Clear the transient error from the footer after the 2s flash.
		if m.ctx != nil {
			m.ctx.Error = nil
		}
		return m, nil

	case ProjectItemsFetchedMsg:
		m.isFetching = false
		m.table.SetIsLoading(false)
		if msg.Err != nil {
			if m.ctx != nil {
				m.ctx.Error = fmt.Errorf("failed to fetch project items: %w", msg.Err)
			}
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
		m.table.Columns = BuildColumns(m.schema, m.items)
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
// When the status picker is open it is rendered below the table as an overlay.
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
	if pickerView := m.statusPicker.View(); pickerView != "" {
		parts = append(parts, pickerView)
	}
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
	extraFieldNames := m.extraFieldNames
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
			extraFieldNames,
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

// Refresh invalidates this project's cached items and refetches the first
// page from the network. A plain FetchItems(false) would serve the disk cache
// within its TTL, making an explicit refresh a no-op.
func (m *Model) Refresh() tea.Cmd {
	if m.ctx != nil && m.ctx.ProjectsCache != nil {
		cacheKey := "project-items/" + m.projectID
		if err := m.ctx.ProjectsCache.Invalidate(cacheKey); err != nil {
			log.Warn("projectitemsview: refresh cache invalidation", "key", cacheKey, "err", err)
		}
	}
	return m.FetchItems(false)
}

// LoadMore fetches the next page of items if there is one.
func (m *Model) LoadMore() tea.Cmd {
	if m.pageInfo == nil || !m.pageInfo.HasNextPage {
		return nil
	}
	return m.FetchItems(true)
}

// Items returns the current items slice (used by projectsection for NumRows).
func (m *Model) Items() []data.ProjectItemData { return m.items }

// TableCurrItem / TableNextItem / TablePrevItem / TableFirstItem / TableLastItem
// expose the items-view table cursor to projectsection so the root model's
// navigation keys (j/k/g/G) operate on the drill-down table, not the projects list.
func (m *Model) TableCurrItem() int  { return m.table.GetCurrItem() }
func (m *Model) TableNextItem() int  { return m.table.NextItem() }
func (m *Model) TablePrevItem() int  { return m.table.PrevItem() }
func (m *Model) TableFirstItem() int { return m.table.FirstItem() }
func (m *Model) TableLastItem() int  { return m.table.LastItem() }

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

// --------------------------------------------------------------------------
// Status mutation helpers
// --------------------------------------------------------------------------

// handleEditStatus opens the status picker for the currently selected item,
// or surfaces a hint when the project has no Status field.
func (m *Model) handleEditStatus() tea.Cmd {
	if m.schema == nil || m.schema.StatusField == nil {
		// No Status field on this project — surface a footer hint via context error.
		if m.ctx != nil {
			m.ctx.Error = fmt.Errorf("this project has no Status field: edit in web UI")
		}
		return nil
	}

	item := m.GetCurrItem()
	if item == nil {
		return nil
	}

	// Store the item ID and its current status for potential rollback.
	m.pickerItemID = item.ID
	if item.Fields != nil {
		m.previousStatus = item.Fields[m.schema.StatusField.ID]
	}

	m.statusPicker.Open(m.schema.StatusField.Options)
	return nil
}

// onStatusPicked is the statuspicker.onPick callback. It applies the optimistic
// update and fires the async mutation command.
func (m *Model) onStatusPicked(optionID string) tea.Cmd {
	if m.schema == nil || m.schema.StatusField == nil {
		return nil
	}

	// Find the option name for the optimistic display value.
	optName := optionID
	for _, opt := range m.schema.StatusField.Options {
		if opt.ID == optionID {
			optName = opt.Name
			break
		}
	}

	// Optimistically update the item in place.
	itemID := m.pickerItemID
	for i, item := range m.items {
		if item.ID == itemID {
			if m.items[i].Fields == nil {
				m.items[i].Fields = make(data.FieldValues)
			}
			m.items[i].Fields[m.schema.StatusField.ID] = data.FieldValueSingleSelect{
				OptionID: optionID,
				Name:     optName,
			}
			break
		}
	}
	m.table.SetRows(BuildRows(m.ctx, m.filteredItems(), m.schema))

	// Capture values for the goroutine closure.
	projectID := m.projectID
	statusFieldID := m.schema.StatusField.ID
	sectionId := m.sectionId
	taskID := fmt.Sprintf("update_status_%s_%d", itemID, time.Now().UnixNano())
	var cache *persistcache.Store
	if m.ctx != nil {
		cache = m.ctx.ProjectsCache
	}

	optIDCopy := optionID
	// Copy for the closure: m.previousStatus is overwritten as soon as the
	// picker opens for another item, and the closure runs on a goroutine —
	// reading the field there is both a data race and a wrong-rollback bug.
	prevStatus := m.previousStatus

	// Register the task with the footer spinner.
	var startCmd tea.Cmd
	if m.ctx != nil && m.ctx.StartTask != nil {
		task := context.Task{
			Id:           taskID,
			StartText:    "Updating status…",
			FinishedText: "Status updated",
			State:        context.TaskStart,
		}
		startCmd = m.ctx.StartTask(task)
	}

	// Async mutation command.
	mutateCmd := func() tea.Msg {
		updatedItem, err := data.UpdateItemStatus(cache, projectID, itemID, statusFieldID, &optIDCopy)
		if err != nil {
			log.Warn("UpdateItemStatus: mutation failed",
				"item_id", itemID,
				"field_id", statusFieldID,
				"error_class", fmt.Sprintf("%T", err),
				"err", err,
			)
			return constants.TaskFinishedMsg{
				TaskId:      taskID,
				SectionId:   sectionId,
				SectionType: sectionTypeProject,
				Err:         err,
				Msg: StatusUpdateErrMsg{
					ItemID:         itemID,
					PreviousStatus: prevStatus,
					Err:            err,
				},
			}
		}
		return constants.TaskFinishedMsg{
			TaskId:      taskID,
			SectionId:   sectionId,
			SectionType: sectionTypeProject,
			Msg: StatusUpdatedMsg{
				ItemID:           itemID,
				IntendedOptionID: optIDCopy,
				UpdatedItem:      updatedItem,
			},
		}
	}

	return tea.Batch(startCmd, mutateCmd)
}

// onStatusCancelled is the statuspicker.onCancel callback. No mutation is fired.
func (m *Model) onStatusCancelled() tea.Cmd {
	return nil
}

// handleStatusUpdated reconciles the item from the server response.
// If the server's value differs from the intended value (concurrent edit),
// a hint is surfaced in the footer.
func (m *Model) handleStatusUpdated(msg StatusUpdatedMsg) tea.Cmd {
	if m.schema == nil || m.schema.StatusField == nil {
		return nil
	}
	statusFieldID := m.schema.StatusField.ID

	// Determine the server-authoritative status value.
	serverStatus := msg.UpdatedItem.Fields[statusFieldID]

	// Reconcile: apply the server value (not the intended value) to the item.
	for i, item := range m.items {
		if item.ID == msg.ItemID {
			if m.items[i].Fields == nil {
				m.items[i].Fields = make(data.FieldValues)
			}
			if serverStatus != nil {
				m.items[i].Fields[statusFieldID] = serverStatus
			} else {
				delete(m.items[i].Fields, statusFieldID)
			}
			break
		}
	}
	m.table.SetRows(BuildRows(m.ctx, m.filteredItems(), m.schema))

	// Check whether the server value differs from what we intended (concurrent edit).
	serverSSV, serverIsSSV := serverStatus.(data.FieldValueSingleSelect)
	if serverIsSSV && serverSSV.OptionID != msg.IntendedOptionID {
		if m.ctx != nil {
			m.ctx.Error = fmt.Errorf("status was changed concurrently - refreshed")
		}
	}

	return nil
}

// handleStatusUpdateErr reverts the optimistic update and surfaces a 2s error flash.
func (m *Model) handleStatusUpdateErr(msg StatusUpdateErrMsg) tea.Cmd {
	if m.schema == nil || m.schema.StatusField == nil {
		return nil
	}
	statusFieldID := m.schema.StatusField.ID

	// Revert to the saved previous status.
	for i, item := range m.items {
		if item.ID == msg.ItemID {
			if m.items[i].Fields == nil {
				m.items[i].Fields = make(data.FieldValues)
			}
			if msg.PreviousStatus != nil {
				m.items[i].Fields[statusFieldID] = msg.PreviousStatus
			} else {
				delete(m.items[i].Fields, statusFieldID)
			}
			break
		}
	}
	m.table.SetRows(BuildRows(m.ctx, m.filteredItems(), m.schema))

	// Surface error in the footer for 2 seconds.
	if m.ctx != nil {
		m.ctx.Error = fmt.Errorf("failed to update status: %w", msg.Err)
	}

	return tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
		return clearStatusErrorMsg{}
	})
}

// clearStatusErrorMsg clears the status-update error from the footer after the
// 2-second flash window.
type clearStatusErrorMsg struct{}
