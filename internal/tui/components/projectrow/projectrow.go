package projectrow

import (
	"fmt"
	"strconv"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/table"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/utils"
)

// Data wraps *data.ProjectData and implements data.RowData so that
// projectrow.Data can be used in sections that expect a typed row.
type Data struct {
	Primary *data.ProjectData
}

func (d Data) GetTitle() string {
	if d.Primary == nil {
		return ""
	}
	return d.Primary.Title
}

func (d Data) GetRepoNameWithOwner() string {
	if d.Primary == nil {
		return ""
	}
	owner := d.Primary.Owner
	if owner.Login == "" {
		return "viewer"
	}
	return string(owner.Kind) + ":" + owner.Login
}

// GetNumber converts the string project number to int.
// Returns 0 if the value is empty or non-numeric.
func (d Data) GetNumber() int {
	if d.Primary == nil {
		return 0
	}
	n, err := strconv.Atoi(d.Primary.Number)
	if err != nil {
		return 0
	}
	return n
}

func (d Data) GetUrl() string {
	if d.Primary == nil {
		return ""
	}
	return d.Primary.URL
}

func (d Data) GetUpdatedAt() time.Time {
	if d.Primary == nil {
		return time.Time{}
	}
	return d.Primary.UpdatedAt
}

// Project is the row renderer for the projects list.
type Project struct {
	Ctx     *context.ProgramContext
	Data    *Data
	Columns []table.Column
}

func (p *Project) getTextStyle() lipgloss.Style {
	return components.GetIssueTextStyle(p.Ctx)
}

func (p *Project) renderState() string {
	if p.Data == nil || p.Data.Primary == nil {
		return p.getTextStyle().Render("-")
	}
	if p.Data.Primary.Closed {
		return lipgloss.NewStyle().
			Foreground(p.Ctx.Styles.Colors.ClosedPR).
			Render("")
	}
	return lipgloss.NewStyle().
		Foreground(p.Ctx.Styles.Colors.OpenPR).
		Render("")
}

func (p *Project) renderTitle() string {
	if p.Data == nil || p.Data.Primary == nil {
		return p.getTextStyle().Render("-")
	}
	return components.RenderIssueTitle(
		p.Ctx,
		statusFromClosed(p.Data.Primary.Closed),
		p.Data.Primary.Title,
		0,
	)
}

func (p *Project) renderOwner() string {
	if p.Data == nil || p.Data.Primary == nil {
		return p.getTextStyle().Render("-")
	}
	owner := p.Data.Primary.Owner
	if owner.Login == "" {
		return p.getTextStyle().Render("viewer")
	}
	return p.getTextStyle().Render(fmt.Sprintf("%s:%s", owner.Kind, owner.Login))
}

func (p *Project) renderStatus() string {
	if p.Data == nil || p.Data.Primary == nil {
		return p.getTextStyle().Render("-")
	}
	if p.Data.Primary.Closed {
		return p.getTextStyle().Render("closed")
	}
	return p.getTextStyle().Render("open")
}

func (p *Project) renderItemsCount() string {
	if p.Data == nil || p.Data.Primary == nil {
		return p.getTextStyle().Render("-")
	}
	return p.getTextStyle().Render(fmt.Sprintf("%d", p.Data.Primary.ItemsCount))
}

func (p *Project) renderUpdatedAt() string {
	if p.Data == nil || p.Data.Primary == nil {
		return p.getTextStyle().Render("-")
	}
	timeFormat := p.Ctx.Config.Defaults.DateFormat
	var out string
	if timeFormat == "" || timeFormat == "relative" {
		out = utils.TimeElapsed(p.Data.Primary.UpdatedAt)
	} else {
		out = p.Data.Primary.UpdatedAt.Format(timeFormat)
	}
	return p.getTextStyle().Foreground(p.Ctx.Theme.FaintText).Render(out)
}

func (p *Project) ToTableRow(_ bool) table.Row {
	return table.Row{
		p.renderState(),
		p.renderTitle(),
		p.renderOwner(),
		p.renderStatus(),
		p.renderItemsCount(),
		p.renderUpdatedAt(),
	}
}

// statusFromClosed converts a bool into the string expected by
// components.RenderIssueTitle ("OPEN" / "CLOSED").
func statusFromClosed(closed bool) string {
	if closed {
		return "CLOSED"
	}
	return "OPEN"
}
