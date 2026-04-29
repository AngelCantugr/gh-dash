package projectsection

import (
	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/table"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/utils"
)

// GetSectionColumns returns the column definitions for the projects list.
//
// Columns (matching §4.1 of the spec):
//   - State icon        (3)
//   - Title             (grow)
//   - Owner             (20)
//   - Status            (6)
//   - Items             (6)
//   - Updated           (10)
func GetSectionColumns(
	_ config.ProjectsSectionConfig,
	_ *context.ProgramContext,
) []table.Column {
	return []table.Column{
		{
			Title: "",
			Width: utils.IntPtr(3),
		},
		{
			Title: "Title",
			Grow:  utils.BoolPtr(true),
		},
		{
			Title: "Owner",
			Width: utils.IntPtr(20),
		},
		{
			Title: "Status",
			Width: utils.IntPtr(6),
		},
		{
			Title: "Items",
			Width: utils.IntPtr(6),
		},
		{
			Title: "Updated",
			Width: utils.IntPtr(10),
		},
	}
}
