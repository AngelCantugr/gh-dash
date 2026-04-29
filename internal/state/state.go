package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"charm.land/log/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/xdgpath"
)

const (
	stateFileName    = "state.json"
	debounceDuration = 500 * time.Millisecond
)

// SessionState is the top-level on-disk JSON format.
type SessionState struct {
	LastSectionID string                  `json:"lastSectionID"`
	PerSection    map[string]SectionState `json:"perSection"`
}

// SectionState is the per-section session state.
type SectionState struct {
	Cursor        int       `json:"cursor"`
	LastProjectID string    `json:"lastProjectID"`
	LastVisitedAt time.Time `json:"lastVisitedAt"`
}

// Store is a per-user JSON session-state store on disk. It persists last
// visited section, cursor index per section, and last visited project per
// section.
//
// Two-instance scenario: concurrent Store instances pointing at the same file
// operate as last-writer-wins. No file lock is held between reads and writes
// (MVP limitation). Both instances remain functional; a user running two
// terminals may lose cursor position across sessions — this is the
// documented tradeoff for MVP simplicity.
type Store struct {
	mu       sync.RWMutex
	state    SessionState
	filePath string

	timerMu sync.Mutex
	timer   *time.Timer
}

// New opens (or initializes) the state file at xdgpath.StateDir()/state.json.
// On a missing file it returns an empty-state Store (no error).
// On a corrupt file it logs a warning, resets to defaults, and returns no error.
func New() (*Store, error) {
	dir, err := xdgpath.StateDir()
	if err != nil {
		return nil, err
	}
	return newWithFilePath(filepath.Join(dir, stateFileName)), nil
}

// newWithFilePath creates a Store backed by filePath. The directory does not
// need to exist yet — it is created lazily on the first save. This helper
// exists so tests can inject a temp-dir path without going through xdgpath.
func newWithFilePath(filePath string) *Store {
	s := &Store{
		filePath: filePath,
		state: SessionState{
			PerSection: make(map[string]SectionState),
		},
	}
	s.load()
	return s
}

// load reads state from disk. Missing file → no-op. Corrupt file → warn + reset.
func (s *Store) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Warn("state: failed to read state file, using defaults", "err", err)
		return
	}

	var loaded SessionState
	if err := json.Unmarshal(data, &loaded); err != nil {
		log.Warn("state: corrupt state file, resetting to defaults", "err", err)
		return
	}

	if loaded.PerSection == nil {
		loaded.PerSection = make(map[string]SectionState)
	}
	s.state = loaded
}

// Snapshot returns a deep copy of the current session state.
func (s *Store) Snapshot() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.copyStateLocked()
}

// copyStateLocked returns a deep copy of s.state. Caller must hold at least
// a read lock on s.mu.
func (s *Store) copyStateLocked() SessionState {
	snap := SessionState{
		LastSectionID: s.state.LastSectionID,
		PerSection:    make(map[string]SectionState, len(s.state.PerSection)),
	}
	for k, v := range s.state.PerSection {
		snap.PerSection[k] = v
	}
	return snap
}

// SetLastSection records the most recently active section.
func (s *Store) SetLastSection(sectionID string) {
	s.mu.Lock()
	s.state.LastSectionID = sectionID
	s.mu.Unlock()
	s.scheduleSave()
}

// SetCursor records the cursor index for a given section.
// Writes to disk are debounced: multiple calls within 500 ms coalesce into a
// single disk write.
func (s *Store) SetCursor(sectionID string, cursor int) {
	s.mu.Lock()
	sec := s.state.PerSection[sectionID]
	sec.Cursor = cursor
	s.state.PerSection[sectionID] = sec
	s.mu.Unlock()
	s.scheduleSave()
}

// SetLastProject records the most recently drilled project for a section.
// Persisted for future use (§6.5); not yet consumed by the UI in MVP.
func (s *Store) SetLastProject(sectionID, projectID string) {
	s.mu.Lock()
	sec := s.state.PerSection[sectionID]
	sec.LastProjectID = projectID
	sec.LastVisitedAt = time.Now().UTC()
	s.state.PerSection[sectionID] = sec
	s.mu.Unlock()
	s.scheduleSave()
}

// scheduleSave resets (or creates) the debounce timer. When the timer fires,
// the current state is written to disk.
func (s *Store) scheduleSave() {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(debounceDuration, func() {
		if err := s.save(); err != nil {
			log.Warn("state: background save failed", "err", err)
		}
	})
}

// Flush cancels any pending debounce timer and writes the current state to
// disk immediately. Best-effort: errors are logged, not returned. Intended to
// be called on graceful shutdown (e.g., from tea.QuitMsg handling).
func (s *Store) Flush() {
	s.timerMu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.timerMu.Unlock()

	if err := s.save(); err != nil {
		log.Warn("state: flush failed", "err", err)
	}
}

// save marshals the current state and writes it atomically (temp → rename).
func (s *Store) save() error {
	if s.filePath == "" {
		return nil
	}

	s.mu.RLock()
	snap := s.copyStateLocked()
	s.mu.RUnlock()

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".tmp-state-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	log.Debug("state: saved session state")
	return nil
}
