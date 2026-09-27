package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithContext(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	_ = os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	_ = os.WriteFile(filepath.Join(home, ".claude/CLAUDE.md"), []byte("# me\n\n## Jev\n\nglobal\n\n## Other\nno\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	_ = os.WriteFile(filepath.Join(home, ".codex/AGENTS.md"), []byte("#jev\nglobal\n"), 0o644) // a synced copy: deduplicated
	_ = os.WriteFile(filepath.Join(repo, "AGENTS.md"), []byte("# repo\n\n### Jev notes\nfolder\n"), 0o644)
	_ = os.WriteFile(filepath.Join(repo, "CLAUDE.md"), []byte("## jev\nfolder\nclaude only\n"), 0o644)
	sub := filepath.Join(repo, "sub")
	_ = os.MkdirAll(sub, 0o755)
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd); NoContext = false })
	_ = os.Chdir(sub)

	c := Config{}
	if got := c.WithContext(Profile{}, "the text", "call"); got != "Context: global\nfolder\nclaude only\ncall\n\nthe text" {
		t.Fatalf("text state: %q", got)
	}
	if got := c.WithContext(Profile{}, `{"message":"hi","context":"own"}`); got != `{"context":"global\nfolder\nclaude only\nown","message":"hi"}` {
		t.Fatalf("JSON state must stay JSON with a merged context field: %q", got)
	}
	NoContext = true
	if got := c.WithContext(Profile{}, "x", "call"); got != "Context: call\n\nx" {
		t.Fatalf("--no-context keeps only the call's context: %q", got)
	}
	if got := c.WithContext(Profile{}, "x"); got != "x" {
		t.Fatalf("no context must leave the state alone: %q", got)
	}
	if w := (Question{Instructions: "Q?", Context: "bg"}).Wire()["instructions"]; !strings.HasPrefix(w.(string), "Context: bg\n") {
		t.Fatalf("question context: %q", w)
	}
}
