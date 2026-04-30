package data

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/log/v2"
	gh "github.com/cli/go-gh/v2/pkg/api"
	graphql "github.com/cli/shurcooL-graphql"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/persistcache"
)

const (
	projectsPageSize = 100
	projectsCacheTTL = time.Hour
)

// projectNode is the shape returned by GitHub's projectsV2 GraphQL nodes.
// Explicit graphql tags on Id and Url avoid misgeneration from all-caps identifiers.
type projectNode struct {
	Id        string `graphql:"id"`
	Number    int
	Title     string
	Url       string `graphql:"url"`
	Closed    bool
	Public    bool
	UpdatedAt time.Time
	Items     struct {
		TotalCount int
	}
}

// FetchProjects fetches GitHub ProjectsV2 projects for the given owners.
// An empty owners slice triggers a viewer fallback query.
// Per-owner failures are isolated: partial results plus a joined error are returned.
// When cache is nil, caching is skipped. When FF_MOCK_DATA is set, fixture files are returned.
func FetchProjects(
	cache *persistcache.Store,
	owners []OwnerRef,
	filters ProjectFilters,
) ([]ProjectData, error) {
	if config.IsFeatureEnabled(config.FF_MOCK_DATA) {
		return fetchProjectsMockData(owners, filters)
	}

	if err := ensureProjectsClient(); err != nil {
		return nil, fmt.Errorf("FetchProjects: init client: %w", err)
	}

	seen := make(map[string]bool)
	var all []ProjectData
	var errs []error

	if len(owners) == 0 {
		projects, err := fetchViewerProjects(cache)
		if err != nil {
			errs = append(errs, fmt.Errorf("viewer: %w", err))
		} else {
			for _, p := range projects {
				if !seen[p.ID] {
					seen[p.ID] = true
					all = append(all, p)
				}
			}
		}
	} else {
		for _, owner := range owners {
			projects, err := fetchOwnerProjects(cache, owner)
			if err != nil {
				log.Warn("FetchProjects: failed for owner",
					"owner_kind", ownerKindString(owner.Kind),
					"owner_login", owner.Login,
					"err", err,
				)
				errs = append(errs, fmt.Errorf("owner %s: %w", owner.Login, err))
				continue
			}
			for _, p := range projects {
				if !seen[p.ID] {
					seen[p.ID] = true
					all = append(all, p)
				}
			}
		}
	}

	return applyProjectFilters(all, filters), errors.Join(errs...)
}

// ensureProjectsClient lazily initialises the shared package-level GraphQL client.
// Mirrors the FF_MOCK_DATA handling from prapi.go so the same mock server is used
// across all data-layer functions when the flag is active.
func ensureProjectsClient() error {
	if client != nil {
		return nil
	}
	var err error
	if config.IsFeatureEnabled(config.FF_MOCK_DATA) {
		log.Info("using mock data", "server", "https://localhost:3000")
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{ //nolint:gosec
			InsecureSkipVerify: true,
		}
		client, err = gh.NewGraphQLClient(gh.ClientOptions{Host: "localhost:3000", AuthToken: "fake-token"})
	} else {
		client, err = gh.DefaultGraphQLClient()
	}
	return err
}

func fetchOwnerProjects(cache *persistcache.Store, owner OwnerRef) ([]ProjectData, error) {
	start := time.Now()
	kindStr := ownerKindString(owner.Kind)
	cacheKey := fmt.Sprintf("projects/%s/%s", kindStr, owner.Login)

	if cache != nil {
		if data, hit, err := cache.Get(cacheKey); err != nil {
			log.Warn("FetchProjects: cache read error", "key", cacheKey, "err", err)
		} else if hit {
			var projects []ProjectData
			if jsonErr := json.Unmarshal(data, &projects); jsonErr == nil {
				log.Debug("FetchProjects: cache hit",
					"owner_kind", kindStr,
					"owner_login", owner.Login,
					"project_count", len(projects),
					"elapsed_ms", time.Since(start).Milliseconds(),
					"cache_hit", true,
				)
				return projects, nil
			}
		}
	}

	nodes, err := fetchProjectNodesForOwner(owner)
	if err != nil {
		return nil, err
	}

	projects := make([]ProjectData, 0, len(nodes))
	for _, n := range nodes {
		projects = append(projects, projectNodeToData(n, owner))
	}

	log.Debug("FetchProjects: fetched",
		"owner_kind", kindStr,
		"owner_login", owner.Login,
		"project_count", len(projects),
		"elapsed_ms", time.Since(start).Milliseconds(),
		"cache_hit", false,
	)

	if cache != nil {
		if data, marshalErr := json.Marshal(projects); marshalErr == nil {
			if putErr := cache.Put(cacheKey, data, projectsCacheTTL); putErr != nil {
				log.Warn("FetchProjects: cache write error", "key", cacheKey, "err", putErr)
			}
		}
	}

	return projects, nil
}

func fetchViewerProjects(cache *persistcache.Store) ([]ProjectData, error) {
	start := time.Now()
	const cacheKey = "projects/viewer"

	if cache != nil {
		if data, hit, err := cache.Get(cacheKey); err != nil {
			log.Warn("FetchProjects: cache read error", "key", cacheKey, "err", err)
		} else if hit {
			var projects []ProjectData
			if jsonErr := json.Unmarshal(data, &projects); jsonErr == nil {
				log.Debug("FetchProjects: cache hit (viewer)",
					"project_count", len(projects),
					"elapsed_ms", time.Since(start).Milliseconds(),
					"cache_hit", true,
				)
				return projects, nil
			}
		}
	}

	var queryResult struct {
		Viewer struct {
			ProjectsV2 struct {
				Nodes []projectNode
			} `graphql:"projectsV2(first: $first)"`
		}
	}
	variables := map[string]any{
		"first": graphql.Int(projectsPageSize),
	}

	if err := client.Query("ViewerProjects", &queryResult, variables); err != nil {
		return nil, fmt.Errorf("viewer query: %w", err)
	}

	// Viewer projects are attributed to a synthetic user owner with login "viewer".
	viewerOwner := OwnerRef{Kind: OwnerKindUser, Login: "viewer"}
	projects := make([]ProjectData, 0, len(queryResult.Viewer.ProjectsV2.Nodes))
	for _, n := range queryResult.Viewer.ProjectsV2.Nodes {
		projects = append(projects, projectNodeToData(n, viewerOwner))
	}

	log.Debug("FetchProjects: fetched (viewer)",
		"project_count", len(projects),
		"elapsed_ms", time.Since(start).Milliseconds(),
		"cache_hit", false,
	)

	if cache != nil {
		if data, marshalErr := json.Marshal(projects); marshalErr == nil {
			if putErr := cache.Put(cacheKey, data, projectsCacheTTL); putErr != nil {
				log.Warn("FetchProjects: cache write error", "key", cacheKey, "err", putErr)
			}
		}
	}

	return projects, nil
}

// fetchProjectNodesForOwner dispatches the correct GraphQL query shape for the owner kind.
func fetchProjectNodesForOwner(owner OwnerRef) ([]projectNode, error) {
	switch owner.Kind {
	case OwnerKindOrg:
		var queryResult struct {
			Organization struct {
				ProjectsV2 struct {
					Nodes []projectNode
				} `graphql:"projectsV2(first: $first)"`
			} `graphql:"organization(login: $login)"`
		}
		variables := map[string]any{
			"login": graphql.String(owner.Login),
			"first": graphql.Int(projectsPageSize),
		}
		if err := client.Query("OrgProjects", &queryResult, variables); err != nil {
			return nil, fmt.Errorf("org query: %w", err)
		}
		return queryResult.Organization.ProjectsV2.Nodes, nil

	case OwnerKindUser:
		var queryResult struct {
			User struct {
				ProjectsV2 struct {
					Nodes []projectNode
				} `graphql:"projectsV2(first: $first)"`
			} `graphql:"user(login: $login)"`
		}
		variables := map[string]any{
			"login": graphql.String(owner.Login),
			"first": graphql.Int(projectsPageSize),
		}
		if err := client.Query("UserProjects", &queryResult, variables); err != nil {
			return nil, fmt.Errorf("user query: %w", err)
		}
		return queryResult.User.ProjectsV2.Nodes, nil

	default:
		return nil, fmt.Errorf("unknown owner kind: %s", owner.Kind)
	}
}

func projectNodeToData(n projectNode, owner OwnerRef) ProjectData {
	return ProjectData{
		ID:         n.Id,
		Number:     fmt.Sprintf("%d", n.Number),
		Title:      n.Title,
		URL:        n.Url,
		Owner:      owner,
		Closed:     n.Closed,
		Public:     n.Public,
		ItemsCount: n.Items.TotalCount,
		UpdatedAt:  n.UpdatedAt,
	}
}

// applyProjectFilters applies client-side filters to the project list.
func applyProjectFilters(projects []ProjectData, filters ProjectFilters) []ProjectData {
	if filters.Closed == nil && filters.TitleContains == "" {
		return projects
	}
	result := make([]ProjectData, 0, len(projects))
	titleLower := strings.ToLower(filters.TitleContains)
	for _, p := range projects {
		if filters.Closed != nil && p.Closed != *filters.Closed {
			continue
		}
		if titleLower != "" && !strings.Contains(strings.ToLower(p.Title), titleLower) {
			continue
		}
		result = append(result, p)
	}
	return result
}

func ownerKindString(kind OwnerKind) string {
	if kind == OwnerKindOrg || kind == OwnerKindUser {
		return string(kind)
	}
	return "unknown"
}

// --------------------------------------------------------------------------
// FetchProjectItems — cursor-paginated project item fetch
// --------------------------------------------------------------------------

const (
	projectItemsCacheTTL  = 5 * time.Minute
	projectFieldsMaxFirst = 50
	// projectFieldValuesMax is the per-item fieldValues ceiling sent to GraphQL.
	// The literal "20" in the fieldValues tag below must stay in sync with this value.
	projectFieldValuesMax = 20
)

// gqlFieldRef is the common shape for a field reference inside a field-value node.
// All ProjectV2FieldCommon implementors expose id directly.
type gqlFieldRef struct {
	Id string `graphql:"id"`
}

// gqlFieldOption is one option in a single-select field's option list.
type gqlFieldOption struct {
	Id    string `graphql:"id"`
	Name  string
	Color string
}

// gqlFieldNode represents a ProjectV2FieldConfiguration union node.
type gqlFieldNode struct {
	TypeName string `graphql:"__typename"`
	AsField  struct {
		Id       string `graphql:"id"`
		Name     string
		DataType string
	} `graphql:"... on ProjectV2Field"`
	AsSingleSelect struct {
		Id      string `graphql:"id"`
		Name    string
		Options []gqlFieldOption
	} `graphql:"... on ProjectV2SingleSelectField"`
	AsIteration struct {
		Id   string `graphql:"id"`
		Name string
	} `graphql:"... on ProjectV2IterationField"`
}

// gqlFieldValueNode represents a ProjectV2ItemFieldValue union node.
type gqlFieldValueNode struct {
	TypeName       string `graphql:"__typename"`
	AsSingleSelect struct {
		Field    gqlFieldRef
		OptionId string
		Name     string
	} `graphql:"... on ProjectV2ItemFieldSingleSelectValue"`
	AsNumber struct {
		Field  gqlFieldRef
		Number *float64
	} `graphql:"... on ProjectV2ItemFieldNumberValue"`
	AsDate struct {
		Field gqlFieldRef
		Date  string
	} `graphql:"... on ProjectV2ItemFieldDateValue"`
	AsIteration struct {
		Field       gqlFieldRef
		IterationId string
		Title       string
		StartDate   string
		Duration    int
	} `graphql:"... on ProjectV2ItemFieldIterationValue"`
	AsText struct {
		Field gqlFieldRef
		Text  string
	} `graphql:"... on ProjectV2ItemFieldTextValue"`
}

// gqlItemNode represents a single ProjectV2Item from the items connection.
type gqlItemNode struct {
	Id       string `graphql:"id"`
	ItemType string `graphql:"type"`
	Content  struct {
		AsIssue struct {
			Title      string
			Number     int
			Url        string `graphql:"url"`
			Repository struct {
				NameWithOwner string
			}
		} `graphql:"... on Issue"`
		AsPR struct {
			Title      string
			Number     int
			Url        string `graphql:"url"`
			Repository struct {
				NameWithOwner string
			}
		} `graphql:"... on PullRequest"`
		AsDraftIssue struct {
			Title string
		} `graphql:"... on DraftIssue"`
	}
	FieldValues struct {
		Nodes    []gqlFieldValueNode
		PageInfo struct {
			HasNextPage bool
			EndCursor   string
		}
	} `graphql:"fieldValues(first: 20)"`
	UpdatedAt time.Time
}

// FetchProjectItems pages items for a given project.
//
// after empty ⇒ first page (checks cache). after non-empty ⇒ load-more:
// reads existing cached items, fetches the next page, stitches, and rewrites
// the cache file atomically.
//
// extraFieldNames are YAML-configured field names that get resolved to field
// IDs at schema time and stored in ProjectSchema.ExtraFields.
func FetchProjectItems(
	cache *persistcache.Store,
	projectID, after string,
	limit int,
	extraFieldNames []string,
) (ProjectSchema, []ProjectItemData, PageInfo, error) {
	if config.IsFeatureEnabled(config.FF_MOCK_DATA) {
		return fetchProjectItemsMockData(projectID)
	}

	cacheKey := fmt.Sprintf("project-items/%s", projectID)
	isFirstPage := after == ""

	// First-page cache hit: return immediately without hitting the network.
	if isFirstPage && cache != nil {
		if raw, hit, err := cache.Get(cacheKey); err != nil {
			log.Warn("FetchProjectItems: cache read error", "key", cacheKey, "err", err)
		} else if hit {
			var cached projectItemsCache
			if jsonErr := json.Unmarshal(raw, &cached); jsonErr == nil {
				log.Debug("FetchProjectItems: cache hit",
					"project_id", projectID,
					"item_count", len(cached.Items),
				)
				return cached.Schema, cached.Items, cached.PageInfo, nil
			}
		}
	}

	if err := ensureProjectsClient(); err != nil {
		return ProjectSchema{}, nil, PageInfo{}, fmt.Errorf("FetchProjectItems: init client: %w", err)
	}

	// Load-more: read existing items from cache to stitch with the new page.
	var existingItems []ProjectItemData
	if !isFirstPage && cache != nil {
		if raw, hit, err := cache.Get(cacheKey); err == nil && hit {
			var cached projectItemsCache
			if jsonErr := json.Unmarshal(raw, &cached); jsonErr == nil {
				existingItems = cached.Items
			}
		}
	}

	// Capture generation before the network call so PutIfFresh can detect
	// a concurrent invalidation.
	var capturedGen uint64
	if cache != nil {
		capturedGen = cache.Generation("project-items/")
	}

	// Build optional after-cursor.
	var afterCursor *graphql.String
	if after != "" {
		s := graphql.String(after)
		afterCursor = &s
	}

	var queryResult struct {
		Node struct {
			ProjectV2 struct {
				Fields struct {
					Nodes    []gqlFieldNode
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
				} `graphql:"fields(first: 50)"`
				Items struct {
					TotalCount int
					PageInfo   struct {
						HasNextPage bool
						EndCursor   string
					}
					Nodes []gqlItemNode
				} `graphql:"items(first: $first, after: $after)"`
			} `graphql:"... on ProjectV2"`
		} `graphql:"node(id: $projectId)"`
	}

	variables := map[string]any{
		"projectId": graphql.String(projectID),
		"first":     graphql.Int(limit),
		"after":     afterCursor,
	}

	log.Debug("FetchProjectItems: querying", "project_id", projectID, "after", after, "limit", limit)

	if err := client.Query("FetchProjectItems", &queryResult, variables); err != nil {
		return ProjectSchema{}, nil, PageInfo{}, fmt.Errorf("FetchProjectItems: query: %w", err)
	}

	proj := queryResult.Node.ProjectV2

	// Warn if the fields connection is truncated.
	if proj.Fields.PageInfo.HasNextPage {
		log.Warn(fmt.Sprintf("FetchProjectItems: fields(first:%d) truncated — some fields may be missing", projectFieldsMaxFirst),
			"project_id", projectID)
	}

	// Parse schema from the fields connection.
	schema := parseProjectSchema(proj.Fields.Nodes, extraFieldNames)

	// Status-field fallback: if fields were truncated and Status is still
	// unset, paginate beyond field #50 to find it.
	if schema.StatusField == nil && proj.Fields.PageInfo.HasNextPage {
		log.Warn(fmt.Sprintf("FetchProjectItems: Status field not in first %d fields, running fallback", projectFieldsMaxFirst),
			"project_id", projectID)
		statusField, err := fetchStatusFieldFallback(projectID, proj.Fields.PageInfo.EndCursor)
		if err != nil {
			log.Warn("FetchProjectItems: Status fallback failed", "project_id", projectID, "err", err)
		} else if statusField != nil {
			schema.StatusField = statusField
		}
	}

	// Parse items.
	newItems := make([]ProjectItemData, 0, len(proj.Items.Nodes))
	for _, node := range proj.Items.Nodes {
		if node.FieldValues.PageInfo.HasNextPage {
			log.Warn(fmt.Sprintf("FetchProjectItems: fieldValues(first:%d) truncated", projectFieldValuesMax),
				"project_id", projectID, "item_id", node.Id)
		}
		newItems = append(newItems, parseProjectItem(node))
	}

	// Stitch new items onto any existing cached items (load-more).
	allItems := append(existingItems, newItems...)

	pageInfo := PageInfo{
		HasNextPage: proj.Items.PageInfo.HasNextPage,
		EndCursor:   proj.Items.PageInfo.EndCursor,
	}

	// Write accumulated result to cache.
	if cache != nil {
		cached := projectItemsCache{
			Schema:   schema,
			Items:    allItems,
			PageInfo: pageInfo,
		}
		if raw, err := json.Marshal(cached); err == nil {
			if putErr := cache.PutIfFresh(cacheKey, raw, projectItemsCacheTTL, capturedGen); putErr != nil {
				if putErr != persistcache.ErrStaleGeneration {
					log.Warn("FetchProjectItems: cache write error", "key", cacheKey, "err", putErr)
				}
			}
		}
	}

	log.Debug("FetchProjectItems: done",
		"project_id", projectID,
		"new_items", len(newItems),
		"total_items", len(allItems),
		"has_next_page", pageInfo.HasNextPage,
	)

	return schema, allItems, pageInfo, nil
}

// fetchStatusFieldFallback cursor-paginates through fields beyond the first
// 50 to find the "Status" single-select field.
// Called only when fields(first:50) returned hasNextPage=true and no Status
// field was found in the initial result.
func fetchStatusFieldFallback(projectID, afterCursor string) (*StatusFieldDef, error) {
	cursor := afterCursor
	for {
		var q struct {
			Node struct {
				ProjectV2 struct {
					Fields struct {
						Nodes    []gqlFieldNode
						PageInfo struct {
							HasNextPage bool
							EndCursor   string
						}
					} `graphql:"fields(first: 50, after: $fieldAfter)"`
				} `graphql:"... on ProjectV2"`
			} `graphql:"node(id: $projectId)"`
		}
		vars := map[string]any{
			"projectId":  graphql.String(projectID),
			"fieldAfter": graphql.String(cursor),
		}
		if err := client.Query("FetchProjectFieldsFallback", &q, vars); err != nil {
			return nil, err
		}
		for _, node := range q.Node.ProjectV2.Fields.Nodes {
			if node.AsSingleSelect.Name == "Status" && node.AsSingleSelect.Id != "" {
				def := &StatusFieldDef{ID: node.AsSingleSelect.Id}
				for _, opt := range node.AsSingleSelect.Options {
					def.Options = append(def.Options, StatusOption{
						ID:    opt.Id,
						Name:  opt.Name,
						Color: opt.Color,
					})
				}
				return def, nil
			}
		}
		if !q.Node.ProjectV2.Fields.PageInfo.HasNextPage {
			break
		}
		cursor = q.Node.ProjectV2.Fields.PageInfo.EndCursor
	}
	return nil, nil // Status field not found
}

// parseProjectSchema builds a ProjectSchema from a slice of field nodes.
// extraFieldNames are resolved to field IDs; duplicates within the project
// log a warn and the first match wins.
func parseProjectSchema(fieldNodes []gqlFieldNode, extraFieldNames []string) ProjectSchema {
	type namedField struct {
		id   string
		name string
		node gqlFieldNode
	}

	var orderedFields []namedField
	seenNames := make(map[string]bool)

	for _, node := range fieldNodes {
		var id, name string
		switch {
		case node.AsSingleSelect.Id != "":
			id, name = node.AsSingleSelect.Id, node.AsSingleSelect.Name
		case node.AsField.Id != "":
			id, name = node.AsField.Id, node.AsField.Name
		case node.AsIteration.Id != "":
			id, name = node.AsIteration.Id, node.AsIteration.Name
		default:
			continue
		}
		if seenNames[name] {
			log.Warn("FetchProjectItems: duplicate field name in project schema",
				"field_name", name)
			continue // first match wins
		}
		seenNames[name] = true
		orderedFields = append(orderedFields, namedField{id: id, name: name, node: node})
	}

	// Detect Status field.
	var statusField *StatusFieldDef
	for _, f := range orderedFields {
		if f.name == "Status" && f.node.AsSingleSelect.Id != "" {
			def := &StatusFieldDef{ID: f.id}
			for _, opt := range f.node.AsSingleSelect.Options {
				def.Options = append(def.Options, StatusOption{
					ID:    opt.Id,
					Name:  opt.Name,
					Color: opt.Color,
				})
			}
			statusField = def
			break
		}
	}

	// Resolve extra fields by name.
	nameToField := make(map[string]namedField, len(orderedFields))
	for _, f := range orderedFields {
		nameToField[f.name] = f
	}

	extraFields := make(map[string]FieldDef)
	var extraFieldOrder []string
	for _, name := range extraFieldNames {
		if f, ok := nameToField[name]; ok {
			if _, exists := extraFields[f.id]; !exists {
				extraFields[f.id] = FieldDef{ID: f.id, Name: f.name}
				extraFieldOrder = append(extraFieldOrder, f.id)
			}
		}
	}

	return ProjectSchema{
		StatusField:     statusField,
		ExtraFields:     extraFields,
		ExtraFieldOrder: extraFieldOrder,
	}
}

// parseItemType converts a raw GraphQL type string to an ItemType constant.
func parseItemType(s string) ItemType {
	switch s {
	case "ISSUE":
		return ItemTypeIssue
	case "PULL_REQUEST":
		return ItemTypePullRequest
	case "DRAFT_ISSUE":
		return ItemTypeDraftIssue
	case "REDACTED":
		return ItemTypeRedacted
	default:
		return ItemTypeRedacted
	}
}

// parseProjectItem converts a gqlItemNode to a ProjectItemData.
func parseProjectItem(node gqlItemNode) ProjectItemData {
	itemType := parseItemType(node.ItemType)

	var title, repo, url string
	switch itemType {
	case ItemTypeIssue:
		title = node.Content.AsIssue.Title
		repo = node.Content.AsIssue.Repository.NameWithOwner
		url = node.Content.AsIssue.Url
	case ItemTypePullRequest:
		title = node.Content.AsPR.Title
		repo = node.Content.AsPR.Repository.NameWithOwner
		url = node.Content.AsPR.Url
	case ItemTypeDraftIssue:
		title = node.Content.AsDraftIssue.Title
		// repo and url remain empty
	case ItemTypeRedacted:
		// all remain empty
	}

	fields := make(FieldValues)
	for _, fvNode := range node.FieldValues.Nodes {
		fieldID, fv := parseFieldValue(fvNode)
		if fieldID != "" {
			fields[fieldID] = fv
		}
	}

	return ProjectItemData{
		ID:        node.Id,
		Type:      itemType,
		Title:     title,
		Repo:      repo,
		URL:       url,
		Fields:    fields,
		UpdatedAt: node.UpdatedAt,
	}
}

// parseFieldValue converts a gqlFieldValueNode to a (fieldID, FieldValue) pair.
// Returns ("", FieldValueUnknown{}) for unrecognised or empty nodes.
func parseFieldValue(node gqlFieldValueNode) (string, FieldValue) {
	switch node.TypeName {
	case "ProjectV2ItemFieldSingleSelectValue":
		fieldID := node.AsSingleSelect.Field.Id
		return fieldID, FieldValueSingleSelect{
			OptionID: node.AsSingleSelect.OptionId,
			Name:     node.AsSingleSelect.Name,
		}
	case "ProjectV2ItemFieldNumberValue":
		fieldID := node.AsNumber.Field.Id
		if node.AsNumber.Number == nil {
			return fieldID, FieldValueUnknown{}
		}
		return fieldID, FieldValueNumber{Number: *node.AsNumber.Number}
	case "ProjectV2ItemFieldDateValue":
		fieldID := node.AsDate.Field.Id
		return fieldID, FieldValueDate{Date: node.AsDate.Date}
	case "ProjectV2ItemFieldIterationValue":
		fieldID := node.AsIteration.Field.Id
		return fieldID, FieldValueIteration{
			IterationID: node.AsIteration.IterationId,
			Title:       node.AsIteration.Title,
			StartDate:   node.AsIteration.StartDate,
			Duration:    node.AsIteration.Duration,
		}
	case "ProjectV2ItemFieldTextValue":
		fieldID := node.AsText.Field.Id
		return fieldID, FieldValueText{Text: node.AsText.Text}
	default:
		return "", FieldValueUnknown{}
	}
}

// fetchProjectItemsMockData returns canned fixture data from
// testdata/projects/items/<projectID>.json. The fixture is in
// projectItemsCache format, ignoring the after cursor (mock always
// returns the full first-page fixture).
func fetchProjectItemsMockData(projectID string) (ProjectSchema, []ProjectItemData, PageInfo, error) {
	path := filepath.Join("testdata", "projects", "items", projectID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ProjectSchema{}, nil, PageInfo{}, nil
		}
		return ProjectSchema{}, nil, PageInfo{}, fmt.Errorf("mock data read %q: %w", path, err)
	}
	var cached projectItemsCache
	if err := json.Unmarshal(raw, &cached); err != nil {
		return ProjectSchema{}, nil, PageInfo{}, fmt.Errorf("mock data parse %q: %w", path, err)
	}
	return cached.Schema, cached.Items, cached.PageInfo, nil
}

// fetchProjectsMockData returns canned fixture data from testdata/projects/ for development.
// Fixture files contain JSON-encoded []ProjectData per owner kind/login.
func fetchProjectsMockData(owners []OwnerRef, filters ProjectFilters) ([]ProjectData, error) {
	seen := make(map[string]bool)
	var all []ProjectData

	for _, path := range mockProjectFilePaths(owners) {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("mock data read %q: %w", path, err)
		}
		var projects []ProjectData
		if err := json.Unmarshal(data, &projects); err != nil {
			return nil, fmt.Errorf("mock data parse %q: %w", path, err)
		}
		for _, p := range projects {
			if !seen[p.ID] {
				seen[p.ID] = true
				all = append(all, p)
			}
		}
	}

	return applyProjectFilters(all, filters), nil
}

// mockProjectFilePaths returns fixture file paths relative to the package directory.
// An empty owners list maps to the viewer fallback fixture.
func mockProjectFilePaths(owners []OwnerRef) []string {
	if len(owners) == 0 {
		return []string{filepath.Join("testdata", "projects", "viewer.json")}
	}
	paths := make([]string, 0, len(owners))
	for _, o := range owners {
		paths = append(paths, filepath.Join("testdata", "projects", ownerKindString(o.Kind), o.Login+".json"))
	}
	return paths
}

// --------------------------------------------------------------------------
// UpdateItemStatus — projectV2 single-select status mutation
// --------------------------------------------------------------------------

// gqlUpdateStatusResult is the GraphQL response shape for updateProjectV2ItemFieldValue.
type gqlUpdateStatusResult struct {
	UpdateProjectV2ItemFieldValue struct {
		ProjectV2Item struct {
			Id          string `graphql:"id"`
			FieldValues struct {
				Nodes []gqlFieldValueNode
			} `graphql:"fieldValues(first: 20)"`
			UpdatedAt time.Time
		}
	} `graphql:"updateProjectV2ItemFieldValue(input: $input)"`
}

// gqlClearStatusResult is the GraphQL response shape for clearProjectV2ItemFieldValue.
type gqlClearStatusResult struct {
	ClearProjectV2ItemFieldValue struct {
		ProjectV2Item struct {
			Id          string `graphql:"id"`
			FieldValues struct {
				Nodes []gqlFieldValueNode
			} `graphql:"fieldValues(first: 20)"`
			UpdatedAt time.Time
		}
	} `graphql:"clearProjectV2ItemFieldValue(input: $input)"`
}

// UpdateProjectV2ItemFieldValueInput matches the GitHub GraphQL input type.
type UpdateProjectV2ItemFieldValueInput struct {
	ProjectID string                        `json:"projectId"`
	ItemID    string                        `json:"itemId"`
	FieldID   string                        `json:"fieldId"`
	Value     UpdateProjectV2ItemFieldValue `json:"value"`
}

// UpdateProjectV2ItemFieldValue holds the single-select option ID.
type UpdateProjectV2ItemFieldValue struct {
	SingleSelectOptionID string `json:"singleSelectOptionId"`
}

// ClearProjectV2ItemFieldValueInput matches the GitHub GraphQL input type.
type ClearProjectV2ItemFieldValueInput struct {
	ProjectID string `json:"projectId"`
	ItemID    string `json:"itemId"`
	FieldID   string `json:"fieldId"`
}

// UpdateItemStatus mutates the Status field of a project item.
//
//   - optionID == nil  → clearProjectV2ItemFieldValue (removes the status)
//   - optionID != nil  → updateProjectV2ItemFieldValue (sets the status to the given option)
//
// On success the function:
//  1. Returns a ProjectItemData built from the mutation response (for reconciliation).
//  2. Invalidates the project-items cache prefix so the next FetchProjectItems returns fresh data.
func UpdateItemStatus(
	cache *persistcache.Store,
	projectID, itemID, statusFieldID string,
	optionID *string,
) (ProjectItemData, error) {
	if err := ensureProjectsClient(); err != nil {
		return ProjectItemData{}, fmt.Errorf("UpdateItemStatus: init client: %w", err)
	}

	var (
		resultID        string
		resultFieldVals []gqlFieldValueNode
		resultUpdatedAt time.Time
	)

	if optionID == nil {
		// Clear the status field.
		var mutResult gqlClearStatusResult
		vars := map[string]any{
			"input": map[string]any{
				"projectId": projectID,
				"itemId":    itemID,
				"fieldId":   statusFieldID,
			},
		}
		if err := client.Mutate("ClearProjectItemStatus", &mutResult, vars); err != nil {
			return ProjectItemData{}, fmt.Errorf("UpdateItemStatus: clear mutation: %w", err)
		}
		item := mutResult.ClearProjectV2ItemFieldValue.ProjectV2Item
		resultID = item.Id
		resultFieldVals = item.FieldValues.Nodes
		resultUpdatedAt = item.UpdatedAt
	} else {
		// Set the status field.
		var mutResult gqlUpdateStatusResult
		vars := map[string]any{
			"input": map[string]any{
				"projectId": projectID,
				"itemId":    itemID,
				"fieldId":   statusFieldID,
				"value": map[string]any{
					"singleSelectOptionId": *optionID,
				},
			},
		}
		if err := client.Mutate("UpdateProjectItemStatus", &mutResult, vars); err != nil {
			return ProjectItemData{}, fmt.Errorf("UpdateItemStatus: update mutation: %w", err)
		}
		item := mutResult.UpdateProjectV2ItemFieldValue.ProjectV2Item
		resultID = item.Id
		resultFieldVals = item.FieldValues.Nodes
		resultUpdatedAt = item.UpdatedAt
	}

	// Build the returned item from the mutation response.
	fields := make(FieldValues)
	for _, fvNode := range resultFieldVals {
		fieldID, fv := parseFieldValue(fvNode)
		if fieldID != "" {
			fields[fieldID] = fv
		}
	}
	updatedItem := ProjectItemData{
		ID:        resultID,
		Fields:    fields,
		UpdatedAt: resultUpdatedAt,
	}

	// Invalidate the per-project items cache so PutIfFresh calls from in-flight
	// load-more goroutines are rejected and the next FetchProjectItems is fresh.
	if cache != nil {
		cacheKey := fmt.Sprintf("project-items/%s", projectID)
		if err := cache.Invalidate(cacheKey); err != nil {
			log.Warn("UpdateItemStatus: cache invalidation error", "key", cacheKey, "err", err)
		}
	}

	newValue := "(cleared)"
	if optionID != nil {
		newValue = *optionID
	}
	log.Debug("UpdateItemStatus: success",
		"item_id", itemID,
		"field_id", statusFieldID,
		"new_value", newValue,
	)

	return updatedItem, nil
}
