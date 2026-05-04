package constants

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// StaggeredCmdMsg carries a deferred tea.Cmd that should be executed when
// this message is received by the root model. Used to spread section fetches
// over time and avoid GitHub's secondary (abuse) rate limiter.
type StaggeredCmdMsg struct{ Cmd tea.Cmd }

// StaggerCmds wraps each cmd beyond the first with an increasing delay so
// sections don't all hit the GitHub API simultaneously.
func StaggerCmds(cmds []tea.Cmd, delay time.Duration) tea.Cmd {
	batched := make([]tea.Cmd, 0, len(cmds))
	for i, cmd := range cmds {
		if i == 0 {
			batched = append(batched, cmd)
			continue
		}
		d := time.Duration(i) * delay
		c := cmd // capture loop variable
		batched = append(batched, tea.Tick(d, func(time.Time) tea.Msg {
			return StaggeredCmdMsg{Cmd: c}
		}))
	}
	return tea.Batch(batched...)
}
