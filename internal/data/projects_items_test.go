package data

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/persistcache"
)

// --------------------------------------------------------------------------
// FieldValues JSON round-trip
// --------------------------------------------------------------------------

func TestFieldValuesMarshalUnmarshal(t *testing.T) {
	original := FieldValues{
		"FIELD_SS":   FieldValueSingleSelect{OptionID: "OPT_1", Name: "Todo"},
		"FIELD_NUM":  FieldValueNumber{Number: 42.5},
		"FIELD_DATE": FieldValueDate{Date: "2024-01-15"},
		"FIELD_ITER": FieldValueIteration{
			IterationID: "ITER_1",
			Title:       "Sprint 1",
			StartDate:   "2024-01-01",
			Duration:    14,
		},
		"FIELD_TEXT":    FieldValueText{Text: "hello"},
		"FIELD_ISSUE":   FieldValueIssue{Title: "Bug", Number: 42, URL: "https://github.com/o/r/issues/42"},
		"FIELD_UNKNOWN": FieldValueUnknown{},
	}

	raw, err := json.Marshal(original)
	require.NoError(t, err)

	var restored FieldValues
	require.NoError(t, json.Unmarshal(raw, &restored))

	require.Len(t, restored, len(original))

	ss, ok := restored["FIELD_SS"].(FieldValueSingleSelect)
	require.True(t, ok, "FIELD_SS should be FieldValueSingleSelect")
	assert.Equal(t, "OPT_1", ss.OptionID)
	assert.Equal(t, "Todo", ss.Name)

	num, ok := restored["FIELD_NUM"].(FieldValueNumber)
	require.True(t, ok)
	assert.InDelta(t, 42.5, num.Number, 1e-9)

	date, ok := restored["FIELD_DATE"].(FieldValueDate)
	require.True(t, ok)
	assert.Equal(t, "2024-01-15", date.Date)

	iter, ok := restored["FIELD_ITER"].(FieldValueIteration)
	require.True(t, ok)
	assert.Equal(t, "ITER_1", iter.IterationID)
	assert.Equal(t, "Sprint 1", iter.Title)
	assert.Equal(t, 14, iter.Duration)

	txt, ok := restored["FIELD_TEXT"].(FieldValueText)
	require.True(t, ok)
	assert.Equal(t, "hello", txt.Text)

	issue, ok := restored["FIELD_ISSUE"].(FieldValueIssue)
	require.True(t, ok)
	assert.Equal(t, 42, issue.Number)

	_, ok = restored["FIELD_UNKNOWN"].(FieldValueUnknown)
	require.True(t, ok)
}

func TestFieldValuesMarshalUnknownKind(t *testing.T) {
	// An unknown kind in the envelope should unmarshal to FieldValueUnknown.
	raw := json.RawMessage(`{"FIELD_X":{"kind":"nonexistent","value":{}}}`)
	var fv FieldValues
	require.NoError(t, json.Unmarshal(raw, &fv))
	_, ok := fv["FIELD_X"].(FieldValueUnknown)
	require.True(t, ok)
}

// --------------------------------------------------------------------------
// parseItemType
// --------------------------------------------------------------------------

func TestParseItemType(t *testing.T) {
	tests := []struct {
		input string
		want  ItemType
	}{
		{"ISSUE", ItemTypeIssue},
		{"PULL_REQUEST", ItemTypePullRequest},
		{"DRAFT_ISSUE", ItemTypeDraftIssue},
		{"REDACTED", ItemTypeRedacted},
		{"", ItemTypeRedacted},
		{"unknown_type", ItemTypeRedacted},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, parseItemType(tt.input))
		})
	}
}

// --------------------------------------------------------------------------
// parseFieldValue
// --------------------------------------------------------------------------

func TestParseFieldValue(t *testing.T) {
	t.Run("single_select", func(t *testing.T) {
		node := gqlFieldValueNode{
			TypeName: "ProjectV2ItemFieldSingleSelectValue",
			AsSingleSelect: struct {
				Field    gqlFieldRef
				OptionId string
				Name     string
			}{
				Field:    newFieldRef("FIELD_001"),
				OptionId: "OPT_001",
				Name:     "Todo",
			},
		}
		fieldID, fv := parseFieldValue(node)
		assert.Equal(t, "FIELD_001", fieldID)
		ss, ok := fv.(FieldValueSingleSelect)
		require.True(t, ok)
		assert.Equal(t, "OPT_001", ss.OptionID)
		assert.Equal(t, "Todo", ss.Name)
	})

	t.Run("number", func(t *testing.T) {
		n := 3.14
		node := gqlFieldValueNode{
			TypeName: "ProjectV2ItemFieldNumberValue",
			AsNumber: struct {
				Field  gqlFieldRef
				Number *float64
			}{
				Field:  newFieldRef("FIELD_NUM"),
				Number: &n,
			},
		}
		fieldID, fv := parseFieldValue(node)
		assert.Equal(t, "FIELD_NUM", fieldID)
		num, ok := fv.(FieldValueNumber)
		require.True(t, ok)
		assert.InDelta(t, 3.14, num.Number, 1e-9)
	})

	t.Run("number_nil_is_unknown", func(t *testing.T) {
		node := gqlFieldValueNode{
			TypeName: "ProjectV2ItemFieldNumberValue",
			AsNumber: struct {
				Field  gqlFieldRef
				Number *float64
			}{
				Field:  newFieldRef("FIELD_NUM"),
				Number: nil,
			},
		}
		fieldID, fv := parseFieldValue(node)
		assert.Equal(t, "FIELD_NUM", fieldID)
		_, ok := fv.(FieldValueUnknown)
		require.True(t, ok)
	})

	t.Run("date", func(t *testing.T) {
		node := gqlFieldValueNode{
			TypeName: "ProjectV2ItemFieldDateValue",
			AsDate: struct {
				Field gqlFieldRef
				Date  string
			}{
				Field: newFieldRef("FIELD_DATE"),
				Date:  "2024-03-01",
			},
		}
		fieldID, fv := parseFieldValue(node)
		assert.Equal(t, "FIELD_DATE", fieldID)
		d, ok := fv.(FieldValueDate)
		require.True(t, ok)
		assert.Equal(t, "2024-03-01", d.Date)
	})

	t.Run("iteration", func(t *testing.T) {
		node := gqlFieldValueNode{
			TypeName: "ProjectV2ItemFieldIterationValue",
			AsIteration: struct {
				Field       gqlFieldRef
				IterationId string
				Title       string
				StartDate   string
				Duration    int
			}{
				Field:       newFieldRef("FIELD_ITER"),
				IterationId: "ITER_42",
				Title:       "Sprint 42",
				StartDate:   "2024-04-01",
				Duration:    7,
			},
		}
		fieldID, fv := parseFieldValue(node)
		assert.Equal(t, "FIELD_ITER", fieldID)
		it, ok := fv.(FieldValueIteration)
		require.True(t, ok)
		assert.Equal(t, "ITER_42", it.IterationID)
		assert.Equal(t, "Sprint 42", it.Title)
		assert.Equal(t, 7, it.Duration)
	})

	t.Run("text", func(t *testing.T) {
		node := gqlFieldValueNode{
			TypeName: "ProjectV2ItemFieldTextValue",
			AsText: struct {
				Field gqlFieldRef
				Text  string
			}{
				Field: newFieldRef("FIELD_TEXT"),
				Text:  "some text",
			},
		}
		fieldID, fv := parseFieldValue(node)
		assert.Equal(t, "FIELD_TEXT", fieldID)
		txt, ok := fv.(FieldValueText)
		require.True(t, ok)
		assert.Equal(t, "some text", txt.Text)
	})

	t.Run("unknown_typename_no_panic", func(t *testing.T) {
		node := gqlFieldValueNode{TypeName: "SomeFutureType"}
		fieldID, fv := parseFieldValue(node)
		assert.Empty(t, fieldID)
		_, ok := fv.(FieldValueUnknown)
		require.True(t, ok, "unrecognised type must return FieldValueUnknown, not panic")
	})
}

// --------------------------------------------------------------------------
// parseProjectSchema
// --------------------------------------------------------------------------

func makeFieldNode(id, name, kind string, opts []gqlFieldOption) gqlFieldNode {
	switch kind {
	case "single_select":
		return gqlFieldNode{
			TypeName: "ProjectV2SingleSelectField",
			AsSingleSelect: struct {
				Id      string `graphql:"id"`
				Name    string
				Options []gqlFieldOption
			}{Id: id, Name: name, Options: opts},
		}
	case "iteration":
		return gqlFieldNode{
			TypeName: "ProjectV2IterationField",
			AsIteration: struct {
				Id   string `graphql:"id"`
				Name string
			}{Id: id, Name: name},
		}
	default:
		return gqlFieldNode{
			TypeName: "ProjectV2Field",
			AsField: struct {
				Id       string `graphql:"id"`
				Name     string
				DataType string
			}{Id: id, Name: name, DataType: "TEXT"},
		}
	}
}

func TestParseProjectSchema_StatusPresent(t *testing.T) {
	nodes := []gqlFieldNode{
		makeFieldNode("FIELD_TEXT", "Description", "field", nil),
		makeFieldNode("FIELD_STATUS", "Status", "single_select", []gqlFieldOption{
			{Id: "OPT_1", Name: "Todo", Color: "BLUE"},
			{Id: "OPT_2", Name: "Done", Color: "GREEN"},
		}),
		makeFieldNode("FIELD_PRIORITY", "Priority", "field", nil),
	}

	schema := parseProjectSchema(nodes, []string{"Priority"})

	require.NotNil(t, schema.StatusField)
	assert.Equal(t, "FIELD_STATUS", schema.StatusField.ID)
	require.Len(t, schema.StatusField.Options, 2)
	assert.Equal(t, "Todo", schema.StatusField.Options[0].Name)

	require.NotEmpty(t, schema.ExtraFields)
	assert.Equal(t, "FIELD_PRIORITY", schema.ExtraFields["FIELD_PRIORITY"].ID)
	assert.Equal(t, []string{"FIELD_PRIORITY"}, schema.ExtraFieldOrder)
}

func TestParseProjectSchema_StatusAbsent(t *testing.T) {
	nodes := []gqlFieldNode{
		makeFieldNode("FIELD_A", "Alpha", "field", nil),
		makeFieldNode("FIELD_B", "Beta", "field", nil),
	}
	schema := parseProjectSchema(nodes, nil)
	assert.Nil(t, schema.StatusField, "no Status field → StatusField must be nil")
}

func TestParseProjectSchema_DuplicateFieldName(t *testing.T) {
	// Two fields with the same name; first wins.
	nodes := []gqlFieldNode{
		makeFieldNode("FIELD_FIRST", "Priority", "field", nil),
		makeFieldNode("FIELD_SECOND", "Priority", "field", nil),
	}
	schema := parseProjectSchema(nodes, []string{"Priority"})
	require.Len(t, schema.ExtraFields, 1)
	assert.Equal(t, "FIELD_FIRST", schema.ExtraFields["FIELD_FIRST"].ID)
}

func TestParseProjectSchema_ExtraFieldOrder(t *testing.T) {
	nodes := []gqlFieldNode{
		makeFieldNode("FIELD_STATUS", "Status", "single_select", nil),
		makeFieldNode("FIELD_A", "Alpha", "field", nil),
		makeFieldNode("FIELD_B", "Beta", "field", nil),
		makeFieldNode("FIELD_C", "Gamma", "field", nil),
	}
	// Request in a specific order that doesn't match declaration order.
	schema := parseProjectSchema(nodes, []string{"Gamma", "Alpha"})
	assert.Equal(t, []string{"FIELD_C", "FIELD_A"}, schema.ExtraFieldOrder)
}

func TestParseProjectSchema_UnknownExtraFieldSkipped(t *testing.T) {
	nodes := []gqlFieldNode{
		makeFieldNode("FIELD_A", "Alpha", "field", nil),
	}
	schema := parseProjectSchema(nodes, []string{"Nonexistent"})
	assert.Empty(t, schema.ExtraFields)
	assert.Empty(t, schema.ExtraFieldOrder)
}

// --------------------------------------------------------------------------
// parseProjectItem
// --------------------------------------------------------------------------

// buildFieldValueNodes is a test helper that constructs gqlFieldValueNode
// slices using field assignment to avoid anonymous-struct-literal pitfalls.
func buildFieldValueNodes() []gqlFieldValueNode {
	n := 5.0
	var ss gqlFieldValueNode
	ss.TypeName = "ProjectV2ItemFieldSingleSelectValue"
	ss.AsSingleSelect.Field = newFieldRef("F_STATUS")
	ss.AsSingleSelect.OptionId = "OPT_1"
	ss.AsSingleSelect.Name = "Todo"

	var num gqlFieldValueNode
	num.TypeName = "ProjectV2ItemFieldNumberValue"
	num.AsNumber.Field = newFieldRef("F_NUM")
	num.AsNumber.Number = &n

	return []gqlFieldValueNode{ss, num}
}

func TestParseProjectItem_AllTypes(t *testing.T) {
	t.Run("issue", func(t *testing.T) {
		var node gqlItemNode
		node.Id = "PVTI_001"
		node.ItemType = "ISSUE"
		node.Content.AsIssue.Title = "Fix bug"
		node.Content.AsIssue.Number = 7
		node.Content.AsIssue.Url = "https://github.com/owner/repo/issues/7"
		node.Content.AsIssue.Repository.NameWithOwner = "owner/repo"
		node.FieldValues.Nodes = buildFieldValueNodes()
		node.UpdatedAt = time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

		item := parseProjectItem(node)
		assert.Equal(t, "PVTI_001", item.ID)
		assert.Equal(t, ItemTypeIssue, item.Type)
		assert.Equal(t, "Fix bug", item.Title)
		assert.Equal(t, "owner/repo", item.Repo)
		assert.Equal(t, "https://github.com/owner/repo/issues/7", item.URL)
		assert.Len(t, item.Fields, 2)
	})

	t.Run("pull_request", func(t *testing.T) {
		var node gqlItemNode
		node.Id = "PVTI_PR"
		node.ItemType = "PULL_REQUEST"
		node.Content.AsPR.Title = "Add feature"
		node.Content.AsPR.Number = 12
		node.Content.AsPR.Url = "https://github.com/owner/repo/pull/12"
		node.Content.AsPR.Repository.NameWithOwner = "owner/repo"

		item := parseProjectItem(node)
		assert.Equal(t, ItemTypePullRequest, item.Type)
		assert.Equal(t, "Add feature", item.Title)
		assert.Equal(t, "https://github.com/owner/repo/pull/12", item.URL)
	})

	t.Run("draft_issue", func(t *testing.T) {
		var node gqlItemNode
		node.Id = "PVTI_DRAFT"
		node.ItemType = "DRAFT_ISSUE"
		node.Content.AsDraftIssue.Title = "Draft idea"

		item := parseProjectItem(node)
		assert.Equal(t, ItemTypeDraftIssue, item.Type)
		assert.Equal(t, "Draft idea", item.Title)
		assert.Empty(t, item.Repo)
		assert.Empty(t, item.URL)
	})

	t.Run("redacted", func(t *testing.T) {
		var node gqlItemNode
		node.Id = "PVTI_REDACTED"
		node.ItemType = "REDACTED"

		item := parseProjectItem(node)
		assert.Equal(t, ItemTypeRedacted, item.Type)
		assert.Empty(t, item.Title)
		assert.Empty(t, item.Repo)
		assert.Empty(t, item.URL)
	})
}

// --------------------------------------------------------------------------
// Status field fallback logic (schema-level unit test)
// --------------------------------------------------------------------------

func TestStatusFieldFallback_SchemaLevel(t *testing.T) {
	// Build 50 non-Status fields.
	nodes := make([]gqlFieldNode, 50)
	for i := range nodes {
		nodes[i] = makeFieldNode(
			fmt.Sprintf("FIELD_%02d", i),
			fmt.Sprintf("Field%d", i),
			"field", nil,
		)
	}

	schema := parseProjectSchema(nodes, nil)
	assert.Nil(t, schema.StatusField,
		"Status field must be nil when not present in the first 50 fields")

	// Simulate what fetchStatusFieldFallback would return.
	statusDef := &StatusFieldDef{
		ID: "FIELD_STATUS_FALLBACK",
		Options: []StatusOption{
			{ID: "OPT_1", Name: "Todo", Color: "BLUE"},
		},
	}
	schema.StatusField = statusDef

	require.NotNil(t, schema.StatusField)
	assert.Equal(t, "FIELD_STATUS_FALLBACK", schema.StatusField.ID)
	assert.Len(t, schema.StatusField.Options, 1)
}

// --------------------------------------------------------------------------
// FetchProjectItems — mock data (FF_MOCK_DATA=true)
// --------------------------------------------------------------------------

func TestFetchProjectItemsMockData(t *testing.T) {
	t.Setenv(config.FF_MOCK_DATA, "1")

	schema, items, pageInfo, err := FetchProjectItems(nil, "PVT_001", "", 20, []string{"Priority", "Iteration"})

	require.NoError(t, err)
	require.Len(t, items, 4, "fixture has 4 items")

	// Schema
	require.NotNil(t, schema.StatusField)
	assert.Equal(t, "FIELD_STATUS_001", schema.StatusField.ID)
	require.Len(t, schema.StatusField.Options, 3)

	// Extra fields from fixture
	require.NotEmpty(t, schema.ExtraFields)
	require.Equal(t, []string{"FIELD_PRIORITY_001", "FIELD_ITER_001"}, schema.ExtraFieldOrder)

	// PageInfo
	assert.False(t, pageInfo.HasNextPage)

	// Item types
	assert.Equal(t, ItemTypeIssue, items[0].Type)
	assert.Equal(t, "Fix login bug", items[0].Title)
	assert.Equal(t, "owner/my-repo", items[0].Repo)

	assert.Equal(t, ItemTypePullRequest, items[1].Type)
	assert.Equal(t, "Add feature X", items[1].Title)

	assert.Equal(t, ItemTypeDraftIssue, items[2].Type)
	assert.Empty(t, items[2].Repo)
	assert.Empty(t, items[2].URL)

	assert.Equal(t, ItemTypeRedacted, items[3].Type)
	assert.Empty(t, items[3].Title)
	assert.Empty(t, items[3].URL)

	// Field values for item[0]
	fv0 := items[0].Fields
	require.NotNil(t, fv0)
	ss, ok := fv0["FIELD_STATUS_001"].(FieldValueSingleSelect)
	require.True(t, ok)
	assert.Equal(t, "OPT_TODO", ss.OptionID)
	assert.Equal(t, "Todo", ss.Name)
}

func TestFetchProjectItemsMockData_MissingFixture(t *testing.T) {
	t.Setenv(config.FF_MOCK_DATA, "1")

	schema, items, pageInfo, err := FetchProjectItems(nil, "PVT_NONEXISTENT", "", 20, nil)

	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Nil(t, schema.StatusField)
	assert.False(t, pageInfo.HasNextPage)
}

func TestFetchProjectItemsMockData_ManyFields(t *testing.T) {
	t.Setenv(config.FF_MOCK_DATA, "1")

	schema, items, _, err := FetchProjectItems(nil, "PVT_MANY_FIELDS", "", 20, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotNil(t, schema.StatusField,
		"many-fields fixture pre-populates StatusField (represents post-fallback result)")
	assert.Equal(t, "FIELD_STATUS_MANY", schema.StatusField.ID)
}

// --------------------------------------------------------------------------
// FetchProjectItems — cache hit
// --------------------------------------------------------------------------

func TestFetchProjectItemsCacheHit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := persistcache.New()
	require.NoError(t, err)

	// Pre-populate cache.
	cached := projectItemsCache{
		Schema: ProjectSchema{
			StatusField: &StatusFieldDef{ID: "CACHED_STATUS"},
		},
		Items: []ProjectItemData{
			{ID: "CACHE_ITEM_001", Type: ItemTypeIssue, Title: "Cached issue"},
		},
		PageInfo: PageInfo{HasNextPage: false},
	}
	raw, err := json.Marshal(cached)
	require.NoError(t, err)
	require.NoError(t, store.Put("project-items/PROJ_CACHE", raw, time.Hour))

	// client is nil — must not be called; cache should serve the response.
	savedClient := client
	client = nil
	defer func() { client = savedClient }()

	schema, items, pageInfo, err := FetchProjectItems(store, "PROJ_CACHE", "", 20, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "CACHE_ITEM_001", items[0].ID)
	require.NotNil(t, schema.StatusField)
	assert.Equal(t, "CACHED_STATUS", schema.StatusField.ID)
	assert.False(t, pageInfo.HasNextPage)
}

// --------------------------------------------------------------------------
// FetchProjectItems — cache write + generation counter
// --------------------------------------------------------------------------

func TestFetchProjectItemsCacheWrite(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := persistcache.New()
	require.NoError(t, err)

	// Invalidate the prefix before writing so the captured generation is stale.
	require.NoError(t, store.Invalidate("project-items/"))

	// Capture the current generation (post-invalidate, so gen=1).
	gen := store.Generation("project-items/")

	cacheKey := "project-items/PROJ_WRITE_TEST"

	// Write via PutIfFresh with the captured generation — should succeed.
	cached := projectItemsCache{
		Schema: ProjectSchema{StatusField: &StatusFieldDef{ID: "SF_001"}},
		Items:  []ProjectItemData{{ID: "ITEM_001", Type: ItemTypeIssue, Title: "Test"}},
	}
	raw, err := json.Marshal(cached)
	require.NoError(t, err)
	require.NoError(t, store.PutIfFresh(cacheKey, raw, time.Minute, gen))

	// Now bump the generation (simulate a concurrent invalidation).
	require.NoError(t, store.Invalidate("project-items/"))
	newGen := store.Generation("project-items/")
	assert.NotEqual(t, gen, newGen, "generation must change after invalidate")

	// A PutIfFresh with the OLD generation must be rejected.
	err = store.PutIfFresh(cacheKey, raw, time.Minute, gen)
	assert.ErrorIs(t, err, persistcache.ErrStaleGeneration,
		"stale generation must be rejected")
}

// --------------------------------------------------------------------------
// Load-more (stitch) test via mock data
// --------------------------------------------------------------------------

func TestFetchProjectItemsLoadMoreStichesMockData(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := persistcache.New()
	require.NoError(t, err)

	// Pre-populate cache as if a first page was already fetched.
	firstPage := projectItemsCache{
		Schema: ProjectSchema{StatusField: &StatusFieldDef{ID: "SF_LOAD"}},
		Items: []ProjectItemData{
			{ID: "ITEM_PAGE1", Type: ItemTypeIssue, Title: "Page 1 item"},
		},
		PageInfo: PageInfo{HasNextPage: true, EndCursor: "CURSOR_PAGE2"},
	}
	raw, err := json.Marshal(firstPage)
	require.NoError(t, err)
	require.NoError(t, store.Put("project-items/PROJ_LOAD", raw, time.Hour))

	// Read from cache directly (simulates "first page from cache").
	raw2, hit, err := store.Get("project-items/PROJ_LOAD")
	require.NoError(t, err)
	require.True(t, hit)

	var cached projectItemsCache
	require.NoError(t, json.Unmarshal(raw2, &cached))
	assert.Len(t, cached.Items, 1)
	assert.True(t, cached.PageInfo.HasNextPage)
	assert.Equal(t, "CURSOR_PAGE2", cached.PageInfo.EndCursor)

	// Simulate load-more: manually stitch new items.
	newItems := []ProjectItemData{
		{ID: "ITEM_PAGE2", Type: ItemTypePullRequest, Title: "Page 2 item"},
	}
	stitched := projectItemsCache{
		Schema:   cached.Schema,
		Items:    append(cached.Items, newItems...),
		PageInfo: PageInfo{HasNextPage: false},
	}
	stitchedRaw, err := json.Marshal(stitched)
	require.NoError(t, err)
	require.NoError(t, store.Put("project-items/PROJ_LOAD", stitchedRaw, time.Hour))

	// Verify the stitched cache.
	raw3, hit, err := store.Get("project-items/PROJ_LOAD")
	require.NoError(t, err)
	require.True(t, hit)

	var final projectItemsCache
	require.NoError(t, json.Unmarshal(raw3, &final))
	require.Len(t, final.Items, 2)
	assert.Equal(t, "ITEM_PAGE1", final.Items[0].ID)
	assert.Equal(t, "ITEM_PAGE2", final.Items[1].ID)
	assert.False(t, final.PageInfo.HasNextPage)
}

// --------------------------------------------------------------------------
// TreeSortItems
// --------------------------------------------------------------------------

func TestTreeSortItems_MultiRepoSameNumber(t *testing.T) {
	// Repo A #5 is a real parent; repo B independently has its own #5.
	// The child (parent = A#5) must nest under A#5, not B#5.
	items := []ProjectItemData{
		{ID: "b5", Type: ItemTypeIssue, Title: "B five", Repo: "org/repo-b", Number: 5},
		{ID: "a5", Type: ItemTypeIssue, Title: "A five", Repo: "org/repo-a", Number: 5},
		{ID: "child", Type: ItemTypeIssue, Title: "child of A#5", Repo: "org/repo-a", Number: 9, ParentNumber: 5, ParentRepo: "org/repo-a"},
	}

	sorted := TreeSortItems(items)

	require.Len(t, sorted, 3)
	ids := []string{sorted[0].ID, sorted[1].ID, sorted[2].ID}
	require.Equal(t, []string{"b5", "a5", "child"}, ids, "child must immediately follow its real (repo-a) parent")
	assert.Equal(t, 0, sorted[0].Depth)
	assert.Equal(t, 0, sorted[1].Depth)
	assert.Equal(t, 1, sorted[2].Depth)
}

func TestTreeSortItems_ParentRepoFallsBackToOwnRepo(t *testing.T) {
	// ParentRepo unset (stale cache entry): assume the parent lives in the
	// item's own repo.
	items := []ProjectItemData{
		{ID: "parent", Type: ItemTypeIssue, Repo: "org/repo-a", Number: 1},
		{ID: "child", Type: ItemTypeIssue, Repo: "org/repo-a", Number: 2, ParentNumber: 1},
	}

	sorted := TreeSortItems(items)

	require.Len(t, sorted, 2)
	require.Equal(t, "parent", sorted[0].ID)
	require.Equal(t, "child", sorted[1].ID)
	assert.Equal(t, 1, sorted[1].Depth)
}

func TestTreeSortItems_CycleNotDropped(t *testing.T) {
	// A→B→A cycle plus a self-parented item: none can be classified as a
	// root, but every row must still be emitted exactly once.
	items := []ProjectItemData{
		{ID: "a", Type: ItemTypeIssue, Repo: "org/repo", Number: 1, ParentNumber: 2, ParentRepo: "org/repo"},
		{ID: "b", Type: ItemTypeIssue, Repo: "org/repo", Number: 2, ParentNumber: 1, ParentRepo: "org/repo"},
		{ID: "self", Type: ItemTypeIssue, Repo: "org/repo", Number: 3, ParentNumber: 3, ParentRepo: "org/repo"},
		{ID: "root", Type: ItemTypeIssue, Repo: "org/repo", Number: 4},
	}

	sorted := TreeSortItems(items)

	require.Len(t, sorted, len(items), "cyclic items must not be dropped")
	seen := make(map[string]int)
	for _, it := range sorted {
		seen[it.ID]++
	}
	for _, id := range []string{"a", "b", "self", "root"} {
		assert.Equal(t, 1, seen[id], "item %s must appear exactly once", id)
	}
}

func TestTreeSortItems_CycleChildStillNested(t *testing.T) {
	// A child of a cycle member is emitted with its parent's subtree, not
	// orphaned at the end.
	items := []ProjectItemData{
		{ID: "a", Type: ItemTypeIssue, Repo: "org/repo", Number: 1, ParentNumber: 2, ParentRepo: "org/repo"},
		{ID: "b", Type: ItemTypeIssue, Repo: "org/repo", Number: 2, ParentNumber: 1, ParentRepo: "org/repo"},
		{ID: "leaf", Type: ItemTypeIssue, Repo: "org/repo", Number: 5, ParentNumber: 1, ParentRepo: "org/repo"},
	}

	sorted := TreeSortItems(items)

	require.Len(t, sorted, 3)
	idx := make(map[string]int)
	for i, it := range sorted {
		idx[it.ID] = i
	}
	assert.Greater(t, idx["leaf"], idx["a"], "leaf must come after its parent a")
}
