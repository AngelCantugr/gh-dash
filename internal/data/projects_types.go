package data

import "time"

// OwnerKind identifies whether an owner is an organization or an individual user.
type OwnerKind int

const (
	OwnerOrg  OwnerKind = iota // organization
	OwnerUser                  // user account
)

// OwnerRef identifies the owner whose projects should be queried.
type OwnerRef struct {
	Kind  OwnerKind
	Login string
}

// ProjectFilters are applied client-side after fetching project data.
// A nil Closed pointer means "any state" (both open and closed).
type ProjectFilters struct {
	Closed        *bool  // nil = any; true = closed only; false = open only
	TitleContains string // case-insensitive substring; empty = no filter
}

// ProjectData holds the data for a single GitHub ProjectsV2 project.
type ProjectData struct {
	ID, Number, Title, URL string
	Owner                  OwnerRef
	Closed, Public         bool
	ItemsCount             int
	OpenItemsCountLoaded   int // approximation; not fetched from the API
	UpdatedAt              time.Time
}
