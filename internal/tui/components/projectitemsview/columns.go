package projectitemsview

import (
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

	return table.Row{title, typeStr, repoStr, statusStr, updatedStr}
}

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
