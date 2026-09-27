package core

import (
	"strings"
	"testing"
)

func TestEval(t *testing.T) {
	ans := map[string]Answer{"destroys": {Type: "noul", Noul: fp(0.9)}, "remote": {Type: "noul", Noul: fp(0.2)},
		"team": {Type: "choice", Choice: "billing", Confidence: fp(0.7)}, "sev": {Type: "score", Score: fp(1.6)}}
	cases := map[string]bool{"": true, "*": true, "destroys >= 0.8": true, "destroys < 0.8": false,
		"destroys >= 0.8 && remote >= 0.5": false, "destroys >= 0.8 && remote >= 0.5 || sev > 1": true,
		"team == billing": true, "team != billing": false, "team >= 0.6": true, "sev <= 1": false}
	for c, want := range cases {
		if got, err := Eval(c, ans); err != nil || got != want {
			t.Errorf("%q: got %v, %v; want %v", c, got, err, want)
		}
	}
	if _, err := Eval("nope >= 1", ans); err == nil || !strings.Contains(err.Error(), "not asked") {
		t.Errorf("unknown name must error: %v", err)
	}
	if _, err := Eval("destroys is high", ans); err == nil {
		t.Error("bad syntax must error")
	}
}

func TestOutput(t *testing.T) {
	deny := Decision{Action: "deny", Reason: "r1"}
	warn := Decision{Action: "warn", Reason: "r2"}
	ctx := Decision{Action: "context", Reason: "c"}
	if o := Output("PreToolUse", []Decision{warn, deny}, false); !strings.Contains(o, `"permissionDecision":"deny"`) {
		t.Errorf("deny wins: %s", o)
	}
	if o := Output("PreToolUse", []Decision{warn}, false); !strings.Contains(o, `"permissionDecision":"ask"`) {
		t.Errorf("warn asks: %s", o)
	}
	if o := Output("PreToolUse", []Decision{{Action: "allow"}}, false); o != "" {
		t.Errorf("allow is silent: %s", o)
	}
	if o := Output("Stop", []Decision{{Action: "block", Reason: "b"}}, true); o != "" {
		t.Errorf("stop_hook_active must not block again: %s", o)
	}
	if o := Output("UserPromptSubmit", []Decision{ctx, warn}, false); !strings.Contains(o, `"additionalContext":"c\nWarning: r2"`) {
		t.Errorf("contexts join: %s", o)
	}
	if o := Output("PreToolUse", []Decision{{Action: "exec", Raw: `{"x":1}`}}, false); o != `{"x":1}` {
		t.Errorf("exec output is passed through: %s", o)
	}
}

func TestPluginMatchAndState(t *testing.T) {
	p := Plugin{On: "PreToolUse:Bash|Write"}
	if !p.Matches("PreToolUse", Payload{"tool_name": "Write"}) || p.Matches("PreToolUse", Payload{"tool_name": "Read"}) || p.Matches("Stop", Payload{}) {
		t.Error("tool regex / event matching")
	}
	st, skip := StateOf("PreToolUse", Payload{"tool_name": "Bash", "tool_input": map[string]any{"command": "rm -rf x"}, "cwd": "/r"}, "")
	if skip != "" || !strings.Contains(st, "rm -rf x") || !strings.Contains(st, "/r") {
		t.Errorf("PreToolUse state: %q %q", st, skip)
	}
	if got := fill("kind={{kind}} p={{d}}", map[string]Answer{"kind": {Type: "choice", Choice: "ops"}, "d": {Type: "noul", Noul: fp(0.5)}}); got != "kind=ops p=0.50" {
		t.Errorf("fill: %q", got)
	}
}
