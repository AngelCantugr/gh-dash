package persistcache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dlvhdr/gh-dash/v4/internal/xdgpath"
)

// ErrStaleGeneration is returned by PutIfFresh when the captured generation
// no longer matches the current generation for the key prefix.
var ErrStaleGeneration = errors.New("persistcache: stale generation")

// cacheEntry is the on-disk JSON format for a single cache entry.
type cacheEntry struct {
	Value     json.RawMessage `json:"value"`
	ExpiresAt time.Time       `json:"expiresAt"`
}

// Store is a TTL-based on-disk JSON cache. Each key maps to a single
// <root>/<key>.json file. Generation counters live in memory only —
// they reset to zero on process start, which is acceptable because the
// only consumers are within the same process.
type Store struct {
	root  string
	mu    sync.Mutex        // protects gens
	gens  map[string]uint64 // generation counters keyed by prefix string
	keyMu sync.Map          // map[string]*sync.Mutex; per-key write serialization
}

// New opens (or initializes) a cache rooted at xdgpath.CacheDir().
// Returns an error if the directory can't be created or read.
func New() (*Store, error) {
	root, err := xdgpath.CacheDir()
	if err != nil {
		return nil, fmt.Errorf("persistcache: get cache dir: %w", err)
	}
	return newWithRoot(root)
}

// newWithRoot creates a Store rooted at root, creating the directory if needed.
// Used by New and by tests.
func newWithRoot(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("persistcache: create cache root %q: %w", root, err)
	}
	return &Store{
		root: root,
		gens: make(map[string]uint64),
	}, nil
}

// validateKey returns an error if key is not a safe relative path.
// Keys must be non-empty, must not start with "/", must not contain ".."
// segments, and must not contain backslashes.
func validateKey(key string) error {
	if key == "" {
		return errors.New("persistcache: empty key")
	}
	if strings.HasPrefix(key, "/") {
		return errors.New("persistcache: key must not be an absolute path")
	}
	if strings.Contains(key, "\\") {
		return errors.New("persistcache: key must not contain backslashes")
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == ".." {
			return errors.New("persistcache: key must not contain .. segments")
		}
	}
	return nil
}

// keyPath returns the filesystem path for a cache key.
func (s *Store) keyPath(key string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}
	return filepath.Join(s.root, filepath.FromSlash(key)+".json"), nil
}

// keyPrefix extracts the directory prefix (including trailing slash) from key.
//
//	"projects/abc" → "projects/"
//	"abc"          → ""
//	"a/b/c"        → "a/b/"
func keyPrefix(key string) string {
	i := strings.LastIndex(key, "/")
	if i < 0 {
		return ""
	}
	return key[:i+1]
}

// getKeyMu returns the per-key mutex, creating it on first access.
func (s *Store) getKeyMu(key string) *sync.Mutex {
	val, _ := s.keyMu.LoadOrStore(key, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// Get reads a cache entry.
//   - Hit (not expired): returns (data, true, nil)
//   - Miss (file absent or TTL elapsed): returns (nil, false, nil)
//   - Read error: returns (nil, false, err)
//   - Corrupt JSON: deletes the file and returns (nil, false, nil)
func (s *Store) Get(key string) ([]byte, bool, error) {
	path, err := s.keyPath(key)
	if err != nil {
		return nil, false, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	var entry cacheEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		// Self-heal: delete the corrupt file and return a miss.
		_ = os.Remove(path)
		return nil, false, nil
	}

	if time.Now().After(entry.ExpiresAt) {
		return nil, false, nil
	}

	return []byte(entry.Value), true, nil
}

// Put writes a cache entry atomically (temp+rename) with the given TTL.
// It bumps the generation counter for the key's directory prefix so that
// in-flight PutIfFresh calls with an older captured generation are rejected.
func (s *Store) Put(key string, data []byte, ttl time.Duration) error {
	s.mu.Lock()
	s.gens[keyPrefix(key)]++
	s.mu.Unlock()

	return s.writeEntry(key, data, ttl)
}

// writeEntry performs the atomic file write: marshal → temp file → rename.
func (s *Store) writeEntry(key string, data []byte, ttl time.Duration) error {
	path, err := s.keyPath(key)
	if err != nil {
		return err
	}

	entry := cacheEntry{
		Value:     json.RawMessage(data),
		ExpiresAt: time.Now().Add(ttl),
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Serialize writes to the same key to prevent interleaved renames.
	mu := s.getKeyMu(key)
	mu.Lock()
	defer mu.Unlock()

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(encoded); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// Invalidate removes all cache entries whose path starts with
// <root>/<prefix> and bumps the generation counter for prefix so that
// in-flight PutIfFresh calls with the old generation are rejected.
func (s *Store) Invalidate(prefix string) error {
	s.mu.Lock()
	s.gens[prefix]++
	s.mu.Unlock()

	dir := filepath.Join(s.root, filepath.FromSlash(prefix))

	var toRemove []string

	if strings.HasSuffix(prefix, "/") {
		// Prefix is a directory — walk it and collect all .json files.
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if !d.IsDir() && strings.HasSuffix(d.Name(), ".json") {
				toRemove = append(toRemove, path)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	} else {
		// Prefix is a partial name — find .json files in the parent
		// directory whose base names start with the prefix's base name.
		parentDir := filepath.Dir(dir)
		namePrefix := filepath.Base(dir)
		entries, err := os.ReadDir(parentDir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		for _, e := range entries {
			if !e.IsDir() &&
				strings.HasSuffix(e.Name(), ".json") &&
				strings.HasPrefix(e.Name(), namePrefix) {
				toRemove = append(toRemove, filepath.Join(parentDir, e.Name()))
			}
		}
	}

	for _, p := range toRemove {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Generation returns the current generation counter for the given prefix.
// Callers should capture this value before starting an async fetch and pass
// it to PutIfFresh once the fetch completes.
func (s *Store) Generation(prefix string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gens[prefix]
}

// PutIfFresh writes only if the captured generation still matches the
// current generation for the key's directory prefix.
// Returns ErrStaleGeneration if the generation has advanced since
// capturedGen was obtained (e.g. due to an Invalidate or a concurrent Put).
func (s *Store) PutIfFresh(key string, data []byte, ttl time.Duration, capturedGen uint64) error {
	prefix := keyPrefix(key)

	s.mu.Lock()
	currentGen := s.gens[prefix]
	s.mu.Unlock()

	if capturedGen != currentGen {
		return ErrStaleGeneration
	}

	return s.writeEntry(key, data, ttl)
}
