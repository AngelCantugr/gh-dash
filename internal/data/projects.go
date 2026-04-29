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

const projectsPageSize = 100
const projectsCacheTTL = time.Hour

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
		ID:        n.Id,
		Number:    fmt.Sprintf("%d", n.Number),
		Title:     n.Title,
		URL:       n.Url,
		Owner:     owner,
		Closed:    n.Closed,
		Public:    n.Public,
		ItemsCount: n.Items.TotalCount,
		UpdatedAt: n.UpdatedAt,
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
