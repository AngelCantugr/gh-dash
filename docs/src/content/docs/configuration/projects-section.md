---
title: Projects Sections
description: Configure GitHub Projects v2 sections in gh-dash.
---

:::tip
The Projects view is **enabled by default** as of the latest release. You can opt out by
setting the environment variable `FF_PROJECTS_VIEW=false` if you encounter any issues.
:::

GitHub Projects v2 sections let you browse your organisation's or user's project boards
directly in the terminal. Each section shows a list of projects; pressing **Enter** drills
down into the items of the selected project.

## YAML schema

```yaml
projectsSections:
  - title: "My Work"
    owners:
      - org:my-org
      - user:alice
    filters:
      closed: false
      titleContains: ""
    extraFields:
      - Priority
      - Iteration
    limit: 100
    cache:
      projectsTTL: 1h
      itemsTTL: 5m
  - title: "Org Roadmaps"
    owners:
      - org:platform-team
    filters:
      closed: false
    extraFields:
      - Priority

# Top-level disk cache (optional)
cache:
  enabled: true
  dir: ""          # defaults to OS cache dir

# Cursor-position persistence (optional)
state:
  enabled: true
```

### Field reference

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `title` | string | **required** | Label shown in the section tab. |
| `owners` | list of owner refs | `[]` | Projects owned by these orgs or users are included. Omit to query as the authenticated viewer. |
| `filters.closed` | bool | `false` | When `true`, include closed projects. |
| `filters.titleContains` | string | `""` | Case-insensitive substring filter on project title. |
| `extraFields` | list of strings | `[]` | Additional project-item custom-field names to fetch and display as columns. Field IDs are resolved automatically by name. |
| `limit` | int | `20` | Maximum number of projects to fetch per page. |
| `cache.projectsTTL` | duration | `"0s"` (disabled) | How long to cache the projects list before re-fetching. |
| `cache.itemsTTL` | duration | `"0s"` (disabled) | How long to cache project items before re-fetching. |

## Owner syntax

Owners are specified as `"<kind>:<login>"` strings:

```yaml
owners:
  - org:my-organisation     # all projects owned by the org
  - user:alice              # all projects owned by the user alice
```

:::caution
Bare logins like `my-organisation` (without the `org:` or `user:` prefix) are **rejected**
at config-parse time with an error message. Always include the kind prefix.
:::

If `owners` is omitted entirely, `gh-dash` queries projects visible to the currently
authenticated viewer (i.e. the GitHub account used by `gh auth login`).

## Extra fields

GitHub Projects v2 allows custom fields (e.g. Priority, Sprint, Iteration). Add their
**display names** to `extraFields`:

```yaml
extraFields:
  - Priority
  - Iteration
  - Sprint
```

`gh-dash` resolves the field's internal ID by name at fetch time and adds a column for each
extra field in the drill-down items view. Field names are matched case-insensitively.

## Cache and TTL configuration

Two independent TTL knobs control how long data is held in memory:

- **`cache.projectsTTL`** — controls the projects list (the top-level view).
- **`cache.itemsTTL`** — controls the items fetched after drilling into a project.

Both accept human-readable duration strings recognised by Go's `time.ParseDuration`:
`"1h"`, `"30m"`, `"5m30s"`, etc. A value of `"0s"` (the default) disables caching and
always fetches fresh data.

To persist the cache to disk between sessions, enable the top-level disk cache:

```yaml
cache:
  enabled: true
  dir: ""   # leave empty to use the OS default cache directory
```

## Cursor-state persistence

When `state.enabled` is `true`, `gh-dash` remembers the last-selected row in each section
across restarts:

```yaml
state:
  enabled: true
```

:::note
State is saved per-session. If two `gh-dash` instances are running simultaneously, the last
one to exit wins — the other instance's cursor position will be overwritten.
:::

## Making the Projects view the startup view

Set `defaults.view` to `projects` in your config:

```yaml
defaults:
  view: projects
```

Make sure you have at least one entry under `projectsSections`, otherwise the view will be
empty.

## Keybindings reference

See the [Projects View keybindings page](/keybindings/projects-view/) for the full list of
keybindings available in the projects list and drill-down views.

## Known limitations

- **Viewer-scoped item count**: GitHub's Projects v2 API returns items scoped to the
  authenticated viewer. Items assigned to other team members are fetched, but the total
  count shown may differ from the web UI.
- **Two-instance state conflict**: If two `gh-dash` instances write cursor state
  simultaneously, the last writer wins. Avoid running two instances with `state.enabled:
  true` against the same state file.
- **No search**: The projects list view does not support the `/` search prefix currently
  available in PR and Issue sections.

## Future work

Planned improvements tracked in the project's roadmap:

- Full-text search within the projects list.
- Status mutation directly from the items drill-down view.
- Support for filtering by custom field values.
