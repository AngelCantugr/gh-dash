package persistcache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// helper: create a Store rooted in an isolated temp directory.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := newWithRoot(t.TempDir())
	require.NoError(t, err)
	return store
}

// TestNew_CreatesRoot verifies that New() (via newWithRoot) creates the
// cache root directory when it is absent.
func TestNew_CreatesRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache", "v1")
	// root does not exist yet
	_, err := os.Stat(root)
	require.True(t, os.IsNotExist(err), "precondition: root must not exist")

	store, err := newWithRoot(root)
	require.NoError(t, err)
	require.NotNil(t, store)

	info, err := os.Stat(root)
	require.NoError(t, err)
	require.True(t, info.IsDir(), "cache root must be a directory")
}

// TestGet_Miss verifies that Get on a non-existent key returns (nil, false, nil).
func TestGet_Miss(t *testing.T) {
	s := newTestStore(t)
	data, hit, err := s.Get("nonexistent")
	require.NoError(t, err)
	require.False(t, hit)
	require.Nil(t, data)
}

// TestPutGet_RoundTrip verifies that data written with Put can be read back.
func TestPutGet_RoundTrip(t *testing.T) {
	s := newTestStore(t)

	payload := []byte(`{"repo":"owner/name","count":42}`)
	require.NoError(t, s.Put("mykey", payload, time.Hour))

	data, hit, err := s.Get("mykey")
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, payload, data)
}

// TestPutGet_NestedKey verifies that a key with path separators works.
func TestPutGet_NestedKey(t *testing.T) {
	s := newTestStore(t)

	payload := []byte(`["a","b","c"]`)
	require.NoError(t, s.Put("projects/owner123", payload, time.Hour))

	data, hit, err := s.Get("projects/owner123")
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, payload, data)
}

// TestTTL_NotExpired verifies that a fresh entry (TTL not yet elapsed) is returned.
func TestTTL_NotExpired(t *testing.T) {
	s := newTestStore(t)

	payload := []byte(`"hello"`)
	require.NoError(t, s.Put("k", payload, time.Hour))

	data, hit, err := s.Get("k")
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, payload, data)
}

// TestTTL_Expired verifies that an entry with elapsed TTL is treated as a miss.
func TestTTL_Expired(t *testing.T) {
	s := newTestStore(t)

	payload := []byte(`"stale"`)
	// Write with a negative TTL so it is already expired.
	require.NoError(t, s.Put("expired", payload, -time.Second))

	data, hit, err := s.Get("expired")
	require.NoError(t, err)
	require.False(t, hit, "expired entry must be a miss")
	require.Nil(t, data)
}

// TestGet_CorruptFile verifies that a corrupt JSON file is deleted and Get
// returns a miss (no error).
func TestGet_CorruptFile(t *testing.T) {
	s := newTestStore(t)

	// Write a syntactically invalid JSON file directly.
	keyFile := filepath.Join(s.root, "corrupt.json")
	require.NoError(t, os.WriteFile(keyFile, []byte("NOT_VALID_JSON{{{"), 0o644))

	data, hit, err := s.Get("corrupt")
	require.NoError(t, err)
	require.False(t, hit, "corrupt file must be a miss")
	require.Nil(t, data)

	// The file must have been deleted.
	_, statErr := os.Stat(keyFile)
	require.True(t, os.IsNotExist(statErr), "corrupt cache file must be deleted by Get")
}

// TestGet_ValidJSONMissingFields verifies that a structurally valid JSON file
// that does not match the cacheEntry schema is also treated as corrupt and
// deleted.
func TestGet_ValidJSONMissingFields(t *testing.T) {
	s := newTestStore(t)

	// Valid JSON but missing the required "expiresAt" field results in the
	// zero-value time (before epoch) which counts as expired, not corrupt.
	// Instead write something that cannot unmarshal into cacheEntry at all.
	keyFile := filepath.Join(s.root, "badschema.json")
	require.NoError(t, os.WriteFile(keyFile, []byte(`[1,2,3]`), 0o644))

	// [1,2,3] cannot unmarshal into cacheEntry (which is a JSON object).
	data, hit, err := s.Get("badschema")
	require.NoError(t, err)
	require.False(t, hit)
	require.Nil(t, data)

	// File must be deleted.
	_, statErr := os.Stat(keyFile)
	require.True(t, os.IsNotExist(statErr))
}

// TestInvalidate_Directory verifies that Invalidate("prefix/") removes all
// .json files under that directory and returns nil for non-existent directories.
func TestInvalidate_Directory(t *testing.T) {
	s := newTestStore(t)

	// Write several entries under "projects/".
	keys := []string{"projects/a", "projects/b", "projects/c"}
	for _, k := range keys {
		require.NoError(t, s.Put(k, []byte(`"v"`), time.Hour))
	}
	// Write an entry outside "projects/".
	require.NoError(t, s.Put("other/x", []byte(`"v"`), time.Hour))

	require.NoError(t, s.Invalidate("projects/"))

	for _, k := range keys {
		_, hit, err := s.Get(k)
		require.NoError(t, err)
		require.False(t, hit, "key %q must be a miss after Invalidate", k)
	}

	// Unrelated key must still be present.
	_, hit, err := s.Get("other/x")
	require.NoError(t, err)
	require.True(t, hit, "unrelated key must survive Invalidate")
}

// TestInvalidate_NonExistentDirectory verifies that Invalidate on a path that
// does not exist is a no-op (no error).
func TestInvalidate_NonExistentDirectory(t *testing.T) {
	s := newTestStore(t)
	err := s.Invalidate("ghost/")
	require.NoError(t, err)
}

// TestGenerationCounter_Initial verifies that a brand-new prefix returns 0.
func TestGenerationCounter_Initial(t *testing.T) {
	s := newTestStore(t)
	require.Equal(t, uint64(0), s.Generation("projects/"))
}

// TestGenerationCounter_BumpsOnInvalidate verifies that Invalidate bumps
// the generation counter for the given prefix.
func TestGenerationCounter_BumpsOnInvalidate(t *testing.T) {
	s := newTestStore(t)

	gen0 := s.Generation("projects/")
	require.NoError(t, s.Invalidate("projects/"))
	require.Equal(t, gen0+1, s.Generation("projects/"))
}

// TestGenerationCounter_BumpsOnPut verifies that Put bumps the generation
// counter for the key's prefix.
func TestGenerationCounter_BumpsOnPut(t *testing.T) {
	s := newTestStore(t)

	gen0 := s.Generation("projects/")
	require.NoError(t, s.Put("projects/abc", []byte(`"v"`), time.Hour))
	require.Equal(t, gen0+1, s.Generation("projects/"))
}

// TestPutIfFresh_StaleGenerationRejected verifies that PutIfFresh returns
// ErrStaleGeneration when the prefix generation has advanced.
func TestPutIfFresh_StaleGenerationRejected(t *testing.T) {
	s := newTestStore(t)

	// Capture the generation before any writes.
	capturedGen := s.Generation("projects/")

	// Invalidate bumps the generation.
	require.NoError(t, s.Invalidate("projects/"))

	// PutIfFresh with the old captured generation must be rejected.
	err := s.PutIfFresh("projects/abc", []byte(`"v"`), time.Hour, capturedGen)
	require.ErrorIs(t, err, ErrStaleGeneration)

	// The key must NOT have been written.
	_, hit, getErr := s.Get("projects/abc")
	require.NoError(t, getErr)
	require.False(t, hit, "stale PutIfFresh must not write the cache entry")
}

// TestPutIfFresh_StaleAfterPartialKeyInvalidate verifies that invalidating a
// single key ("dir/key", no trailing slash) also advances the *directory*
// generation that PutIfFresh checks. Regression test: UpdateItemStatus
// invalidates "project-items/<projectID>" while in-flight load-more fetches
// call PutIfFresh gated on Generation("project-items/") — without the
// directory bump the stale write would succeed and resurrect pre-mutation
// data.
func TestPutIfFresh_StaleAfterPartialKeyInvalidate(t *testing.T) {
	s := newTestStore(t)

	// Simulate an in-flight fetch capturing the directory generation.
	capturedGen := s.Generation("project-items/")

	// A mutation invalidates just this project's entry (partial-name form).
	require.NoError(t, s.Invalidate("project-items/PVT_abc"))

	// The in-flight fetch completes; its write must be rejected as stale.
	err := s.PutIfFresh("project-items/PVT_abc", []byte(`"stale"`), time.Hour, capturedGen)
	require.ErrorIs(t, err, ErrStaleGeneration)

	_, hit, getErr := s.Get("project-items/PVT_abc")
	require.NoError(t, getErr)
	require.False(t, hit, "stale PutIfFresh must not write the cache entry")
}

// TestPutIfFresh_CurrentGenerationAccepted verifies that PutIfFresh succeeds
// when the generation has not advanced.
func TestPutIfFresh_CurrentGenerationAccepted(t *testing.T) {
	s := newTestStore(t)

	capturedGen := s.Generation("projects/")
	payload := []byte(`"fresh data"`)

	err := s.PutIfFresh("projects/abc", payload, time.Hour, capturedGen)
	require.NoError(t, err)

	data, hit, getErr := s.Get("projects/abc")
	require.NoError(t, getErr)
	require.True(t, hit)
	require.Equal(t, payload, data)
}

// TestAtomicWrite_TempFileOrphaned verifies that an orphaned .tmp-* file
// (simulating a SIGKILL between CreateTemp and Rename) does not corrupt
// the existing cache entry — Get returns the previously written value.
func TestAtomicWrite_TempFileOrphaned(t *testing.T) {
	s := newTestStore(t)

	oldPayload := []byte(`"old value"`)
	require.NoError(t, s.Put("akey", oldPayload, time.Hour))

	// Simulate a torn write: write a .tmp- file as if the rename never happened.
	tmp, err := os.CreateTemp(s.root, ".tmp-*")
	require.NoError(t, err)
	entry := cacheEntry{
		Value:     json.RawMessage(`"new value"`),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	encoded, _ := json.Marshal(entry)
	_, _ = tmp.Write(encoded)
	_ = tmp.Close()

	// Get must return the old value, not the temp file.
	data, hit, err := s.Get("akey")
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, oldPayload, data, "Get must return the previously committed value")
}

// TestAtomicWrite_NoFileYet verifies that an orphaned .tmp-* file with no
// corresponding .json file causes Get to return a miss.
func TestAtomicWrite_NoFileYet(t *testing.T) {
	s := newTestStore(t)

	// Plant a .tmp- file without an accompanying .json file.
	tmp, err := os.CreateTemp(s.root, ".tmp-*")
	require.NoError(t, err)
	_ = tmp.Close()

	_, hit, err := s.Get("notyetwritten")
	require.NoError(t, err)
	require.False(t, hit, "orphaned temp file must not cause a hit")
}

// TestConcurrentPuts_DifferentKeys verifies that concurrent writes to
// distinct keys complete without deadlock.
func TestConcurrentPuts_DifferentKeys(t *testing.T) {
	s := newTestStore(t)

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key%d", i)
			data := []byte(fmt.Sprintf(`"value%d"`, i))
			_ = s.Put(key, data, time.Hour)
		}(i)
	}
	wg.Wait()

	// All keys must be readable.
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("key%d", i)
		data, hit, err := s.Get(key)
		require.NoError(t, err)
		require.True(t, hit, "key %q must exist after concurrent Put", key)
		expected := []byte(fmt.Sprintf(`"value%d"`, i))
		require.Equal(t, expected, data)
	}
}

// TestConcurrentPuts_SameKey verifies that concurrent writes to the same key
// complete without deadlock. The last writer wins; the final value must be
// valid JSON.
func TestConcurrentPuts_SameKey(t *testing.T) {
	s := newTestStore(t)

	const n = 30
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			data := []byte(fmt.Sprintf(`"value%d"`, i))
			_ = s.Put("shared", data, time.Hour)
		}(i)
	}
	wg.Wait()

	data, hit, err := s.Get("shared")
	require.NoError(t, err)
	require.True(t, hit)
	require.NotEmpty(t, data)
	// The stored bytes must themselves be valid JSON.
	var v interface{}
	require.NoError(t, json.Unmarshal(data, &v))
}

// TestValidateKey covers various invalid and valid key strings.
func TestValidateKey(t *testing.T) {
	cases := []struct {
		key   string
		valid bool
	}{
		{"", false},
		{"/absolute", false},
		{"a/../b", false},
		{"..", false},
		{"a\\b", false},
		{"normal", true},
		{"projects/owner123", true},
		{"a/b/c", true},
	}
	for _, tc := range cases {
		err := validateKey(tc.key)
		if tc.valid {
			require.NoError(t, err, "key %q should be valid", tc.key)
		} else {
			require.Error(t, err, "key %q should be invalid", tc.key)
		}
	}
}

// TestKeyPrefix covers the prefix extraction helper.
func TestKeyPrefix(t *testing.T) {
	require.Equal(t, "projects/", keyPrefix("projects/abc"))
	require.Equal(t, "", keyPrefix("flatkey"))
	require.Equal(t, "a/b/", keyPrefix("a/b/c"))
}
