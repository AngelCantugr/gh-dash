package projectsection

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/projectrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/section"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/table"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/utils"
)

const SectionType = "project"

// Model is the list section for GitHub ProjectsV2. It embeds section.BaseModel
// for free search, pagination, prompt-confirmation, and table behaviour, and
// adds a typed Projects slice for rendering.
type Model struct {
	section.BaseModel
	Projects      []projectrow.Data
	cfg           config.ProjectsSectionConfig
	initialCursor int  // cursor restored from StateStore on first fetch
	cursorApplied bool // true after initialCursor has been applied to the table
}

// sectionKey returns the state-store key used to persist this section's cursor.
func (m *Model) sectionKey() string {
	return fmt.Sprintf("projects/%d", m.Id)
}

// NewModel constructs a single projects section.
func NewModel(
	id int,
	ctx *context.ProgramContext,
	cfg config.ProjectsSectionConfig,
	lastUpdated time.Time,
	createdAt time.Time,
) Model {
	m := Model{cfg: cfg}
	m.BaseModel = section.NewModel(
		ctx,
		section.NewSectionOptions{
			Id:          id,
			Config:      cfg.ToSectionConfig(),
			Type:        SectionType,
			Columns:     GetSectionColumns(cfg, ctx),
			Singular:    m.GetItemSingularForm(),
			Plural:      m.GetItemPluralForm(),
			LastUpdated: lastUpdated,
			CreatedAt:   createdAt,
		},
	)
	m.Projects = []projectrow.Data{}
	// Projects use structured filters, not a free-text search string
	m.IsSearchSupported = false

	// Restore cursor from session state so the user returns to their last position.
	if ctx.StateStore != nil {
		snap := ctx.StateStore.Snapshot()
		key := fmt.Sprintf("projects/%d", id)
		if ss, ok := snap.PerSection[key]; ok {
			m.initialCursor = ss.Cursor
		}
	}

	return m
}

// ── section.Section interface ────────────────────────────────────────────────

func (m *Model) Update(msg tea.Msg) (section.Section, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.IsSearchFocused() {
			switch msg.String() {
			case "ctrl+c", "esc":
				m.SearchBar.SetValue(m.SearchValue)
				blinkCmd := m.SetIsSearching(false)
				return m, blinkCmd
			case "enter":
				m.SearchValue = m.SearchBar.Value()
				m.SyncSmartFilterWithSearchValue()
				m.SetIsSearching(false)
				m.ResetRows()
				return m, tea.Batch(m.FetchNextPageSectionRows()...)
			}
			break
		}

		if m.IsPromptConfirmationFocused() {
			switch msg.String() {
			case "ctrl+c", "esc":
				m.PromptConfirmationBox.Reset()
				cmd = m.SetIsPromptConfirmationShown(false)
				return m, cmd
			case "enter":
				m.PromptConfirmationBox.Reset()
				blinkCmd := m.SetIsPromptConfirmationShown(false)
				return m, blinkCmd
			}
			break
		}

	case SectionProjectsFetchedMsg:
		if m.LastFetchTaskId == msg.TaskId {
			if m.PageInfo != nil {
				m.Projects = append(m.Projects, msg.Projects...)
			} else {
				m.Projects = msg.Projects
			}
			m.TotalCount = msg.TotalCount
			m.PageInfo = &msg.PageInfo
			m.SetIsLoading(false)
			m.Table.SetRows(m.BuildRows())
			m.Table.UpdateLastUpdated(time.Now())
			m.UpdateTotalItemsCount(m.TotalCount)
			// Restore cursor to the last persisted position on the first fetch.
			if !m.cursorApplied && m.initialCursor > 0 {
				m.Table.SetCurrItem(m.initialCursor)
				m.cursorApplied = true
			}
		}
	}

	search, searchCmd := m.SearchBar.Update(msg)
	m.Table.SetRows(m.BuildRows())
	m.SearchBar = search

	prompt, promptCmd := m.PromptConfirmationBox.Update(msg)
	m.PromptConfirmationBox = prompt

	prevCursor := m.Table.GetCurrItem()
	tbl, tableCmd := m.Table.Update(msg)
	m.Table = tbl
	newCursor := m.Table.GetCurrItem()

	// Persist cursor changes to disk (debounced 500ms by state.Store).
	if m.Ctx.StateStore != nil && prevCursor != newCursor {
		m.Ctx.StateStore.SetCursor(m.sectionKey(), newCursor)
	}

	return m, tea.Batch(cmd, searchCmd, promptCmd, tableCmd)
}

func (m Model) BuildRows() []table.Row {
	var rows []table.Row
	currItem := m.Table.GetCurrItem()
	for i, p := range m.Projects {
		pCopy := p
		proj := projectrow.Project{
			Ctx:     m.Ctx,
			Data:    &pCopy,
			Columns: m.Table.Columns,
		}
		rows = append(rows, proj.ToTableRow(currItem == i))
	}
	if rows == nil {
		rows = []table.Row{}
	}
	return rows
}

func (m *Model) NumRows() int {
	return len(m.Projects)
}

// SectionProjectsFetchedMsg is emitted by the fetch command when the network
// response arrives. It is wrapped in constants.TaskFinishedMsg.
type SectionProjectsFetchedMsg struct {
	Projects   []projectrow.Data
	TotalCount int
	PageInfo   data.PageInfo
	TaskId     string
}

func (m *Model) GetCurrRow() data.RowData {
	idx := m.Table.GetCurrItem()
	if idx < 0 || idx >= len(m.Projects) {
		return nil
	}
	p := m.Projects[idx]
	return &p
}

// FetchNextPageSectionRows kicks off an async fetch for this section's projects.
// It follows the LastFetchTaskId pattern from prssection to drop stale responses.
// Projects are fetched in a single page (no cursor pagination).
func (m *Model) FetchNextPageSectionRows() []tea.Cmd {
	if m == nil {
		return nil
	}

	// Projects are fetched in one shot — after the first fetch PageInfo is set
	// with HasNextPage=false so we never re-fetch unless explicitly refreshed.
	if m.PageInfo != nil && !m.PageInfo.HasNextPage {
		return nil
	}

	var cmds []tea.Cmd

	taskId := fmt.Sprintf("fetching_projects_%d_%s", m.Id, time.Now().String())
	isFirstFetch := m.LastFetchTaskId == ""
	m.LastFetchTaskId = taskId

	task := context.Task{
		Id:           taskId,
		StartText:    fmt.Sprintf(`Fetching projects for "%s"`, m.Config.Title),
		FinishedText: fmt.Sprintf(`Projects for "%s" have been fetched`, m.Config.Title),
		State:        context.TaskStart,
		Error:        nil,
	}
	startCmd := m.Ctx.StartTask(task)
	cmds = append(cmds, startCmd)

	// Capture values needed inside the goroutine closure.
	owners := m.cfg.Owners
	filters := m.cfg.Filters
	sectionId := m.Id
	sectionType := m.Type
	limit := m.Config.Limit
	defaultLimit := m.Ctx.Config.Defaults.PrsLimit

	fetchCmd := func() tea.Msg {
		fetchLimit := defaultLimit
		if limit != nil {
			fetchLimit = *limit
		}

		// Convert config.ProjectFilters → data.ProjectFilters
		// (config uses bool, data uses *bool)
		dataOwners := make([]data.OwnerRef, len(owners))
		for i, o := range owners {
			dataOwners[i] = data.OwnerRef{
				Kind:  o.Kind,
				Login: o.Login,
			}
		}
		closedPtr := &filters.Closed
		dataFilters := data.ProjectFilters{
			Closed:        closedPtr,
			TitleContains: filters.TitleContains,
		}

		projects, err := data.FetchProjects(m.Ctx.ProjectsCache, dataOwners, dataFilters)
		if err != nil {
			return constants.TaskFinishedMsg{
				SectionId:   sectionId,
				SectionType: sectionType,
				TaskId:      taskId,
				Err:         err,
			}
		}

		// Apply client-side limit
		if fetchLimit > 0 && len(projects) > fetchLimit {
			projects = projects[:fetchLimit]
		}

		rows := make([]projectrow.Data, 0, len(projects))
		for _, p := range projects {
			pCopy := p
			rows = append(rows, projectrow.Data{Primary: &pCopy})
		}

		return constants.TaskFinishedMsg{
			SectionId:   sectionId,
			SectionType: sectionType,
			TaskId:      taskId,
			Msg: SectionProjectsFetchedMsg{
				Projects:   rows,
				TotalCount: len(rows),
				// Projects are fetched all at once — no next page.
				PageInfo: data.PageInfo{HasNextPage: false},
				TaskId:   taskId,
			},
		}
	}
	cmds = append(cmds, fetchCmd)

	m.IsLoading = true
	if isFirstFetch {
		m.SetIsLoading(true)
		cmds = append(cmds, m.Table.StartLoadingSpinner())
	}

	return cmds
}

func (m *Model) ResetRows() {
	m.Projects = nil
	m.BaseModel.ResetRows()
}

func (m Model) GetItemSingularForm() string {
	return "Project"
}

func (m Model) GetItemPluralForm() string {
	return "Projects"
}

func (m Model) GetTotalCount() int {
	return m.TotalCount
}

func (m *Model) SetIsLoading(val bool) {
	m.IsLoading = val
	m.Table.SetIsLoading(val)
}

func (m Model) GetPagerContent() string {
	pagerContent := ""
	timeElapsed := utils.TimeElapsed(m.LastUpdated())
	if timeElapsed == "now" {
		timeElapsed = "just now"
	} else {
		timeElapsed = fmt.Sprintf("~%v ago", timeElapsed)
	}
	if m.TotalCount > 0 {
		pagerContent = fmt.Sprintf(
			"%v Updated %v • %v %v/%v (fetched %v)",
			constants.WaitingIcon,
			timeElapsed,
			m.SingularForm,
			m.Table.GetCurrItem()+1,
			m.TotalCount,
			len(m.Table.Rows),
		)
	}
	return m.Ctx.Styles.ListViewPort.PagerStyle.Render(pagerContent)
}

// FetchAllSections creates/refreshes all project sections from config and
// kicks off an async fetch for each. Existing section state is preserved
// across refreshes.
func FetchAllSections(
	ctx *context.ProgramContext,
	oldSections []section.Section,
) (sections []section.Section, fetchAllCmd tea.Cmd) {
	cfgs := ctx.Config.ProjectsSections
	fetchCmds := make([]tea.Cmd, 0, len(cfgs))
	sections = make([]section.Section, 0, len(cfgs))

	for i, sectionConfig := range cfgs {
		sectionModel := NewModel(
			i, // projects view has no search-section prefix; first real section is index 0
			ctx,
			sectionConfig,
			time.Now(),
			time.Now(),
		)
		// Preserve existing data across refresh so the user sees stale rows
		// while the new fetch is in flight.
		if i < len(oldSections) && oldSections[i] != nil {
			if old, ok := oldSections[i].(*Model); ok {
				sectionModel.Projects = old.Projects
				sectionModel.LastFetchTaskId = old.LastFetchTaskId
			}
		}
		sections = append(sections, &sectionModel)
		fetchCmds = append(fetchCmds, sectionModel.FetchNextPageSectionRows()...)
	}

	return sections, tea.Batch(fetchCmds...)
}
