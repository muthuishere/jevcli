package core

import (
	"testing"
	"time"
)

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := Profile{URL: "http://x", Model: "m"}
	w := map[string]any{"type": "noul", "instructions": "q"}
	k := CacheKey(p, "state", w)
	if _, _, ok := CacheGet(dir, k, time.Hour); ok {
		t.Fatal("hit on an empty cache")
	}
	v := 0.9
	CachePut(dir, k, Answer{Type: "noul", Noul: &v}, "m1")
	a, m, ok := CacheGet(dir, k, time.Hour)
	if !ok || a.Noul == nil || *a.Noul != 0.9 || m != "m1" {
		t.Fatalf("got %+v %v %v", a, m, ok)
	}
	if _, _, ok := CacheGet(dir, k, -time.Second); ok {
		t.Fatal("an expired entry was served")
	}
	CacheFresh = true
	_, _, ok = CacheGet(dir, k, time.Hour)
	CacheFresh = false
	if ok {
		t.Fatal("--fresh still read the cache")
	}
	for _, other := range []string{CacheKey(p, "other", w), CacheKey(Profile{URL: "http://x", Model: "m2"}, "state", w),
		CacheKey(p, "state", map[string]any{"type": "noul", "instructions": "q2"})} {
		if other == k {
			t.Fatal("different input gave the same key")
		}
	}
	if n, _ := CacheStat(dir); n != 1 {
		t.Fatalf("stat %d", n)
	}
	if CacheClear(dir) != 1 {
		t.Fatal("clear")
	}
	if _, _, ok := CacheGet(dir, k, time.Hour); ok {
		t.Fatal("hit after clear")
	}
}
