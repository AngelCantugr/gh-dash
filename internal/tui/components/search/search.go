package search

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/cmpcontroller"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/fuzzyselect"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/inputbox"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
)

type Model struct {
	ctx          *context.ProgramContext
	initialValue string
	cmpctl       *cmpcontroller.Controller
}

type SearchOptions struct {
	Prefix       string
	InitialValue string
	Placeholder  string
}

func NewModel(ctx *context.ProgramContext, opts SearchOptions) Model {
	ti := textinput.New()
	ti.Placeholder = opts.Placeholder
	base := lipgloss.NewStyle()
	ti.SetStyles(textinput.Styles{
		Focused: textinput.StyleState{
			Placeholder: lipgloss.NewStyle().Foreground(ctx.Theme.FaintText),
			Prompt:      base.Foreground(ctx.Theme.SecondaryText),
			Text:        base.Foreground(ctx.Theme.PrimaryText),
		},
		Blurred: textinput.StyleState{
			Placeholder: lipgloss.NewStyle().Foreground(ctx.Theme.FaintText),
			Prompt:      base.Foreground(ctx.Theme.SecondaryText),
			Text:        lipgloss.NewStyle().Foreground(ctx.Theme.PrimaryText),
		},
		Cursor: textinput.CursorStyle{
			Color: ctx.Theme.FaintText,
			Shape: tea.CursorBar,
			Blink: true,
		},
	})
	ti.Prompt = fmt.Sprintf(" %s ", opts.Prefix)

	ti.Blur()
	ti.SetValue(opts.InitialValue)
	ti.CursorStart()

	ctl := cmpcontroller.New(
		ctx,
		inputbox.ModelOpts{TextInput: &ti},
	)
	selectStyles := ctx.Styles.Select
	selectStyles.PopupStyle = ctx.Styles.Select.PopupStyle.BorderTop(false).BorderForeground(
		ctx.Styles.Colors.OpenIssue,
	)
	ctl.SetSelectStyles(selectStyles)

	m := Model{
		ctx:          ctx,
		initialValue: opts.InitialValue,
		cmpctl:       &ctl,
	}

	m.cmpctl.Exit()

	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	cmd, _ := m.cmpctl.Update(msg)
	return m, cmd
}

func (m Model) View(ctx *context.ProgramContext) string {
	s := m.ctx.Styles.Search.Root
	if cmp := m.ViewCompletions(); cmp != "" {
		b := lipgloss.RoundedBorder()
		b.BottomLeft = lipgloss.RoundedBorder().MiddleLeft
		b.BottomRight = lipgloss.RoundedBorder().MiddleRight
		s = s.Border(b, true)
	}
	if m.cmpctl.Focused() {
		s = s.BorderForeground(m.ctx.Styles.Colors.OpenIssue)
	}
	return s.Render(m.cmpctl.View())
}

func (m Model) ViewCompletions() string {
	return m.cmpctl.ViewCompletions()
}

func (m *Model) CursorEnd() {
	m.cmpctl.CursorEnd()
}

func (m *Model) Repo() (cmpcontroller.RepoRef, bool) {
	for token := range strings.FieldsSeq(m.Value()) {
		if strings.HasPrefix(token, "repo:") {
			repo, _ := strings.CutPrefix(token, "repo:")
			parts := strings.Split(repo, "/")
			// A half-typed qualifier ("repo:dlvhdr/") is not a repo. Reporting it
			// as one makes Focus start a fetch that the loader then skips, so the
			// user watches a spinner for a lookup that never runs.
			if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
				return cmpcontroller.RepoRef{}, false
			}
			// Build the name from the parts actually used as owner/name rather
			// than the raw token: the fetchers cache under "owner/name", and a
			// qualifier with extra segments would otherwise yield a cache key
			// that never matches, silently turning the refresh key into a no-op.
			return cmpcontroller.RepoRef{
				NameWithOwner: parts[0] + "/" + parts[1],
				Owner:         parts[0],
				Name:          parts[1],
			}, true
		}
	}
	return cmpcontroller.RepoRef{}, false
}

func (m *Model) Focus() tea.Cmd {
	// Users and labels can only be fetched for a single repository. Sections that
	// span repositories — notifications, or filters without a repo: qualifier —
	// skip the fetch entirely rather than issuing a lookup that can only fail.
	repo, scopedToRepo := m.Repo()
	enterFetch := cmpcontroller.FetchWithLoading
	if !scopedToRepo {
		enterFetch = cmpcontroller.FetchNone
	}

	m.cmpctl.SetAutocompleteSource(&fuzzyselect.SearchQuerySource{})
	cmd := m.cmpctl.Enter(cmpcontroller.EnterOptions{
		Mode:                             cmpcontroller.ModeSearch,
		Prompt:                           "",
		Repo:                             repo,
		EnterFetch:                       enterFetch,
		ConfirmDiscardOnCancel:           false,
		HideAutocompleteWhenContextEmpty: false,
		InitialValue:                     m.cmpctl.Value(),
	})

	// Enter deliberately leaves the popup empty, and only a completed fetch
	// filters it afterwards. Without a fetch to wait for, filling it in here is
	// what stops an unscoped search from opening on "no results" while the
	// source still has repo-independent suggestions to offer, such as @me.
	if !scopedToRepo {
		m.cmpctl.Filter()
	}

	m.cmpctl.ShowCompletions()
	return cmd
}

func (m *Model) Blur() {
	m.cmpctl.Exit()
}

func (m *Model) SetValue(val string) {
	m.cmpctl.SetValue(val)
}

func (m *Model) UpdateProgramContext(ctx *context.ProgramContext) {
	oldWidth := m.cmpctl.Width()
	newWidth := m.getInputWidth(ctx)
	m.cmpctl.SetWidth(newWidth)
	if newWidth != oldWidth {
		m.cmpctl.CursorEnd()
	}
}

func (m *Model) getInputWidth(ctx *context.ProgramContext) int {
	return max(
		2,
		ctx.MainContentWidth-4,
	)
}

func (m Model) Value() string {
	return m.cmpctl.Value()
}
