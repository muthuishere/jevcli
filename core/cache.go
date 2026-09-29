package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The answer cache: one small file per (endpoint, model, input, question), named by the hash of all of them, so
// nothing about the input is stored, only what the model said. The verdict is worked out after the lookup, so
// changing --yes / --min never invalidates anything.

// CacheFresh skips cache reads for this process (--fresh); a new answer still overwrites the entry.
var CacheFresh bool

type cacheEntry struct {
	At     int64  `json:"at"`
	Model  any    `json:"model,omitempty"`
	Answer Answer `json:"answer"`
}

// CacheDirPath is where the cache lives: cache_dir from config.json (a leading ~ works), else ~/.cache/jevx.
func (c Config) CacheDirPath() string {
	d := strings.TrimSpace(os.ExpandEnv(c.CacheDir))
	switch {
	case d == "":
		return filepath.Join(Home(), ".cache/jevx")
	case d == "~" || strings.HasPrefix(d, "~/"):
		return filepath.Join(Home(), strings.TrimPrefix(d, "~"))
	}
	return d
}

// CacheKey hashes everything that decides the answer. Headers are left out: rotating a key must not empty the cache.
func CacheKey(p Profile, state string, wire map[string]any) string {
	b, _ := json.Marshal([]any{p.URL, p.Model, state, wire}) // map keys marshal sorted, so the key is stable
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func cachePath(dir, key string) string { return filepath.Join(dir, key[:2], key+".json") }

// CacheGet returns a stored answer younger than ttl (and the model that gave it).
func CacheGet(dir, key string, ttl time.Duration) (Answer, any, bool) {
	if CacheFresh || dir == "" {
		return Answer{}, nil, false
	}
	b, err := os.ReadFile(cachePath(dir, key))
	if err != nil {
		return Answer{}, nil, false
	}
	var e cacheEntry
	if json.Unmarshal(b, &e) != nil || e.Answer.Type == "" || time.Since(time.Unix(e.At, 0)) > ttl {
		return Answer{}, nil, false
	}
	return e.Answer, e.Model, true
}

// CachePut stores an answer. It writes a temp file and renames it, so parallel workers never leave half a file.
// A cache that cannot be written is not an error: the answer is already in hand.
func CachePut(dir, key string, a Answer, model any) {
	if dir == "" {
		return
	}
	p := cachePath(dir, key)
	if os.MkdirAll(filepath.Dir(p), 0o700) != nil {
		return
	}
	b, _ := json.Marshal(cacheEntry{At: time.Now().Unix(), Model: model, Answer: a})
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	if tmp.Close() != nil || werr != nil || os.Rename(tmp.Name(), p) != nil {
		os.Remove(tmp.Name())
	}
}

// CacheStat counts the stored answers and their size.
func CacheStat(dir string) (n int, bytes int64) {
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".json") {
			if fi, e := d.Info(); e == nil {
				n, bytes = n+1, bytes+fi.Size()
			}
		}
		return nil
	})
	return
}

// CacheClear deletes every stored answer (only the two-hex-digit folders this cache makes) and returns how many.
func CacheClear(dir string) int {
	n, _ := CacheStat(dir)
	subs, _ := os.ReadDir(dir)
	for _, s := range subs {
		if s.IsDir() && len(s.Name()) == 2 {
			os.RemoveAll(filepath.Join(dir, s.Name()))
		}
	}
	return n
}
