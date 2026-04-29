package keys

import (
	"fmt"

	"charm.land/bubbles/v2/key"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
)

type ProjectKeyMap struct{}

var ProjectKeys = ProjectKeyMap{}

func ProjectFullHelp() []key.Binding {
	return []key.Binding{}
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

		// No built-in project keys yet — reject unknown built-ins
		return fmt.Errorf("unknown built-in project key: '%s'", projectKey.Builtin)
	}

	return nil
}
