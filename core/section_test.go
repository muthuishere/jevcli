package core

import "testing"

func TestSection(t *testing.T) {
	md := "# Repo\n\nintro\n\n## Jev context\n\nPayments service.\n\n```bash\n# not a heading\n```\n\n### detail\nkept\n\n## Build\nno\n"
	got, ok := Section(md, "Jev")
	if !ok || got != "Payments service.\n\n```bash\n# not a heading\n```\n\n### detail\nkept" {
		t.Fatalf("Section: ok=%v %q", ok, got)
	}
	if _, ok := Section(md, "Jevons"); ok {
		t.Fatal("a different heading must not match")
	}
	if got, _ := Section("### Decision model\nx\n## y", "decision model"); got != "x" {
		t.Fatalf("custom name, any level, any case: %q", got)
	}
}
