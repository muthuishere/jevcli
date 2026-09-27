package core

import (
	"strings"
	"testing"
)

func TestWithContext(t *testing.T) {
	c := Config{Context: "global"}
	p := Profile{Context: "profile"}
	if got := c.WithContext(p, "the text", "call"); got != "Context: global\nprofile\ncall\n\nthe text" {
		t.Fatalf("text state: %q", got)
	}
	got := c.WithContext(p, `{"message":"hi","context":"own"}`, "call")
	if got != `{"context":"global\nprofile\ncall\nown","message":"hi"}` {
		t.Fatalf("JSON state must stay JSON with a merged context field: %q", got)
	}
	if got := (Config{}).WithContext(Profile{}, "x"); got != "x" {
		t.Fatalf("no context must leave the state alone: %q", got)
	}
	if w := (Question{Instructions: "Q?", Context: "bg"}).Wire()["instructions"]; !strings.HasPrefix(w.(string), "Context: bg\n") {
		t.Fatalf("question context: %q", w)
	}
}
