package keys

import (
	"fmt"

	"charm.land/bubbles/v2/key"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
)

// ProjectKeyMap holds keybindings for the projects list view.
type ProjectKeyMap struct {
	Drill      key.Binding // enter — drill into project items
	Refresh    key.Binding // r     — refresh the projects list
	OpenWeb    key.Binding // o     — open the selected project in the browser
	LoadMore   key.Binding // >     — load more items (inside items view)
	Back       key.Binding // esc/b — return to projects list (inside items view)
	EditStatus key.Binding // S     — open status picker for the selected item
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
	LoadMore: key.NewBinding(
		key.WithKeys(">"),
		key.WithHelp(">", "load more"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc", "b"),
		key.WithHelp("esc/b", "back"),
	),
	EditStatus: key.NewBinding(
		key.WithKeys("S"),
		key.WithHelp("S", "edit status"),
	),
}

func ProjectFullHelp() []key.Binding {
	return []key.Binding{
		ProjectKeys.Drill,
		ProjectKeys.Refresh,
		ProjectKeys.OpenWeb,
		ProjectKeys.LoadMore,
		ProjectKeys.Back,
		ProjectKeys.EditStatus,
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
		case "loadMore":
			binding = &ProjectKeys.LoadMore
		case "back":
			binding = &ProjectKeys.Back
		case "editStatus":
			binding = &ProjectKeys.EditStatus
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
