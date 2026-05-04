package data

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
)

// OwnerKind identifies whether an owner is an organization or an individual user.
// Values match config.OwnerKind ("org" / "user") so callers can convert without loss.
type OwnerKind = config.OwnerKind

const (
	OwnerKindOrg  = config.OwnerKindOrg  // "org"
	OwnerKindUser = config.OwnerKindUser // "user"
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

// --------------------------------------------------------------------------
// Project items types
// --------------------------------------------------------------------------

// ItemType identifies the kind of a project item.
// ItemTypeRedacted is for items the viewer cannot see (private repo, etc.).
// Keep redacted items in the list for accurate ItemsCount matching but render distinctly.
type ItemType int

const (
	ItemTypeIssue ItemType = iota
	ItemTypePullRequest
	ItemTypeDraftIssue
	ItemTypeRedacted
)

// StatusOption is one choice in a single-select Status field.
type StatusOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// StatusFieldDef describes the built-in Status single-select field.
type StatusFieldDef struct {
	ID      string         `json:"id"`
	Options []StatusOption `json:"options"`
}

// FieldDef describes a non-Status project field resolved from its YAML name.
type FieldDef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProjectSchema holds the field layout for one project. It is built from the
// GraphQL fields connection at schema fetch time and cached with the items.
type ProjectSchema struct {
	// StatusField is the built-in single-select used for mutation.
	// nil if the project has no field named "Status".
	StatusField *StatusFieldDef `json:"statusField,omitempty"`

	// ExtraFields are resolved by YAML name → field ID once at schema fetch
	// time. Keyed by field ID — same name across projects may map to different
	// field IDs with different types. Duplicates within one project log a warn
	// and first match wins.
	ExtraFields map[string]FieldDef `json:"extraFields,omitempty"`

	// ExtraFieldOrder preserves the YAML-declared order for column rendering.
	ExtraFieldOrder []string `json:"extraFieldOrder,omitempty"`
}

// --------------------------------------------------------------------------
// FieldValue sealed interface
// --------------------------------------------------------------------------

// FieldValue is a sealed interface for the different project field value types
// returned by ProjectV2ItemFieldValue.
// Concrete types: FieldValueSingleSelect, FieldValueNumber, FieldValueDate,
// FieldValueIteration, FieldValueText, FieldValueIssue, FieldValueUnknown.
type FieldValue interface {
	isFieldValue()
	fieldValueKind() string
}

// FieldValueSingleSelect represents a single-select field value.
type FieldValueSingleSelect struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
}

func (FieldValueSingleSelect) isFieldValue()          {}
func (FieldValueSingleSelect) fieldValueKind() string { return "single_select" }

// FieldValueNumber represents a numeric field value.
type FieldValueNumber struct {
	Number float64 `json:"number"`
}

func (FieldValueNumber) isFieldValue()          {}
func (FieldValueNumber) fieldValueKind() string { return "number" }

// FieldValueDate represents a date field value (ISO 8601 string).
type FieldValueDate struct {
	Date string `json:"date"`
}

func (FieldValueDate) isFieldValue()          {}
func (FieldValueDate) fieldValueKind() string { return "date" }

// FieldValueIteration represents an iteration field value.
type FieldValueIteration struct {
	IterationID string `json:"iterationId"`
	Title       string `json:"title"`
	StartDate   string `json:"startDate"`
	Duration    int    `json:"duration"`
}

func (FieldValueIteration) isFieldValue()          {}
func (FieldValueIteration) fieldValueKind() string { return "iteration" }

// FieldValueText represents a text field value.
type FieldValueText struct {
	Text string `json:"text"`
}

func (FieldValueText) isFieldValue()          {}
func (FieldValueText) fieldValueKind() string { return "text" }

// FieldValueIssue represents a field value that references an issue.
type FieldValueIssue struct {
	Title  string `json:"title"`
	Number int    `json:"number"`
	URL    string `json:"url"`
}

func (FieldValueIssue) isFieldValue()          {}
func (FieldValueIssue) fieldValueKind() string { return "issue" }

// FieldValueUnknown is the default for unrecognised field value types.
type FieldValueUnknown struct{}

func (FieldValueUnknown) isFieldValue()          {}
func (FieldValueUnknown) fieldValueKind() string { return "unknown" }

// --------------------------------------------------------------------------
// FieldValues — map[fieldID]FieldValue with custom JSON marshaling
// --------------------------------------------------------------------------

// fieldValueEnvelope is the wire format for a serialised FieldValue.
type fieldValueEnvelope struct {
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
}

// FieldValues is map[fieldID]FieldValue with custom JSON marshaling so the
// sealed interface can be round-tripped through disk cache and fixtures.
type FieldValues map[string]FieldValue

// MarshalJSON implements json.Marshaler for FieldValues.
func (fv FieldValues) MarshalJSON() ([]byte, error) {
	m := make(map[string]fieldValueEnvelope, len(fv))
	for k, v := range fv {
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("FieldValues marshal key %q: %w", k, err)
		}
		m[k] = fieldValueEnvelope{Kind: v.fieldValueKind(), Value: json.RawMessage(data)}
	}
	return json.Marshal(m)
}

// UnmarshalJSON implements json.Unmarshaler for FieldValues.
func (fv *FieldValues) UnmarshalJSON(data []byte) error {
	var m map[string]fieldValueEnvelope
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	result := make(FieldValues, len(m))
	for k, env := range m {
		v, err := unmarshalFieldValue(env)
		if err != nil {
			return fmt.Errorf("FieldValues unmarshal key %q: %w", k, err)
		}
		result[k] = v
	}
	*fv = result
	return nil
}

func unmarshalFieldValue(env fieldValueEnvelope) (FieldValue, error) {
	switch env.Kind {
	case "single_select":
		var v FieldValueSingleSelect
		return v, json.Unmarshal(env.Value, &v)
	case "number":
		var v FieldValueNumber
		return v, json.Unmarshal(env.Value, &v)
	case "date":
		var v FieldValueDate
		return v, json.Unmarshal(env.Value, &v)
	case "iteration":
		var v FieldValueIteration
		return v, json.Unmarshal(env.Value, &v)
	case "text":
		var v FieldValueText
		return v, json.Unmarshal(env.Value, &v)
	case "issue":
		var v FieldValueIssue
		return v, json.Unmarshal(env.Value, &v)
	default:
		return FieldValueUnknown{}, nil
	}
}

// --------------------------------------------------------------------------
// ProjectItemData
// --------------------------------------------------------------------------

// ProjectItemData holds the data for a single project item.
type ProjectItemData struct {
	ID           string      `json:"id"`
	Type         ItemType    `json:"type"`
	Title        string      `json:"title"`
	Repo         string      `json:"repo"`
	URL          string      `json:"url"`
	Number       int         `json:"number,omitempty"`
	ParentNumber int         `json:"parentNumber,omitempty"`
	Depth        int         `json:"depth,omitempty"`
	Fields       FieldValues `json:"fields,omitempty"`
	UpdatedAt    time.Time   `json:"updatedAt"`
}

// RowData interface implementation so ProjectItemData can flow through the
// standard sidebar routing in ui.go without a wrapper type.
func (d *ProjectItemData) GetTitle() string {
	if d == nil {
		return ""
	}
	return d.Title
}

func (d *ProjectItemData) GetRepoNameWithOwner() string {
	if d == nil {
		return ""
	}
	return d.Repo
}

// GetNumber returns 0 — project items have no PR/issue number.
func (d *ProjectItemData) GetNumber() int { return 0 }

func (d *ProjectItemData) GetUrl() string {
	if d == nil {
		return ""
	}
	return d.URL
}

func (d *ProjectItemData) GetUpdatedAt() time.Time {
	if d == nil {
		return time.Time{}
	}
	return d.UpdatedAt
}

// --------------------------------------------------------------------------
// projectItemsCache — single-file on-disk format
// --------------------------------------------------------------------------

// projectItemsCache is the JSON envelope stored at project-items/<projectID>.json.
// It accumulates items across "load more" pages: each subsequent fetch reads the
// file, appends new items, and rewrites the whole file atomically.
type projectItemsCache struct {
	Schema   ProjectSchema     `json:"schema"`
	Items    []ProjectItemData `json:"items"`
	PageInfo PageInfo          `json:"pageInfo"`
}
