---
title: Projects View Keybindings
description: Keybindings for the GitHub Projects v2 view in gh-dash.
---

The Projects view has two sub-views:

1. **Projects list** — the top-level list of all projects returned by your configured sections.
2. **Items view** — the drill-down view showing items inside a single project.

## Projects list keybindings

| Key | Builtin name | Action |
| --- | --- | --- |
| `Enter` | `drill` | Open the selected project and show its items. |
| `r` | `refresh` | Refresh the projects list from GitHub. |
| `o` | `openWeb` | Open the selected project in your default browser. |

## Items view keybindings

| Key | Builtin name | Action |
| --- | --- | --- |
| `Enter` | `drill` | Open the selected item in your default browser. |
| `Esc` / `b` | `back` | Return to the projects list. |
| `r` | `refresh` | Refresh the items list. |
| `>` | `loadMore` | Load the next page of items (pagination). |
| `/` | — | Filter items by text (in-view search). |
| `o` | `openWeb` | Open the selected item in your default browser. |

## Customising keybindings

You can rebind any built-in key or add custom commands via the `keybindings.projects`
section of your config:

```yaml
keybindings:
  projects:
    # Rebind "drill" to "d"
    - key: d
      builtin: drill
    # Rebind "back" to "q"
    - key: q
      builtin: back
    # Add a custom command
    - key: c
      command: "gh issue create --repo {{.RepoPath}}"
      name: "Create issue"
```

:::note
Custom command bindings for the projects view use the same templating engine as PR and
Issue sections. The `{{.RepoPath}}` template variable resolves to the current repo's
`owner/repo` path when `gh-dash` is run inside a git repository.
:::

## Columns — projects list

The projects list displays the following columns:

| Column | Width | Description |
| --- | --- | --- |
| State icon | 3 | Open / closed status indicator |
| Title | (grow) | Project title |
| Owner | 20 | Organisation or user that owns the project |
| Status | 6 | Open / closed label |
| Items | 6 | Number of items in the project |
| Updated | 10 | Last-updated date |

## Columns — items view

The items drill-down view displays:

| Column | Width | Description |
| --- | --- | --- |
| Title | (grow) | Item title |
| Type | 12 | Item type (Issue, PR, Draft, etc.) |
| Repo | 20 | Repository (`owner/name`) |
| Status | 16 | Project item status value |
| Updated | 12 | Last-updated date |
| *Extra fields* | 12 each | One column per entry in `extraFields` |

Extra-field columns are appended in the order they appear in your `extraFields` list.
