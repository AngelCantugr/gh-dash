package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"charm.land/log/v2"
	"github.com/spf13/cobra"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/persistcache"
)

// notifyState is persisted to disk between polls so counts survive restarts.
type notifyState struct {
	// Counts maps section title → last-seen TotalCount.
	Counts map[string]int `json:"counts"`
}

var notifyCmd = &cobra.Command{
	Use:   "notify",
	Short: "Poll PR sections and send a native notification when new PRs arrive",
	Long: `Runs as a background daemon, polling each configured PR section on a fixed
interval. When a section's total count grows since the last poll a macOS
notification banner is shown (Linux: notify-send). The process exits only on
SIGINT / SIGTERM.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		intervalFlag, _ := cmd.Flags().GetDuration("interval")
		stateFile, _ := cmd.Flags().GetString("state-file")

		cfg, err := config.ParseConfig(config.Location{ConfigFlag: cfgFlag})
		if err != nil {
			return fmt.Errorf("notify: load config: %w", err)
		}

		cache, err := persistcache.New()
		if err != nil {
			// Non-fatal: operate without cache.
			log.Warn("notify: cache unavailable", "err", err)
		}

		state := loadNotifyState(stateFile)

		log.Info("gh-dash notify started",
			"interval", intervalFlag,
			"sections", len(cfg.PRSections),
			"state_file", stateFile,
		)

		for {
			changed := pollSections(&cfg, cache, state)
			if changed {
				if err := saveNotifyState(stateFile, state); err != nil {
					log.Warn("notify: save state", "err", err)
				}
			}
			time.Sleep(intervalFlag)
		}
	},
}

func init() {
	notifyCmd.Flags().Duration("interval", 5*time.Minute, "how often to poll GitHub")
	notifyCmd.Flags().String("state-file", defaultNotifyStateFile(), "path to persist last-seen counts")
	rootCmd.AddCommand(notifyCmd)
}

// pollSections fetches each PR section and fires a notification when the count grows.
// Returns true if any count changed (so the state file can be saved).
func pollSections(cfg *config.Config, cache *persistcache.Store, state *notifyState) bool {
	changed := false
	limit := cfg.Defaults.PrsLimit
	if limit == 0 {
		limit = 20
	}

	for _, section := range cfg.PRSections {
		res, err := data.FetchPullRequests(cache, section.Filters, limit, nil)
		if err != nil {
			log.Warn("notify: fetch failed", "section", section.Title, "err", err)
			continue
		}

		prev, seen := state.Counts[section.Title]
		if seen && res.TotalCount > prev {
			diff := res.TotalCount - prev
			sendNotification(
				"gh-dash",
				fmt.Sprintf("%s: %d new PR%s", section.Title, diff, plural(diff)),
			)
			changed = true
		}
		state.Counts[section.Title] = res.TotalCount
		if !seen {
			changed = true
		}
	}
	return changed
}

func sendNotification(title, body string) {
	log.Info("notify", "title", title, "body", body)
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(
			`display notification %q with title %q`,
			body, title,
		)
		if err := exec.Command("osascript", "-e", script).Run(); err != nil {
			log.Warn("notify: osascript failed", "err", err)
		}
	case "linux":
		if err := exec.Command("notify-send", title, body).Run(); err != nil {
			log.Warn("notify: notify-send failed", "err", err)
		}
	default:
		// Fallback: terminal bell.
		fmt.Print("\a")
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func defaultNotifyStateFile() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ".gh-dash-notify.json"
	}
	return filepath.Join(dir, "gh-dash", "notify-state.json")
}

func loadNotifyState(path string) *notifyState {
	state := &notifyState{Counts: make(map[string]int)}
	raw, err := os.ReadFile(path)
	if err != nil {
		return state
	}
	_ = json.Unmarshal(raw, state)
	if state.Counts == nil {
		state.Counts = make(map[string]int)
	}
	return state
}

func saveNotifyState(path string, state *notifyState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}
