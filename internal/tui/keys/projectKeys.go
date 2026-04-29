package keys

import (
	"fmt"

	"charm.land/bubbles/v2/key"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
)

// ProjectKeyMap holds keybindings for the projects list view.
// The full set of project-specific actions will be finalized in PR #9 (drill-down).
type ProjectKeyMap struct {
	Drill   key.Binding // enter — reserved for drill-down (PR #9)
	Refresh key.Binding // r     — refresh the projects list
	OpenWeb key.Binding // o     — open the selected project in the browser
}

var ProjectKeys = ProjectKeyMap{
	Drill: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "open project"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	OpenWeb: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "open in browser"),
	),
}

func ProjectFullHelp() []key.Binding {
	return []key.Binding{
		ProjectKeys.Drill,
		ProjectKeys.Refresh,
		ProjectKeys.OpenWeb,
	}
}

func rebindProjectKeys(keys []config.Keybinding) error {
	CustomProjectsBindings = []key.Binding{}

	for _, projectKey := range keys {
		if projectKey.Builtin == "" {
			// Handle custom commands
			if projectKey.Command != "" {
				name := projectKey.Name
				if projectKey.Name == "" {
					name = config.TruncateCommand(projectKey.Command)
				}

				customBinding := key.NewBinding(
					key.WithKeys(projectKey.Key),
					key.WithHelp(projectKey.Key, name),
				)

				CustomProjectsBindings = append(CustomProjectsBindings, customBinding)
			}
			continue
		}

		var binding *key.Binding

		switch projectKey.Builtin {
		case "drill":
			binding = &ProjectKeys.Drill
		case "refresh":
			binding = &ProjectKeys.Refresh
		case "openWeb":
			binding = &ProjectKeys.OpenWeb
		default:
			return fmt.Errorf("unknown built-in project key: '%s'", projectKey.Builtin)
		}

		binding.SetKeys(projectKey.Key)
		helpDesc := binding.Help().Desc
		if projectKey.Name != "" {
			helpDesc = projectKey.Name
		}
		binding.SetHelp(projectKey.Key, helpDesc)
	}

	return nil
}
