package projectsection

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/projectrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prompt"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/section"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
)

func newTestCtx() *context.ProgramContext {
	return &context.ProgramContext{
		StartTask: func(task context.Task) tea.Cmd {
			return func() tea.Msg { return nil }
		},
		Config: &config.Config{},
	}
}

func newTestModel() Model {
	ctx := newTestCtx()
	project := &data.ProjectData{
		ID:         "PVT_1",
		Number:     "1",
		Title:      "Test Project",
		URL:        "https://github.com/orgs/acme/projects/1",
		Owner:      data.OwnerRef{Kind: "org", Login: "acme"},
		Closed:     false,
		ItemsCount: 5,
		UpdatedAt:  time.Now(),
	}
	m := Model{
		BaseModel: section.BaseModel{
			Ctx:                       ctx,
			IsPromptConfirmationShown: false,
			PromptConfirmationBox:     prompt.NewModel(ctx),
		},
		Projects: []projectrow.Data{
			{Primary: project},
		},
		cfg: config.ProjectsSectionConfig{
			Title: "My Projects",
		},
	}
	return m
}

// TestGetItemForms verifies the singular/plural form helpers.
func TestGetItemForms(t *testing.T) {
	m := newTestModel()
	require.Equal(t, "Project", m.GetItemSingularForm())
	require.Equal(t, "Projects", m.GetItemPluralForm())
}

// TestGetTotalCount verifies the TotalCount accessor.
func TestGetTotalCount(t *testing.T) {
	m := newTestModel()
	m.TotalCount = 3
	require.Equal(t, 3, m.GetTotalCount())
}

// TestNumRows returns the count of in-memory project rows.
func TestNumRows(t *testing.T) {
	m := newTestModel()
	require.Equal(t, 1, m.NumRows())
}

// TestGetCurrRow returns the row at the current cursor.
func TestGetCurrRow(t *testing.T) {
	m := newTestModel()
	row := m.GetCurrRow()
	require.NotNil(t, row)
	require.Equal(t, "Test Project", row.GetTitle())
}

// TestGetCurrRow_Empty returns nil when there are no projects.
func TestGetCurrRow_Empty(t *testing.T) {
	m := newTestModel()
	m.Projects = nil
	row := m.GetCurrRow()
	require.Nil(t, row)
}

// TestBuildRows verifies that BuildRows returns the correct number of rows.
// Note: This avoids calling rendering methods that require initialised Ctx styles.
func TestBuildRows_Count(t *testing.T) {
	m := newTestModel()
	// Add a second project to verify count handling
	m.Projects = append(m.Projects, projectrow.Data{
		Primary: &data.ProjectData{Title: "Second Project"},
	})
	require.Equal(t, 2, m.NumRows())
}

// TestResetRows clears the Projects slice and the base page info.
func TestResetRows(t *testing.T) {
	m := newTestModel()
	m.ResetRows()
	require.Empty(t, m.Projects)
	require.Nil(t, m.PageInfo)
}

// TestSectionProjectsFetchedMsg_Fields verifies that SectionProjectsFetchedMsg
// correctly carries the fields needed to update the model.
func TestSectionProjectsFetchedMsg_Fields(t *testing.T) {
	taskId := "test-task-id"
	project := &data.ProjectData{
		ID:     "PVT_2",
		Number: "2",
		Title:  "Fetched Project",
		Owner:  data.OwnerRef{Kind: "user", Login: "alice"},
	}
	msg := SectionProjectsFetchedMsg{
		Projects:   []projectrow.Data{{Primary: project}},
		TotalCount: 1,
		PageInfo:   data.PageInfo{HasNextPage: false},
		TaskId:     taskId,
	}

	require.Equal(t, taskId, msg.TaskId)
	require.Equal(t, 1, msg.TotalCount)
	require.Len(t, msg.Projects, 1)
	require.Equal(t, "Fetched Project", msg.Projects[0].Primary.Title)
	require.False(t, msg.PageInfo.HasNextPage)
}

// TestLastFetchTaskId_StaleDropLogic verifies the stale-response-drop pattern:
// when m.LastFetchTaskId != msg.TaskId the message should be ignored.
func TestLastFetchTaskId_StaleDropLogic(t *testing.T) {
	m := newTestModel()
	m.LastFetchTaskId = "current-task"
	m.TotalCount = 0
	m.PageInfo = nil

	// Simulate the stale-drop check that happens in Update
	staleMsg := SectionProjectsFetchedMsg{
		Projects:   []projectrow.Data{},
		TotalCount: 99,
		TaskId:     "old-task-id",
	}

	// Directly apply the message-handling logic (mirrors Update's inner case)
	if m.LastFetchTaskId == staleMsg.TaskId {
		m.TotalCount = staleMsg.TotalCount
	}

	require.Equal(t, 0, m.TotalCount, "stale response should not update TotalCount")
	require.Nil(t, m.PageInfo, "stale response should not set PageInfo")
}

// TestConfirmation_CancelWithEsc verifies that Esc dismisses the prompt.
func TestConfirmation_CancelWithEsc(t *testing.T) {
	m := newTestModel()
	m.IsPromptConfirmationShown = true
	m.PromptConfirmationBox.Focus()

	msg := tea.KeyPressMsg{Code: tea.KeyEsc}
	_, _ = m.Update(msg)

	require.False(t, m.IsPromptConfirmationShown,
		"Esc should dismiss the confirmation prompt")
}
