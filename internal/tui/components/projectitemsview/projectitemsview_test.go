package projectitemsview

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/data"
)

// ---------------------------------------------------------------------------
// renderItemType
// ---------------------------------------------------------------------------

func TestRenderItemType_Issue(t *testing.T) {
	require.Equal(t, "issue", renderItemType(data.ItemTypeIssue))
}

func TestRenderItemType_PR(t *testing.T) {
	require.Equal(t, "pr", renderItemType(data.ItemTypePullRequest))
}

func TestRenderItemType_Draft(t *testing.T) {
	require.Equal(t, "draft", renderItemType(data.ItemTypeDraftIssue))
}

func TestRenderItemType_Redacted(t *testing.T) {
	require.Equal(t, "(redacted)", renderItemType(data.ItemTypeRedacted))
}

func TestRenderItemType_Unknown(t *testing.T) {
	// An out-of-range ItemType value should fall through to the default case.
	require.Equal(t, "—", renderItemType(data.ItemType(99)))
}

// ---------------------------------------------------------------------------
// renderStatus
// ---------------------------------------------------------------------------

func TestRenderStatus_NilSchema(t *testing.T) {
	item := data.ProjectItemData{ID: "1", Type: data.ItemTypeIssue}
	result := renderStatus(item, nil)
	require.Equal(t, "—", result)
}

func TestRenderStatus_SchemaNoStatusField(t *testing.T) {
	schema := &data.ProjectSchema{StatusField: nil}
	item := data.ProjectItemData{ID: "1", Type: data.ItemTypeIssue}
	result := renderStatus(item, schema)
	require.Equal(t, "—", result)
}

func TestRenderStatus_ItemMissingField(t *testing.T) {
	schema := &data.ProjectSchema{
		StatusField: &data.StatusFieldDef{ID: "field_status"},
	}
	item := data.ProjectItemData{
		ID:     "1",
		Type:   data.ItemTypeIssue,
		Fields: data.FieldValues{},
	}
	result := renderStatus(item, schema)
	require.Equal(t, "—", result)
}

func TestRenderStatus_ValidSingleSelect(t *testing.T) {
	schema := &data.ProjectSchema{
		StatusField: &data.StatusFieldDef{ID: "field_status"},
	}
	item := data.ProjectItemData{
		ID:   "1",
		Type: data.ItemTypeIssue,
		Fields: data.FieldValues{
			"field_status": data.FieldValueSingleSelect{Name: "In Progress"},
		},
	}
	result := renderStatus(item, schema)
	require.Equal(t, "In Progress", result)
}

func TestRenderStatus_WrongFieldType(t *testing.T) {
	schema := &data.ProjectSchema{
		StatusField: &data.StatusFieldDef{ID: "field_status"},
	}
	// Store a non-SingleSelect value for the status field.
	item := data.ProjectItemData{
		ID:   "1",
		Type: data.ItemTypeIssue,
		Fields: data.FieldValues{
			"field_status": data.FieldValueText{Text: "some text"},
		},
	}
	result := renderStatus(item, schema)
	require.Equal(t, "—", result)
}

// ---------------------------------------------------------------------------
// GetCurrItem
// ---------------------------------------------------------------------------

func TestGetCurrItem_EmptyItems(t *testing.T) {
	m := &Model{items: nil}
	result := m.GetCurrItem()
	require.Nil(t, result)
}

func TestGetCurrItem_ReturnsFirstItem(t *testing.T) {
	m := &Model{
		items: []data.ProjectItemData{
			{ID: "1", Title: "Only item"},
		},
	}
	// table cursor starts at 0 which is in-bounds, so it should return the item.
	item := m.GetCurrItem()
	require.NotNil(t, item)
	require.Equal(t, "1", item.ID)
}

// ---------------------------------------------------------------------------
// ProjectItemsFetchedMsg round-trip
// ---------------------------------------------------------------------------

func TestProjectItemsFetchedMsg_Fields(t *testing.T) {
	now := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	items := []data.ProjectItemData{
		{ID: "i1", Title: "Fix bug", Type: data.ItemTypeIssue, UpdatedAt: now},
		{ID: "p1", Title: "Add feature", Type: data.ItemTypePullRequest, UpdatedAt: now},
	}
	schema := data.ProjectSchema{
		StatusField: &data.StatusFieldDef{ID: "sf1"},
	}
	msg := ProjectItemsFetchedMsg{
		Schema:   schema,
		Items:    items,
		PageInfo: data.PageInfo{HasNextPage: true, EndCursor: "cursor123"},
		Err:      nil,
		Append:   false,
	}

	require.Len(t, msg.Items, 2)
	require.Equal(t, "Fix bug", msg.Items[0].Title)
	require.True(t, msg.PageInfo.HasNextPage)
	require.Equal(t, "cursor123", msg.PageInfo.EndCursor)
	require.False(t, msg.Append)
}

// ---------------------------------------------------------------------------
// GetColumns
// ---------------------------------------------------------------------------

func TestGetColumns_Count(t *testing.T) {
	cols := GetColumns()
	require.Len(t, cols, 5, "expected 5 columns: Title, Type, Repo, Status, Updated")
	require.Equal(t, "Title", cols[0].Title)
	require.Equal(t, "Type", cols[1].Title)
	require.Equal(t, "Repo", cols[2].Title)
	require.Equal(t, "Status", cols[3].Title)
	require.Equal(t, "Updated", cols[4].Title)
	require.NotNil(t, cols[0].Grow, "Title column should have Grow set")
}

// ---------------------------------------------------------------------------
// BuildColumns
// ---------------------------------------------------------------------------

func TestBuildColumns_NilSchema_ReturnsBaseOnly(t *testing.T) {
	cols := BuildColumns(nil, nil)
	require.Len(t, cols, 5)
}

func TestBuildColumns_EmptyExtraFieldOrder_ReturnsBaseOnly(t *testing.T) {
	schema := &data.ProjectSchema{
		ExtraFields:     map[string]data.FieldDef{},
		ExtraFieldOrder: []string{},
	}
	cols := BuildColumns(schema, nil)
	require.Len(t, cols, 5)
}

func TestBuildColumns_ExtraColumnsAppendedInYAMLOrder(t *testing.T) {
	schema := &data.ProjectSchema{
		ExtraFields: map[string]data.FieldDef{
			"FIELD_PRIO": {ID: "FIELD_PRIO", Name: "Priority"},
			"FIELD_ITER": {ID: "FIELD_ITER", Name: "Iteration"},
		},
		ExtraFieldOrder: []string{"FIELD_PRIO", "FIELD_ITER"},
	}
	cols := BuildColumns(schema, nil)
	require.Len(t, cols, 7)
	require.Equal(t, "Priority", cols[5].Title)
	require.Equal(t, "Iteration", cols[6].Title)
	require.NotNil(t, cols[5].Width)
	require.NotNil(t, cols[6].Width)
}

func TestBuildColumns_WidthAtLeastHeaderLength(t *testing.T) {
	schema := &data.ProjectSchema{
		ExtraFields: map[string]data.FieldDef{
			"FIELD_X": {ID: "FIELD_X", Name: "LongHeaderName"},
		},
		ExtraFieldOrder: []string{"FIELD_X"},
	}
	cols := BuildColumns(schema, nil)
	require.Len(t, cols, 6)
	require.GreaterOrEqual(t, *cols[5].Width, len("LongHeaderName"))
}

func TestBuildColumns_SchemaDriftFieldIDSkipped(t *testing.T) {
	// ExtraFieldOrder references a field ID not in ExtraFields map.
	schema := &data.ProjectSchema{
		ExtraFields:     map[string]data.FieldDef{},
		ExtraFieldOrder: []string{"FIELD_GONE"},
	}
	cols := BuildColumns(schema, nil)
	// drift field is skipped — only base columns returned
	require.Len(t, cols, 5)
}

func TestBuildColumns_WidthGrowsWithObservedValues(t *testing.T) {
	schema := &data.ProjectSchema{
		ExtraFields: map[string]data.FieldDef{
			"FIELD_T": {ID: "FIELD_T", Name: "Tag"},
		},
		ExtraFieldOrder: []string{"FIELD_T"},
	}
	observed := []data.ProjectItemData{
		{
			Fields: data.FieldValues{
				"FIELD_T": data.FieldValueText{Text: "a very long text value indeed"},
			},
		},
	}
	cols := BuildColumns(schema, observed)
	require.Len(t, cols, 6)
	require.GreaterOrEqual(t, *cols[5].Width, len("a very long text value indeed"))
}

// ---------------------------------------------------------------------------
// renderExtraField
// ---------------------------------------------------------------------------

func TestRenderExtraField_MissingFromItem_ReturnsDash(t *testing.T) {
	item := data.ProjectItemData{Fields: data.FieldValues{}}
	result := renderExtraField("FIELD_X", item, nil)
	require.Equal(t, "—", result)
}

func TestRenderExtraField_SchemaDriftGuard_ReturnsDash(t *testing.T) {
	schema := &data.ProjectSchema{
		ExtraFields: map[string]data.FieldDef{},
	}
	item := data.ProjectItemData{
		Fields: data.FieldValues{
			"FIELD_X": data.FieldValueText{Text: "should not render"},
		},
	}
	result := renderExtraField("FIELD_X", item, schema)
	require.Equal(t, "—", result)
}

func TestRenderExtraField_SingleSelect(t *testing.T) {
	item := data.ProjectItemData{
		Fields: data.FieldValues{
			"FIELD_SS": data.FieldValueSingleSelect{Name: "High"},
		},
	}
	result := renderExtraField("FIELD_SS", item, nil)
	require.Equal(t, "High", result)
}

func TestRenderExtraField_NumberInteger(t *testing.T) {
	item := data.ProjectItemData{
		Fields: data.FieldValues{
			"FIELD_N": data.FieldValueNumber{Number: 42},
		},
	}
	result := renderExtraField("FIELD_N", item, nil)
	require.Equal(t, "42", result)
}

func TestRenderExtraField_NumberFloat(t *testing.T) {
	item := data.ProjectItemData{
		Fields: data.FieldValues{
			"FIELD_N": data.FieldValueNumber{Number: 3.14},
		},
	}
	result := renderExtraField("FIELD_N", item, nil)
	require.Equal(t, "3.14", result)
}

func TestRenderExtraField_Date(t *testing.T) {
	item := data.ProjectItemData{
		Fields: data.FieldValues{
			"FIELD_D": data.FieldValueDate{Date: "2024-04-22"},
		},
	}
	result := renderExtraField("FIELD_D", item, nil)
	require.Equal(t, "2024-04-22", result)
}

func TestRenderExtraField_DateWithTimestamp(t *testing.T) {
	item := data.ProjectItemData{
		Fields: data.FieldValues{
			"FIELD_D": data.FieldValueDate{Date: "2024-04-22T00:00:00Z"},
		},
	}
	result := renderExtraField("FIELD_D", item, nil)
	require.Equal(t, "2024-04-22", result)
}

func TestRenderExtraField_Iteration(t *testing.T) {
	item := data.ProjectItemData{
		Fields: data.FieldValues{
			"FIELD_I": data.FieldValueIteration{
				Title:     "Sprint 4",
				StartDate: "2024-04-22",
				Duration:  14,
			},
		},
	}
	result := renderExtraField("FIELD_I", item, nil)
	require.Equal(t, "Sprint 4 (Apr 22 → May 5)", result)
}

func TestRenderExtraField_IterationBadStartDate_FallsBackToTitle(t *testing.T) {
	item := data.ProjectItemData{
		Fields: data.FieldValues{
			"FIELD_I": data.FieldValueIteration{
				Title:     "Q2",
				StartDate: "not-a-date",
				Duration:  90,
			},
		},
	}
	result := renderExtraField("FIELD_I", item, nil)
	require.Equal(t, "Q2", result)
}

func TestRenderExtraField_Text(t *testing.T) {
	item := data.ProjectItemData{
		Fields: data.FieldValues{
			"FIELD_TXT": data.FieldValueText{Text: "hello world"},
		},
	}
	result := renderExtraField("FIELD_TXT", item, nil)
	require.Equal(t, "hello world", result)
}

func TestRenderExtraField_Unknown_ReturnsDash(t *testing.T) {
	item := data.ProjectItemData{
		Fields: data.FieldValues{
			"FIELD_U": data.FieldValueUnknown{},
		},
	}
	result := renderExtraField("FIELD_U", item, nil)
	require.Equal(t, "—", result)
}

// ---------------------------------------------------------------------------
// filteredItems (AC #9 — client-side title search)
// ---------------------------------------------------------------------------

func TestFilteredItems_EmptyQuery_ReturnsAll(t *testing.T) {
	m := &Model{
		items: []data.ProjectItemData{
			{ID: "1", Title: "Fix bug"},
			{ID: "2", Title: "Add feature"},
		},
		searchQuery: "",
	}
	got := m.filteredItems()
	require.Len(t, got, 2)
}

func TestFilteredItems_MatchesByTitle(t *testing.T) {
	m := &Model{
		items: []data.ProjectItemData{
			{ID: "1", Title: "Fix auth bug"},
			{ID: "2", Title: "Add dashboard"},
			{ID: "3", Title: "Auth token refresh"},
		},
		searchQuery: "auth",
	}
	got := m.filteredItems()
	require.Len(t, got, 2)
	require.Equal(t, "1", got[0].ID)
	require.Equal(t, "3", got[1].ID)
}

func TestFilteredItems_CaseInsensitive(t *testing.T) {
	m := &Model{
		items: []data.ProjectItemData{
			{ID: "1", Title: "DEPLOY to staging"},
			{ID: "2", Title: "deploy hotfix"},
		},
		searchQuery: "Deploy",
	}
	got := m.filteredItems()
	require.Len(t, got, 2)
}

func TestFilteredItems_NoMatch_ReturnsEmpty(t *testing.T) {
	m := &Model{
		items: []data.ProjectItemData{
			{ID: "1", Title: "Fix bug"},
		},
		searchQuery: "xyz_not_found",
	}
	got := m.filteredItems()
	require.Empty(t, got)
}

// ---------------------------------------------------------------------------
// GetProjectURL (AC #5/#6 — Draft/Redacted open project URL)
// ---------------------------------------------------------------------------

func TestGetProjectURL(t *testing.T) {
	m := &Model{projectURL: "https://github.com/orgs/acme/projects/1"}
	require.Equal(t, "https://github.com/orgs/acme/projects/1", m.GetProjectURL())
}

// ---------------------------------------------------------------------------
// FetchItems no-op hint (AC #10)
// ---------------------------------------------------------------------------

func TestFetchItems_ReturnsNilWhenAlreadyFetching(t *testing.T) {
	m := &Model{isFetching: true}
	cmd := m.FetchItems(false)
	require.Nil(t, cmd, "FetchItems should return nil when a fetch is already in progress")
}
