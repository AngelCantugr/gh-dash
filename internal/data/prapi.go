package data

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"charm.land/log/v2"
	gh "github.com/cli/go-gh/v2/pkg/api"
	graphql "github.com/cli/shurcooL-graphql"
	checks "github.com/dlvhdr/x/gh-checks"
	"github.com/shurcooL/githubv4"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/persistcache"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

func jsonMarshal(v any) ([]byte, error)   { return json.Marshal(v) }
func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

type SuggestedReviewer struct {
	IsAuthor    bool
	IsCommenter bool
	Reviewer    struct {
		Login string
	}
}

type EnrichedPullRequestData struct {
	Url            string
	Number         int
	Title          string
	Body           string
	State          string
	IsDraft        bool
	IsInMergeQueue bool
	Author         struct {
		Login string
	}
	AuthorAssociation string
	UpdatedAt         time.Time
	CreatedAt         time.Time
	Mergeable         string
	ReviewDecision    string
	Additions         int
	Deletions         int
	HeadRefName       string
	BaseRefName       string
	HeadRepository    struct {
		Name string
	}
	HeadRef struct {
		Name string
	}
	Labels             PRLabels  `graphql:"labels(first: 6)"`
	Assignees          Assignees `graphql:"assignees(first: 3)"`
	Repository         Repository
	Commits            LastCommitWithStatusChecks `graphql:"commits(last: 1)"`
	AllCommits         AllCommits                 `graphql:"allCommits: commits(last: 100)"`
	Comments           CommentsWithBody           `graphql:"comments(last: 50, orderBy: { field: UPDATED_AT, direction: DESC })"`
	ReviewThreads      ReviewThreadsWithComments  `graphql:"reviewThreads(last: 50)"`
	ReviewRequests     ReviewRequests             `graphql:"reviewRequests(last: 100)"`
	Reviews            Reviews                    `graphql:"reviews(last: 100)"`
	SuggestedReviewers []SuggestedReviewer
	Files              ChangedFiles `graphql:"files(first: 5)"`
}

type PullRequestData struct {
	Number int
	Title  string
	Body   string
	Author struct {
		Login string
	}
	AuthorAssociation string
	UpdatedAt         time.Time
	CreatedAt         time.Time
	Url               string
	State             string
	Mergeable         string
	ReviewDecision    string
	Additions         int
	Deletions         int
	HeadRefName       string
	BaseRefName       string
	HeadRepository    struct {
		Name string
	}
	HeadRef struct {
		Name string
	}
	Repository       Repository
	Assignees        Assignees      `graphql:"assignees(first: 3)"`
	Comments         Comments       `graphql:"comments"`
	ReviewThreads    ReviewThreads  `graphql:"reviewThreads"`
	Reviews          Reviews        `graphql:"reviews(last: 3)"`
	ReviewRequests   ReviewRequests `graphql:"reviewRequests(last: 5)"`
	Files            ChangedFiles   `graphql:"files(first: 5)"`
	IsDraft          bool
	IsInMergeQueue   bool
	Commits          Commits          `graphql:"commits(last: 1)"`
	Labels           PRLabels         `graphql:"labels(first: 6)"`
	MergeStateStatus MergeStateStatus `graphql:"mergeStateStatus"`
}

type CheckRun struct {
	Name       graphql.String
	Status     graphql.String
	Conclusion checks.CheckRunState
	CheckSuite struct {
		Creator struct {
			Login graphql.String
		}
		WorkflowRun struct {
			Workflow struct {
				Name graphql.String
			}
		}
	}
}

type StatusContext struct {
	Context graphql.String
	State   graphql.String
	Creator struct {
		Login graphql.String
	}
}

type CheckSuiteNode struct {
	Status     graphql.String
	Conclusion graphql.String

	App struct {
		Name graphql.String
	}

	WorkflowRun struct {
		Workflow struct {
			Name graphql.String
		}
	}
}

type CheckSuites struct {
	TotalCount graphql.Int
	Nodes      []CheckSuiteNode
}

type StatusCheckRollupStats struct {
	State    checks.CommitState
	Contexts struct {
		TotalCount                 graphql.Int
		CheckRunCount              graphql.Int
		CheckRunCountsByState      []ContextCountByState
		StatusContextCount         graphql.Int
		StatusContextCountsByState []ContextCountByState
	} `graphql:"contexts(last: 1)"`
}

type AllCommits struct {
	Nodes []struct {
		Commit struct {
			AbbreviatedOid  string
			CommittedDate   time.Time
			MessageHeadline string
			Author          struct {
				Name string
				User struct {
					Login string
				}
			}
			StatusCheckRollup StatusCheckRollupStats
		}
	}
}

type LastCommitWithStatusChecks struct {
	Nodes []struct {
		Commit struct {
			Deployments struct {
				Nodes []struct {
					Task        graphql.String
					Description graphql.String
				}
			} `graphql:"deployments(last: 10)"`
			CommitUrl         graphql.String
			StatusCheckRollup struct {
				State    graphql.String
				Contexts struct {
					TotalCount                 graphql.Int
					CheckRunCount              graphql.Int
					CheckRunCountsByState      []ContextCountByState
					StatusContextCount         graphql.Int
					StatusContextCountsByState []ContextCountByState
					Nodes                      []struct {
						Typename      graphql.String `graphql:"__typename"`
						CheckRun      CheckRun       `graphql:"... on CheckRun"`
						StatusContext StatusContext  `graphql:"... on StatusContext"`
					}
				} `graphql:"contexts(last: 100)"`
			}
			// CheckSuites are fetched separately from StatusCheckRollup because
			// workflows awaiting approval (conclusion ACTION_REQUIRED) and workflows
			// still queued have no CheckRun objects yet, so they don’t appear in
			// StatusCheckRollup.contexts.
			CheckSuites CheckSuites `graphql:"checkSuites(last: 20)"`
		}
	}
	TotalCount int
}

type CommentsWithBody struct {
	TotalCount graphql.Int
	Nodes      []Comment
}

type ContextCountByState = struct {
	Count graphql.Int
	State checks.CheckRunState
}

type Commits struct {
	Nodes []struct {
		Commit struct {
			Deployments struct {
				Nodes []struct {
					Task        graphql.String
					Description graphql.String
				}
			} `graphql:"deployments(last: 10)"`
			CommitUrl         graphql.String
			StatusCheckRollup struct {
				State graphql.String
			}
		}
	}
	TotalCount int
}

type Comment struct {
	Author struct {
		Login string
	}
	Body      string
	UpdatedAt time.Time
}

type ReviewComment struct {
	Author struct {
		Login string
	}
	Body      string
	UpdatedAt time.Time
	StartLine int
	Line      int
}

type ReviewComments struct {
	Nodes      []ReviewComment
	TotalCount int
}

type Comments struct {
	TotalCount int
}

type ReviewThreads struct {
	TotalCount int
}

type Review struct {
	Author struct {
		Login string
	}
	Body      string
	State     string
	UpdatedAt time.Time
}

type Reviews struct {
	TotalCount int
	Nodes      []Review
}

type ReviewThreadsWithComments struct {
	Nodes []struct {
		Id           string
		IsOutdated   bool
		OriginalLine int
		StartLine    int
		Line         int
		Path         string
		Comments     ReviewComments `graphql:"comments(first: 20)"`
	}
}

type ChangedFile struct {
	Additions  int
	Deletions  int
	Path       string
	ChangeType string
}

type ChangedFiles struct {
	TotalCount int
	Nodes      []ChangedFile
}

type RequestedReviewerUser struct {
	Login string `graphql:"login"`
}

type RequestedReviewerTeam struct {
	Slug string `graphql:"slug"`
	Name string `graphql:"name"`
}

type RequestedReviewerBot struct {
	Login string `graphql:"login"`
}

type RequestedReviewerMannequin struct {
	Login string `graphql:"login"`
}

type ReviewRequestNode struct {
	AsCodeOwner       bool `graphql:"asCodeOwner"`
	RequestedReviewer struct {
		User      RequestedReviewerUser      `graphql:"... on User"`
		Team      RequestedReviewerTeam      `graphql:"... on Team"`
		Bot       RequestedReviewerBot       `graphql:"... on Bot"`
		Mannequin RequestedReviewerMannequin `graphql:"... on Mannequin"`
	} `graphql:"requestedReviewer"`
}

type ReviewRequests struct {
	TotalCount int
	Nodes      []ReviewRequestNode
}

func (r ReviewRequestNode) GetReviewerDisplayName() string {
	if r.RequestedReviewer.User.Login != "" {
		return r.RequestedReviewer.User.Login
	}
	if r.RequestedReviewer.Team.Slug != "" {
		return r.RequestedReviewer.Team.Slug
	}
	if r.RequestedReviewer.Bot.Login != "" {
		return r.RequestedReviewer.Bot.Login
	}
	if r.RequestedReviewer.Mannequin.Login != "" {
		return r.RequestedReviewer.Mannequin.Login
	}
	return ""
}

func (r ReviewRequestNode) GetReviewerType() string {
	if r.RequestedReviewer.User.Login != "" {
		return "User"
	}
	if r.RequestedReviewer.Team.Slug != "" {
		return "Team"
	}
	if r.RequestedReviewer.Bot.Login != "" {
		return "Bot"
	}
	if r.RequestedReviewer.Mannequin.Login != "" {
		return "Mannequin"
	}
	return ""
}

func (r ReviewRequestNode) IsTeam() bool {
	return r.RequestedReviewer.Team.Slug != ""
}

type PRLabel struct {
	Color string
	Name  string
}

type PRLabels struct {
	Nodes []Label
}

type MergeStateStatus string

type PageInfo struct {
	HasNextPage bool
	StartCursor string
	EndCursor   string
}

func (data PullRequestData) GetAuthor(theme theme.Theme, showAuthorIcon bool) string {
	author := data.Author.Login
	if showAuthorIcon {
		author += fmt.Sprintf(" %s", GetAuthorRoleIcon(data.AuthorAssociation, theme))
	}
	return author
}

func (data PullRequestData) GetTitle() string {
	return data.Title
}

func (data PullRequestData) GetRepoNameWithOwner() string {
	return data.Repository.NameWithOwner
}

func (data PullRequestData) GetRepoNameAndOwner() (owner, repoName string) {
	return data.Repository.Owner.Login, data.Repository.Name
}

func (data PullRequestData) GetNumber() int {
	return data.Number
}

func (data PullRequestData) GetUrl() string {
	return data.Url
}

func (data PullRequestData) GetUpdatedAt() time.Time {
	return data.UpdatedAt
}

func (data PullRequestData) GetCreatedAt() time.Time {
	return data.CreatedAt
}

// ToPullRequestData converts EnrichedPullRequestData to PullRequestData
// This is useful when we fetch a single PR and need basic PR fields
func (e EnrichedPullRequestData) ToPullRequestData() PullRequestData {
	return PullRequestData{
		Number:            e.Number,
		Title:             e.Title,
		Body:              e.Body,
		Author:            e.Author,
		AuthorAssociation: e.AuthorAssociation,
		UpdatedAt:         e.UpdatedAt,
		CreatedAt:         e.CreatedAt,
		Url:               e.Url,
		State:             e.State,
		Mergeable:         e.Mergeable,
		ReviewDecision:    e.ReviewDecision,
		Additions:         e.Additions,
		Deletions:         e.Deletions,
		HeadRefName:       e.HeadRefName,
		BaseRefName:       e.BaseRefName,
		HeadRepository:    e.HeadRepository,
		HeadRef:           e.HeadRef,
		Repository:        e.Repository,
		Assignees:         e.Assignees,
		IsDraft:           e.IsDraft,
		IsInMergeQueue:    e.IsInMergeQueue,
		Labels:            e.Labels,
		Files:             e.Files,
		// Note: Comments, ReviewThreads, Reviews, ReviewRequests, Commits
		// have different types in EnrichedPullRequestData vs PullRequestData
		// We leave them as zero values since the enriched data will be used instead
	}
}

func makePullRequestsQuery(query string) string {
	return fmt.Sprintf("is:pr archived:false %s sort:updated", query)
}

type PullRequestsResponse struct {
	Prs        []PullRequestData
	TotalCount int
	PageInfo   PageInfo
}

var (
	client       *gh.GraphQLClient
	cachedClient *gh.GraphQLClient
)

func SetClient(c *gh.GraphQLClient) {
	client = c
	cachedClient = c
}

// ClearEnrichmentCache clears the cached GraphQL client used for fetching
// enriched PR/Issue data. Call this when refreshing to ensure fresh data.
func ClearEnrichmentCache() {
	cachedClient = nil
}

// IsEnrichmentCacheCleared returns true if the enrichment cache is cleared.
// This is primarily for testing purposes.
func IsEnrichmentCacheCleared() bool {
	return cachedClient == nil
}

const prsCacheTTL = 30 * time.Minute

// FetchPullRequests fetches PRs matching query. When cache is non-nil and
// pageInfo is nil (first page), a cache hit is returned immediately and a
// successful network response is written back to the cache.
func FetchPullRequests(cache *persistcache.Store, query string, limit int, pageInfo *PageInfo) (PullRequestsResponse, error) {
	var err error
	if client == nil {
		if config.IsFeatureEnabled(config.FF_MOCK_DATA) {
			log.Info("using mock data", "server", "https://localhost:3000")
			http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{
				InsecureSkipVerify: true,
			}
			client, err = gh.NewGraphQLClient(
				gh.ClientOptions{Host: "localhost:3000", AuthToken: "fake-token"},
			)
		} else {
			client, err = gh.DefaultGraphQLClient()
		}
	}

	if err != nil {
		return PullRequestsResponse{}, err
	}

	isFirstPage := pageInfo == nil
	cacheKey := prsCacheKey(query, limit)

	if isFirstPage && cache != nil {
		if raw, hit, cerr := cache.Get(cacheKey); cerr != nil {
			log.Warn("FetchPullRequests: cache read error", "key", cacheKey, "err", cerr)
		} else if hit {
			var cached PullRequestsResponse
			if jsonErr := jsonUnmarshal(raw, &cached); jsonErr == nil {
				log.Debug("FetchPullRequests: cache hit", "query", query, "count", len(cached.Prs))
				return cached, nil
			}
		}
	}

	var queryResult struct {
		Search struct {
			Nodes []struct {
				PullRequest PullRequestData `graphql:"... on PullRequest"`
			}
			IssueCount int
			PageInfo   PageInfo
		} `graphql:"search(type: ISSUE, first: $limit, after: $endCursor, query: $query)"`
	}
	var endCursor *string
	if pageInfo != nil {
		endCursor = &pageInfo.EndCursor
	}
	variables := map[string]any{
		"query":     graphql.String(makePullRequestsQuery(query)),
		"limit":     graphql.Int(limit),
		"endCursor": (*graphql.String)(endCursor),
	}
	log.Debug("Fetching PRs", "query", query, "limit", limit, "endCursor", endCursor)
	err = client.Query("SearchPullRequests", &queryResult, variables)
	if err != nil {
		// On API error (e.g. rate limit), fall back to stale cache if available.
		if isFirstPage && cache != nil {
			if raw, hit, _ := cache.GetStale(cacheKey); hit {
				var stale PullRequestsResponse
				if jsonErr := jsonUnmarshal(raw, &stale); jsonErr == nil {
					log.Warn("FetchPullRequests: API error, serving stale cache", "err", err)
					return stale, nil
				}
			}
		}
		return PullRequestsResponse{}, err
	}
	log.Info("Successfully fetched PRs", "count", queryResult.Search.IssueCount)

	prs := make([]PullRequestData, 0, len(queryResult.Search.Nodes))
	for _, node := range queryResult.Search.Nodes {
		prs = append(prs, node.PullRequest)
	}

	resp := PullRequestsResponse{
		Prs:        prs,
		TotalCount: queryResult.Search.IssueCount,
		PageInfo:   queryResult.Search.PageInfo,
	}

	if isFirstPage && cache != nil {
		if raw, merr := jsonMarshal(resp); merr == nil {
			if werr := cache.Put(cacheKey, raw, prsCacheTTL); werr != nil {
				log.Warn("FetchPullRequests: cache write error", "key", cacheKey, "err", werr)
			}
		}
	}

	return resp, nil
}

// InvalidatePRsCache removes all cached PR responses so the next fetch is fresh.
func InvalidatePRsCache(cache *persistcache.Store) error {
	if cache == nil {
		return nil
	}
	return cache.Invalidate("prs/")
}

// prsCacheKey returns a filesystem-safe cache key for a PR query+limit pair.
func prsCacheKey(query string, limit int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", query, limit)))
	return fmt.Sprintf("prs/%x", h)
}

func FetchPullRequest(prUrl string) (EnrichedPullRequestData, error) {
	var err error
	if client == nil {
		client, err = gh.DefaultGraphQLClient()
		if err != nil {
			return EnrichedPullRequestData{}, err
		}
	}

	var queryResult struct {
		Resource struct {
			PullRequest EnrichedPullRequestData `graphql:"... on PullRequest"`
		} `graphql:"resource(url: $url)"`
	}
	parsedUrl, err := url.Parse(prUrl)
	if err != nil {
		return EnrichedPullRequestData{}, err
	}
	variables := map[string]any{
		"url": githubv4.URI{URL: parsedUrl},
	}
	log.Debug("Fetching PR", "url", prUrl)
	err = client.Query("FetchPullRequest", &queryResult, variables)
	if err != nil {
		return EnrichedPullRequestData{}, err
	}
	log.Info("Successfully fetched PR", "url", prUrl)

	return queryResult.Resource.PullRequest, nil
}

// QueuedMode is a client-side filter mode for narrowing PR results by their
// merge-queue membership. GitHub's search syntax does not expose merge-queue
// status, so this filter is applied after the GraphQL fetch.
type QueuedMode int

const (
	// QueuedAny means no merge-queue filter token was present; pass all PRs.
	QueuedAny QueuedMode = iota
	// QueuedOnly means `is:queued` was present; keep only PRs in a merge queue.
	QueuedOnly
	// QueuedExcluded means `-is:queued` was present; drop PRs in a merge queue.
	QueuedExcluded
)

// ExtractQueuedFilter peels `is:queued` and `-is:queued` tokens out of a
// GitHub search filter string, returning the cleaned filter and the resulting
// QueuedMode. When both tokens appear, the last one wins (mirrors how GitHub
// search treats duplicate `is:` predicates).
func ExtractQueuedFilter(filter string) (string, QueuedMode) {
	mode := QueuedAny
	remaining := make([]string, 0)
	for _, token := range strings.Fields(filter) {
		switch token {
		case "is:queued":
			mode = QueuedOnly
		case "-is:queued":
			mode = QueuedExcluded
		default:
			remaining = append(remaining, token)
		}
	}
	return strings.Join(remaining, " "), mode
}
