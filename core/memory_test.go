package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// testMemory builds a memory over a temp folder: a wiki of three pages, a pinned notice, and a repo the pages cite.
func testMemory(t *testing.T) (*Memory, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir, repo := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(repo, "README.md"), "line one\nDefault port: 21118\nline three\n")
	writeFile(t, filepath.Join(dir, "faq.md"), "# FAQ\n\n## What port does it use?\n21118 is the default port. [[openjevx:README.md:2-2]]\n\n## Licence\nApache-2.0, not MIT.\n")
	writeFile(t, filepath.Join(dir, "services.md"), "# Mission: services\n\nFind and email services leads daily. Touch 2 is due on day 3.\n")
	writeFile(t, filepath.Join(dir, "cooking.md"), "# Cooking\n\nBoil pasta for nine minutes in salted water.\n")
	writeFile(t, filepath.Join(dir, "notices.md"), "# Notices\n\n## Usage throttle until 20:30\nOnly work that is already due; stop when the due item is done.\n")
	m := &Memory{Name: "t", Dir: dir, Pins: []string{"notices.md"}, Repos: map[string]string{"openjevx": repo}}
	if err := m.Index(); err != nil {
		t.Fatal(err)
	}
	return m, repo
}

func TestSectionsKeepHeadingsAndLineRanges(t *testing.T) {
	md := "# Title\nintro\n\n## Port\n21118\n```sh\n# not a heading\n```\n## Next\nx\n"
	got := Sections("p.md", md, 900)
	if len(got) != 3 {
		t.Fatalf("want 3 sections, got %d: %+v", len(got), got)
	}
	if got[1].Head != "Port" || got[1].Start != 5 || got[1].End != 8 || !strings.Contains(got[1].Text, "# not a heading") {
		t.Fatalf("a fenced # line stays in its section, with its line range: %+v", got[1])
	}
	long := "# H\n" + strings.Repeat("word word word word\n", 200)
	for _, s := range Sections("l.md", long, 900) {
		if len(s.Text) > 920 || s.Start > s.End {
			t.Fatalf("long bodies are cut near maxc with valid ranges: len %d, L%d-L%d", len(s.Text), s.Start, s.End)
		}
	}
}

func TestBM25RanksTheMatchingSectionFirst(t *testing.T) {
	m, _ := testMemory(t)
	hits := m.Retrieve("Which port does the server listen on by default?", 1, 2000)
	var bm []MemHit
	for _, h := range hits {
		if h.Why == "bm25" {
			bm = append(bm, h)
		}
	}
	if len(bm) != 1 || bm[0].Page != "faq.md" || bm[0].Head != "What port does it use?" {
		t.Fatalf("BM25 top hit: %+v", hits)
	}
	if got := MemTokens("Licences licence ports classes the a of"); strings.Join(got, " ") != "licence licence port class" {
		t.Fatalf("stemming and stopwords: %v", got)
	}
}

func TestNamedPageThenPinnedThenBM25(t *testing.T) {
	m, _ := testMemory(t)
	hits := m.Retrieve("services. Last 30 minutes: no output, waiting", 2, 2000)
	if len(hits) < 2 || hits[0].Page != "services.md" || hits[0].Why != "named" || hits[1].Page != "notices.md" || hits[1].Why != "pinned" {
		t.Fatalf("named page first, pinned notice second: %+v", hits)
	}
	for _, h := range hits {
		if h.Page == "cooking.md" {
			t.Fatalf("an unrelated page must not be retrieved: %+v", hits)
		}
	}
}

func TestBudgetCapsTheNotes(t *testing.T) {
	m, _ := testMemory(t)
	for _, budget := range []int{100, 300} {
		total := 0
		for _, h := range m.Retrieve("services port licence throttle", 5, budget) {
			total += len(h.text) + 1
		}
		if total > budget+1 {
			t.Fatalf("budget %d: notes use %d chars", budget, total)
		}
	}
}

func TestItemComesFirstAndCitationsSurvive(t *testing.T) {
	m, _ := testMemory(t)
	item := "Claim: OpenJevX listens on port 21118."
	hits := m.Retrieve(item, 2, 2000)
	s := MemoryState(item, hits)
	if !strings.HasPrefix(s, item) || !strings.Contains(s, "[faq.md#What port does it use?]") {
		t.Fatalf("item first, then cited notes: %q", s)
	}
	b, _ := json.Marshal(hits)
	if !strings.Contains(string(b), `"lines":"L4-L4"`) || !strings.Contains(string(b), `"page":"faq.md"`) {
		t.Fatalf("--json carries page and line range: %s", b)
	}
	js := MemoryState(`{"claim":"port 21118","id":7}`, hits)
	var obj map[string]any
	if json.Unmarshal([]byte(js), &obj) != nil || obj["claim"] != "port 21118" || !strings.HasPrefix(js, `{"claim":"port 21118","id":7,"memory":`) {
		t.Fatalf("a JSON state keeps its fields first and gains a last memory field: %s", js)
	}
	if MemoryState(item, nil) != item {
		t.Fatal("no hits: the state is unchanged")
	}
}

func TestCheckMarksStalePagesAndRetrievalSkipsThem(t *testing.T) {
	m, repo := testMemory(t)
	if s := m.Check(); len(s) != 0 {
		t.Fatalf("fresh memory: %v", s)
	}
	writeFile(t, filepath.Join(repo, "README.md"), "line one\nDefault port: 21200\nline three\n")
	s := m.Check()
	if !strings.Contains(s["faq.md"], "openjevx:README.md:2-2 changed") || len(s) != 1 {
		t.Fatalf("a changed cited line marks its page stale: %v", s)
	}
	for _, h := range m.Retrieve("Which port does the server use?", 3, 2000) {
		if h.Page == "faq.md" {
			t.Fatalf("stale pages are not retrieved: %+v", h)
		}
	}
	writeFile(t, filepath.Join(m.Dir, "services.md"), "# Mission: services\n\nchanged\n")
	if !strings.Contains(m.Check()["services.md"], "page changed") {
		t.Fatal("an edited page is stale until re-indexed")
	}
}

func TestSaveLoadAndNames(t *testing.T) {
	m, _ := testMemory(t)
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := LoadMemory("t")
	if err != nil || len(got.Sections) != len(m.Sections) || got.Pins[0] != "notices.md" {
		t.Fatalf("round trip: %v %+v", err, got)
	}
	if st, _ := os.Stat(filepath.Join(MemoryDir(), "t.json")); st.Mode().Perm() != 0o600 {
		t.Fatalf("the index is private: %v", st.Mode())
	}
	if _, err := LoadMemory("../x"); err == nil {
		t.Fatal("a path in the name is refused")
	}
	if ls := ListMemories(); len(ls) != 1 || ls[0] != "t" {
		t.Fatalf("list: %v", ls)
	}
}
