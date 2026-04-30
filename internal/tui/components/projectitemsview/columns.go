package projectitemsview

import (
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/table"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/utils"
)

var (
	colWidthType    = 12
	colWidthRepo    = 20
	colWidthStatus  = 16
	colWidthUpdated = 12

	colWidthExtraMin = 8

	growTrue = true
)

// GetColumns returns the base column definitions for the items table.
func GetColumns() []table.Column {
	return []table.Column{
		{Title: "Title", Grow: &growTrue},
		{Title: "Type", Width: &colWidthType},
		{Title: "Repo", Width: &colWidthRepo},
		{Title: "Status", Width: &colWidthStatus},
		{Title: "Updated", Width: &colWidthUpdated},
	}
}

// BuildColumns returns the full column set for the drill-down table.
// Base columns are always present in fixed order; extras follow in
// YAML-declared order. Widths for extra columns are computed from observed values.
func BuildColumns(schema *data.ProjectSchema, observed []data.ProjectItemData) []table.Column {
	cols := GetColumns()
	if schema == nil {
		return cols
	}
	for _, fieldID := range schema.ExtraFieldOrder {
		def, ok := schema.ExtraFields[fieldID]
		if !ok {
			continue
		}
		w := computeExtraWidth(fieldID, def, observed)
		wCopy := w
		cols = append(cols, table.Column{
			Title: def.Name,
			Width: &wCopy,
		})
	}
	return cols
}

// computeExtraWidth returns the column width for an extra field: the maximum
// of the header title width, the widest rendered value among observed items,
// and the minimum extra column width.
func computeExtraWidth(fieldID string, def data.FieldDef, observed []data.ProjectItemData) int {
	w := utf8.RuneCountInString(def.Name)
	if w < colWidthExtraMin {
		w = colWidthExtraMin
	}
	for _, item := range observed {
		cell := renderExtraField(fieldID, item, nil)
		if cw := utf8.RuneCountInString(cell); cw > w {
			w = cw
		}
	}
	return w
}

// BuildRows converts ProjectItemData slice to table.Row slice for rendering.
func BuildRows(ctx *context.ProgramContext, items []data.ProjectItemData, schema *data.ProjectSchema) []table.Row {
	rows := make([]table.Row, 0, len(items))
	for _, item := range items {
		rows = append(rows, buildRow(ctx, item, schema))
	}
	return rows
}

func buildRow(ctx *context.ProgramContext, item data.ProjectItemData, schema *data.ProjectSchema) table.Row {
	textStyle := ctx.Styles.Common.MainTextStyle

	title := textStyle.Render(item.Title)
	if item.Title == "" {
		title = textStyle.Foreground(ctx.Theme.FaintText).Render("(untitled)")
	}

	typeStr := renderItemType(item.Type)
	repoStr := textStyle.Foreground(ctx.Theme.FaintText).Render(item.Repo)
	if item.Repo == "" {
		repoStr = textStyle.Foreground(ctx.Theme.FaintText).Render("—")
	}
	statusStr := renderStatus(item, schema)
	updatedStr := renderUpdated(ctx, item)

	row := table.Row{title, typeStr, repoStr, statusStr, updatedStr}

	if schema != nil {
		for _, fieldID := range schema.ExtraFieldOrder {
			if _, ok := schema.ExtraFields[fieldID]; !ok {
				row = append(row, "—")
				continue
			}
			row = append(row, renderExtraField(fieldID, item, schema))
		}
	}

	return row
}

// renderExtraField renders the value of an extra field for a given item.
// Returns "—" for missing values, schema-drift, or unsupported types.
func renderExtraField(fieldID string, item data.ProjectItemData, schema *data.ProjectSchema) string {
	// Schema-drift guard: if schema is provided and field ID is not in it, render "—".
	if schema != nil {
		if _, ok := schema.ExtraFields[fieldID]; !ok {
			return "—"
		}
	}

	fv, ok := item.Fields[fieldID]
	if !ok {
		return "—"
	}

	switch v := fv.(type) {
	case data.FieldValueSingleSelect:
		return v.Name
	case data.FieldValueNumber:
		return formatNumber(v.Number)
	case data.FieldValueDate:
		return renderDateValue(v.Date)
	case data.FieldValueIteration:
		return renderIterationValue(v)
	case data.FieldValueText:
		return v.Text
	default:
		return "—"
	}
}

// formatNumber renders a float64 without a trailing ".0" when the value is
// an integer, e.g. 42 → "42", 3.14 → "3.14".
func formatNumber(n float64) string {
	if n == float64(int64(n)) {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// renderDateValue parses an ISO-8601 date string and returns "YYYY-MM-DD".
// Falls back to the raw string if it cannot be parsed.
func renderDateValue(raw string) string {
	// GitHub returns dates as "YYYY-MM-DD" or "YYYY-MM-DDTHH:MM:SSZ".
	for _, layout := range []string{"2006-01-02T15:04:05Z", "2006-01-02"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return raw
}

// renderIterationValue formats an iteration as "Title (MMM D → MMM D)".
// StartDate is "YYYY-MM-DD"; Duration is in days.
func renderIterationValue(v data.FieldValueIteration) string {
	start, err := time.Parse("2006-01-02", v.StartDate)
	if err != nil {
		// Fallback: just the title.
		return v.Title
	}
	end := start.AddDate(0, 0, v.Duration-1)
	startFmt := fmt.Sprintf("%s %d", start.Format("Jan"), start.Day())
	endFmt := fmt.Sprintf("%s %d", end.Format("Jan"), end.Day())
	return fmt.Sprintf("%s (%s → %s)", v.Title, startFmt, endFmt)
}

// renderItemType converts an ItemType to a display string.
func renderItemType(t data.ItemType) string {
	switch t {
	case data.ItemTypeIssue:
		return "issue"
	case data.ItemTypePullRequest:
		return "pr"
	case data.ItemTypeDraftIssue:
		return "draft"
	case data.ItemTypeRedacted:
		return "(redacted)"
	default:
		return "—"
	}
}

func renderStatus(item data.ProjectItemData, schema *data.ProjectSchema) string {
	if schema == nil || schema.StatusField == nil {
		return "—"
	}

	fv, ok := item.Fields[schema.StatusField.ID]
	if !ok {
		return "—"
	}

	ss, ok := fv.(data.FieldValueSingleSelect)
	if !ok {
		return "—"
	}

	return ss.Name
}

func renderUpdated(ctx *context.ProgramContext, item data.ProjectItemData) string {
	if item.UpdatedAt.IsZero() {
		return "—"
	}

	timeFormat := ctx.Config.Defaults.DateFormat
	if timeFormat == "" || timeFormat == "relative" {
		return utils.TimeElapsed(item.UpdatedAt)
	}
	return item.UpdatedAt.Format(timeFormat)
}
