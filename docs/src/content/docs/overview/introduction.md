---
title: Introduction
description: gh-dash — a terminal dashboard for GitHub PRs, issues, and notifications.
---

`gh-dash` is a `gh` CLI extension that renders a terminal dashboard for GitHub pull requests,
issues, and notifications — and now GitHub Projects v2. It is built on
[Bubble Tea](https://github.com/charmbracelet/bubbletea) (Elm-style TUI framework) and sources
data from GitHub's GraphQL API via the `gh` CLI.

## Installation

```sh
gh extension install dlvhdr/gh-dash
```

## Quick start

```sh
gh dash
```

On first run, `gh-dash` writes a default configuration file to
`~/.config/gh-dash/config.yml`. Edit it to add your own sections, filters, and keybindings.

## Views

`gh-dash` organises content into **views** — each view shows a different type of GitHub data.
Switch between views using the global `[` / `]` keybindings.

| View | Key | Description |
| --- | --- | --- |
| Pull Requests | `p` | Filtered lists of pull requests |
| Issues | `i` | Filtered lists of issues |
| Notifications | `n` | Your GitHub notification inbox |
| Projects | (tab) | GitHub Projects v2 boards |

## Further reading

- [Configuration → PR Sections](/configuration/pr-sections/)
- [Configuration → Projects Sections](/configuration/projects-section/)
- [Keybindings → Projects View](/keybindings/projects-view/)
