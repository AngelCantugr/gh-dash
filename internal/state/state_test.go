package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// makeStore creates a Store backed by a file inside tempDir. If filename is
// empty a default "state.json" name is used.
func makeStore(t *testing.T, dir, filename string) *Store {
	t.Helper()
	if filename == "" {
		filename = "state.json"
	}
	return newWithFilePath(filepath.Join(dir, filename))
}

func TestNew_MissingFile(t *testing.T) {
	dir := t.TempDir()
	s := makeStore(t, dir, "")
	snap := s.Snapshot()
	if snap.LastSectionID != "" {
		t.Errorf("expected empty LastSectionID, got %q", snap.LastSectionID)
	}
	if len(snap.PerSection) != 0 {
		t.Errorf("expected empty PerSection, got %v", snap.PerSection)
	}
}

func TestNew_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{invalid json"), 0o644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	// Should not error; state should be reset to defaults.
	s := newWithFilePath(path)
	snap := s.Snapshot()
	if snap.LastSectionID != "" {
		t.Errorf("corrupt file: expected empty LastSectionID, got %q", snap.LastSectionID)
	}
	if len(snap.PerSection) != 0 {
		t.Errorf("corrupt file: expected empty PerSection, got %v", snap.PerSection)
	}
}

func TestSetLastSection(t *testing.T) {
	dir := t.TempDir()
	s := makeStore(t, dir, "")

	s.SetLastSection("my-prs")
	s.Flush()

	snap := s.Snapshot()
	if snap.LastSectionID != "my-prs" {
		t.Errorf("expected LastSectionID %q, got %q", "my-prs", snap.LastSectionID)
	}
}

func TestSetCursor(t *testing.T) {
	dir := t.TempDir()
	s := makeStore(t, dir, "")

	s.SetCursor("my-prs", 5)
	s.Flush()

	snap := s.Snapshot()
	sec, ok := snap.PerSection["my-prs"]
	if !ok {
		t.Fatal("expected PerSection[my-prs] to exist")
	}
	if sec.Cursor != 5 {
		t.Errorf("expected Cursor 5, got %d", sec.Cursor)
	}
}

func TestSetLastProject(t *testing.T) {
	dir := t.TempDir()
	s := makeStore(t, dir, "")

	before := time.Now().UTC().Truncate(time.Second)
	s.SetLastProject("my-prs", "PVT_abc123")
	s.Flush()
	after := time.Now().UTC().Add(time.Second)

	snap := s.Snapshot()
	sec, ok := snap.PerSection["my-prs"]
	if !ok {
		t.Fatal("expected PerSection[my-prs] to exist")
	}
	if sec.LastProjectID != "PVT_abc123" {
		t.Errorf("expected LastProjectID %q, got %q", "PVT_abc123", sec.LastProjectID)
	}
	if sec.LastVisitedAt.Before(before) || sec.LastVisitedAt.After(after) {
		t.Errorf("LastVisitedAt %v out of expected range [%v, %v]", sec.LastVisitedAt, before, after)
	}
}

func TestFlush_WritesImmediately(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s := newWithFilePath(path)

	s.SetCursor("sec1", 7)
	// Do NOT wait for the debounce; call Flush immediately.
	s.Flush()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file to exist after Flush: %v", err)
	}
	var loaded SessionState
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatalf("unmarshal after Flush: %v", err)
	}
	if loaded.PerSection["sec1"].Cursor != 7 {
		t.Errorf("expected cursor 7, got %d", loaded.PerSection["sec1"].Cursor)
	}
}

func TestSetCursor_DebounceCoalesces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s := newWithFilePath(path)

	// Fire 10 rapid SetCursor calls; they should coalesce into one write.
	const calls = 10
	for i := range calls {
		s.SetCursor("sec1", i)
	}

	// Wait past the debounce window.
	time.Sleep(debounceDuration + 200*time.Millisecond)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file after debounce: %v", err)
	}
	var loaded SessionState
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// The final call was SetCursor("sec1", 9).
	if loaded.PerSection["sec1"].Cursor != calls-1 {
		t.Errorf("expected cursor %d (last call), got %d", calls-1, loaded.PerSection["sec1"].Cursor)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s1 := newWithFilePath(path)
	s1.SetLastSection("projects-my-work")
	s1.SetCursor("projects-my-work", 3)
	s1.SetLastProject("projects-my-work", "PVT_abc123")
	s1.Flush()

	// Load into a second store instance.
	s2 := newWithFilePath(path)
	snap := s2.Snapshot()
	if snap.LastSectionID != "projects-my-work" {
		t.Errorf("expected LastSectionID %q, got %q", "projects-my-work", snap.LastSectionID)
	}
	sec := snap.PerSection["projects-my-work"]
	if sec.Cursor != 3 {
		t.Errorf("expected cursor 3, got %d", sec.Cursor)
	}
	if sec.LastProjectID != "PVT_abc123" {
		t.Errorf("expected LastProjectID %q, got %q", "PVT_abc123", sec.LastProjectID)
	}
}

func TestConcurrentSetCursor_NoRace(t *testing.T) {
	dir := t.TempDir()
	s := makeStore(t, dir, "")

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(n int) {
			defer wg.Done()
			s.SetCursor("sec1", n)
		}(i)
	}
	wg.Wait()
	s.Flush()

	// Verify the store is still consistent (no panic, valid state).
	snap := s.Snapshot()
	sec, ok := snap.PerSection["sec1"]
	if !ok {
		t.Fatal("expected PerSection[sec1] to exist after concurrent writes")
	}
	if sec.Cursor < 0 || sec.Cursor >= goroutines {
		t.Errorf("cursor %d out of valid range [0, %d)", sec.Cursor, goroutines)
	}
}

// TestTwoInstances documents the last-writer-wins behavior when two Store
// instances share the same file. Both must remain functional; data loss is
// the documented user tradeoff in the MVP.
func TestTwoInstances_LastWriterWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Instance A writes first.
	a := newWithFilePath(path)
	a.SetCursor("sec1", 1)
	a.Flush()

	// Instance B writes second — wins.
	b := newWithFilePath(path)
	b.SetCursor("sec1", 2)
	b.Flush()

	// Both instances are still functional.
	_ = a.Snapshot()
	_ = b.Snapshot()

	// The file reflects B's last write.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	var loaded SessionState
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if loaded.PerSection["sec1"].Cursor != 2 {
		t.Errorf("expected last-writer (B) cursor 2, got %d", loaded.PerSection["sec1"].Cursor)
	}
}

func TestSnapshot_IsDeepCopy(t *testing.T) {
	dir := t.TempDir()
	s := makeStore(t, dir, "")
	s.SetCursor("sec1", 5)

	snap := s.Snapshot()
	// Mutate the snapshot; the store should not be affected.
	snap.PerSection["sec1"] = SectionState{Cursor: 99}

	snap2 := s.Snapshot()
	if snap2.PerSection["sec1"].Cursor != 5 {
		t.Errorf("snapshot mutation affected store: cursor = %d", snap2.PerSection["sec1"].Cursor)
	}
}
