# gh-dash

A rich terminal UI for GitHub that doesn't break your flow.

## 🌟 Features

> [!NOTE]
> If you like quickly navigating with your keyboard, seeing the PRs and issues you need and you <strong>love the terminal</strong> - <code>DASH</code> is for you! 🫵🏽

- User-defined, per-repo, PRs & issues sections
- Overridable vim-style keyboard hotkeys
- Custom actions to perform your specific workflow needs
- Everything you can do on GitHub - diff, comment, checkout, push, update etc.
- Control every setting with a YAML config file

## 🔧 Building & Installing from Source

No releases are published from this fork — you must build and install from source.

### Prerequisites

- **[GitHub CLI (`gh`)](https://cli.github.com/)** — required at runtime for auth and as the extension host (`gh auth login` before first run)
- **[Go 1.23+](https://go.dev/dl/)** — compiler
- **[go-task](https://taskfile.dev/installation/)** — task runner (`task` CLI)

### Option A: Devbox (recommended)

[Devbox](https://www.jetpack.io/devbox) pins the exact toolchain (Go, go-task, linters) used in CI. One command sets everything up:

```sh
curl -fsSL https://get.jetpack.io/devbox | bash
git clone https://github.com/AngelCantugr/gh-dash dash && cd dash
devbox shell          # installs Go 1.23, go-task, gh, linters automatically
task install          # build → install as gh extension → verify
```

### Option B: Manual (Go + go-task already installed)

```sh
git clone https://github.com/AngelCantugr/gh-dash dash && cd dash
gh auth login         # if not already authenticated
task install          # build → install as gh extension → verify
```

Or without go-task:

```sh
go build .
gh ext install .
gh dash --version
```

### Updating

```sh
git pull
task install          # rebuild and reinstall — or: go build . && gh ext install .
```

### Uninstalling

```sh
gh ext remove dash
```

---

## ❤️ Donating

If you enjoy `DASH` and want to help, consider supporting the project with a
donation at the [sponsors page](https://github.com/sponsors/dlvhdr).

## 👥 Discord

Have questions? Join our [Discord community](https://discord.gg/SXNXp9NctV)!

## 🙏 Contributing

See the contribution guide at [https://www.gh-dash.dev/contributing](https://www.gh-dash.dev/contributing/).

## 🛞 Under the hood

`DASH` uses:

- [bubbletea](https://github.com/charmbracelet/bubbletea) for the TUI
- [lipgloss](https://github.com/charmbracelet/lipgloss) for the styling
- [glamour](https://github.com/charmbracelet/glamour) for rendering markdown
- [vhs](https://github.com/charmbracelet/vhs) for generating the GIF
- [cobra](https://github.com/spf13/cobra) for the CLI
- [gh](https://github.com/cli/cli) for the GitHub functionality
- [delta](https://github.com/dandavison/delta) for viewing PR diffs

## Author

Dolev Hadar [@dlvhdr](https://github.com/dlvhdr).
