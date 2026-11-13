package media

import (
	"os"
	"sync"
)

// HashCache memoises audio-hash computation by file path.
//
// The bulk uploader computes each file's audio-hash twice: once in
// pre-flight (to detect duplicates and unreadable files before any write
// happens) and again during upload. On a library of 2,000+ files that is
// a second full read of the audio directory.
//
// The cache is keyed by absolute path, and it stores the file's size and
// modification time alongside the hash. If either changed, the entry is
// invalidated and the hash recomputed — a file edited between the
// pre-flight and the upload cannot carry a stale hash.
//
// The type is safe for concurrent use: the uploader may hash several
// files in parallel.
type HashCache struct {
	mu      sync.Mutex
	entries map[string]hashEntry
}

type hashEntry struct {
	hash    string
	size    int64
	modTime int64
}

// NewHashCache returns an empty cache.
func NewHashCache() *HashCache {
	return &HashCache{entries: make(map[string]hashEntry)}
}

// Hash returns the audio-hash for a file, computing it only if it is
// not already cached for the file's current size and modification time.
//
// The second return value reports whether the hash came from the cache.
func (c *HashCache) Hash(filePath string) (string, bool, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", false, err
	}

	c.mu.Lock()
	entry, ok := c.entries[filePath]
	c.mu.Unlock()

	if ok && entry.size == info.Size() && entry.modTime == info.ModTime().UnixNano() {
		return entry.hash, true, nil
	}

	hash, err := CalculateAudioHash(filePath)
	if err != nil {
		return "", false, err
	}

	c.mu.Lock()
	c.entries[filePath] = hashEntry{
		hash:    hash,
		size:    info.Size(),
		modTime: info.ModTime().UnixNano(),
	}
	c.mu.Unlock()

	return hash, false, nil
}
