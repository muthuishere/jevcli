package core

import "testing"

func TestSkipTurn(t *testing.T) {
	cases := []struct {
		acts []string
		skip bool
	}{
		{nil, true},
		{[]string{"Bash: ls", "Read x"}, true},
		{[]string{"Edit a.go: x"}, false},
		{[]string{"Edit a.go: x", "Bash: go test ./..."}, true},
		{[]string{"Bash: go test ./...", "Write b.go: y"}, false},
		{[]string{"Edit a.go: x", "Bash: git status"}, false},
	}
	for _, c := range cases {
		if got := skipTurn(c.acts) != ""; got != c.skip {
			t.Errorf("%v: skip=%v, want %v", c.acts, got, c.skip)
		}
	}
}
