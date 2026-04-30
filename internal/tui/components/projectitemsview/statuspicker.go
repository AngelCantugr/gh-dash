package projectitemsview

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/cmp"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
)

// statuspicker is a selection-mode wrapper over cmp.Model.
// Unlike cmpcontroller (which drives text input + filter), statuspicker:
//   - Displays a static list of Status options
//   - Arrow keys navigate; Enter confirms; Esc cancels
//   - No text input
type statuspicker struct {
	cmpModel cmp.Model
	options  []data.StatusOption
	open     bool
	onPick   func(optionID string) tea.Cmd
	onCancel func() tea.Cmd
}

// newStatusPicker creates a new statuspicker that calls onPick when the user
// confirms a selection and onCancel when the user presses Esc.
func newStatusPicker(
	ctx *context.ProgramContext,
	onPick func(optionID string) tea.Cmd,
	onCancel func() tea.Cmd,
) statuspicker {
	m := cmp.NewModel(ctx)
	m.SetWidth(32)
	return statuspicker{
		cmpModel: m,
		onPick:   onPick,
		onCancel: onCancel,
	}
}

// Open populates the picker with the given options and shows the popup.
// Calling with an empty options slice is a no-op (picker stays closed).
func (s *statuspicker) Open(options []data.StatusOption) {
	if len(options) == 0 {
		return
	}
	s.options = options
	suggestions := make([]cmp.Suggestion, len(options))
	for i, opt := range options {
		suggestions[i] = cmp.Suggestion{Value: opt.Name, Detail: opt.Color}
	}
	s.cmpModel.SetSuggestions(suggestions)
	// Show("", nil) — empty string returns all suggestions unfiltered.
	s.cmpModel.Show("", nil)
	s.open = true
}

// Close hides the picker and clears its state.
func (s *statuspicker) Close() {
	s.cmpModel.Hide()
	s.cmpModel.Reset()
	s.open = false
}

// IsOpen reports whether the picker popup is currently visible.
func (s *statuspicker) IsOpen() bool {
	return s.open
}

// SelectedOptionID returns the option ID of the currently highlighted option,
// or "" if no options are available.
func (s *statuspicker) SelectedOptionID() string {
	name := s.cmpModel.Selected()
	for _, opt := range s.options {
		if opt.Name == name {
			return opt.ID
		}
	}
	return ""
}

// Update handles keyboard navigation for the picker.
// Returns the updated picker and any commands (e.g. from onPick/onCancel).
func (s statuspicker) Update(msg tea.Msg) (statuspicker, tea.Cmd) {
	if !s.open {
		return s, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	switch {
	case key.Matches(keyMsg, cmp.NextKey):
		s.cmpModel.Next()
		return s, nil

	case key.Matches(keyMsg, cmp.PrevKey):
		s.cmpModel.Prev()
		return s, nil

	case key.Matches(keyMsg, cmp.SelectKey):
		optID := s.SelectedOptionID()
		s.Close()
		if s.onPick != nil && optID != "" {
			return s, s.onPick(optID)
		}
		return s, nil

	case keyMsg.String() == "esc":
		s.Close()
		if s.onCancel != nil {
			return s, s.onCancel()
		}
		return s, nil
	}

	return s, nil
}

// View renders the popup. Returns "" when the picker is closed.
func (s *statuspicker) View() string {
	if !s.open {
		return ""
	}
	return s.cmpModel.View()
}
