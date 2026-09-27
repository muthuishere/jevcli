// jevcli: ask any System One (Jev-contract) endpoint what the user would decide, and wire it into agents.
//
//	jevcli ask QUESTION [--context TEXT|-] [--option KEY=DESC ... | --true DESC --false DESC] [--profile P] [--json]
//	jevcli judge --request TEXT --proposal TEXT [--action LINE ...] [--profile P] [--json]
//	jevcli install [--no-skill] [--no-hook]     skill into Claude Code + Codex, Stop-hook template (inert until enabled)
//	jevcli uninstall                            remove the skill links and the hook template
//	jevcli hook enable|disable stop | mode shadow|block | profile NAME | status | review [N] | run stop
//	jevcli config show | set-endpoint PROFILE URL [MODEL] [KEY_ENV] | default PROFILE
package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/muthuishere/jevcli/core"
)

//go:embed skill.md
var skillMD []byte

func die(f string, a ...any) { fmt.Fprintf(os.Stderr, "jevcli: "+f+"\n", a...); os.Exit(1) }

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

// ask posts and, when the profile's key is not in the environment, re-executes jevcli under `sec run <KEY> --` so the key
// is injected into this process only (never in argv, files or output).
func ask(name string, p core.Profile, state string, qs map[string]any) map[string]core.Answer {
	ans, err := core.Ask(p, state, qs, 60*time.Second)
	if errors.Is(err, core.ErrNeedKey) {
		if os.Getenv("JEVCLI_SEC_WRAPPED") != "" {
			die("profile %s needs %s and `sec` did not provide it: add it with `sec set %s`", name, p.KeyEnv, p.KeyEnv)
		}
		self, _ := os.Executable()
		sec, lerr := exec.LookPath("sec")
		if lerr != nil {
			die("profile %s needs %s in the environment (or the `sec` CLI to inject it)", name, p.KeyEnv)
		}
		env := append(os.Environ(), "JEVCLI_SEC_WRAPPED=1")
		_ = syscall.Exec(sec, append([]string{"sec", "run", p.KeyEnv, "--", self}, os.Args[1:]...), env)
		die("could not run sec")
	}
	if err != nil {
		die("%v", err)
	}
	return ans
}

func pr(f string, a ...any) { fmt.Printf(f+"\n", a...) }

func cmdAsk(args []string) {
	fs := flag.NewFlagSet("ask", flag.ExitOnError)
	ctx := fs.String("context", "", "facts (- reads stdin)")
	prof := fs.String("profile", "", "endpoint profile")
	js := fs.Bool("json", false, "print the raw answer")
	tru := fs.String("true", "", "yes/no: what the true answer means (the question's own context)")
	fal := fs.String("false", "", "yes/no: what the false answer means")
	var opts multi
	fs.Var(&opts, "option", "KEY=DESCRIPTION (repeat; none = yes/no)")
	// the question may come before, between or after the flags
	q, rest := firstPositional(args)
	_ = fs.Parse(rest)
	for fs.NArg() > 0 {
		if q == "" {
			q = fs.Arg(0)
		}
		_ = fs.Parse(fs.Args()[1:])
	}
	if q == "" {
		die("usage: jevcli ask QUESTION [--context TEXT] [--option KEY=DESC ...]")
	}
	cfg := core.LoadConfig()
	name, p, err := cfg.Profile(*prof)
	if err != nil {
		die("%v", err)
	}
	c := *ctx
	if c == "-" {
		b, _ := io.ReadAll(os.Stdin)
		c = string(b)
	}
	state := core.Redact(strings.TrimSpace(c))
	if state == "" {
		state = q
	}
	var qq map[string]any
	if len(opts) > 0 {
		crit := map[string]string{}
		for _, o := range opts {
			k, v, ok := strings.Cut(o, "=")
			if !ok {
				v = k
			}
			crit[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
		if len(crit) < 2 {
			die("give at least two --option KEY=DESCRIPTION")
		}
		qq = map[string]any{"type": "choice", "instructions": q, "criteria": crit}
	} else {
		// a noul is instructions + criteria that carry the context; without --true/--false it is a bare instruction
		crit := map[string]string{}
		if *tru != "" {
			crit["true"] = *tru
		}
		if *fal != "" {
			crit["false"] = *fal
		}
		qq = map[string]any{"type": "noul", "instructions": q}
		if len(crit) > 0 {
			qq["criteria"] = crit
		}
	}
	a := ask(name, p, state, map[string]any{"q": qq})["q"]
	if *js {
		b, _ := json.Marshal(a)
		pr("%s", b)
		return
	}
	if len(opts) > 0 {
		conf := 0.0
		if a.Confidence != nil {
			conf = *a.Confidence
		}
		pr("%s  (confidence %.2f, %s)", a.Choice, conf, name)
		type kv struct {
			k string
			v float64
		}
		var l []kv
		for k, v := range a.Probabilities {
			l = append(l, kv{k, v})
		}
		sort.Slice(l, func(i, j int) bool { return l[i].v > l[j].v })
		for _, x := range l {
			pr("  %.2f  %s", x.v, x.k)
		}
		return
	}
	verdict := "false"
	if a.P() >= 0.5 {
		verdict = "true"
	}
	pr("%s  (P(true) %.2f, %s)", verdict, a.P(), name)
}

// firstPositional lets the question come before the flags, as in `jevcli ask "Q" --option ...`.
func firstPositional(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

func cmdJudge(args []string) {
	fs := flag.NewFlagSet("judge", flag.ExitOnError)
	reqT := fs.String("request", "", "the user's request")
	prop := fs.String("proposal", "", "the agent's final message")
	prof := fs.String("profile", "", "endpoint profile")
	js := fs.Bool("json", false, "raw answers")
	var acts multi
	fs.Var(&acts, "action", "one tool call as a short line (repeat)")
	_ = fs.Parse(args)
	if *reqT == "" || *prop == "" {
		die("usage: jevcli judge --request TEXT --proposal TEXT [--action LINE ...]")
	}
	cfg := core.LoadConfig()
	name, p, err := cfg.Profile(*prof)
	if err != nil {
		die("%v", err)
	}
	var a2 []string
	for _, a := range acts {
		a2 = append(a2, core.Redact(a))
	}
	st := core.State(core.Redact(*reqT), core.Redact(*prop), a2)
	ans := ask(name, p, st, map[string]any{"accepts": core.NoulQ(p, "accepts"), "wanted_more": core.NoulQ(p, "wanted_more"),
		"reaction": core.ChoiceQ(p, "reaction"), "satisfaction": core.ScoreQ(p, "satisfaction")})
	if *js {
		b, _ := json.Marshal(ans)
		pr("%s", b)
		return
	}
	r, s := ans["reaction"], ans["satisfaction"]
	rp, rc, sv := 0.0, 0.0, 0.0
	if r.Probabilities != nil {
		rp = r.Probabilities[r.Choice]
	}
	if r.Confidence != nil {
		rc = *r.Confidence
	}
	if s.Score != nil {
		sv = *s.Score
	}
	pr("accept        %.2f\nwanted more   %.2f\nreaction      %s (%.2f, confidence %.2f)\nsatisfaction  %.1f / 4  (%s)",
		ans["accepts"].P(), ans["wanted_more"].P(), r.Choice, rp, rc, sv, name)
}

// ------------------------------------------------------------------ install: skill + hook template

func skillDirs() []string {
	h := core.Home()
	var out []string
	for _, d := range []string{filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "skills"), filepath.Join(h, ".claude/skills"), filepath.Join(h, ".codex/skills")} {
		if strings.HasPrefix(d, "skills") { // CLAUDE_CONFIG_DIR unset
			continue
		}
		if r, err := filepath.EvalSymlinks(d); err == nil {
			d = r
		}
		if st, err := os.Stat(d); err == nil && st.IsDir() && !contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func cmdInstall(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	noSkill := fs.Bool("no-skill", false, "do not write the agent skill")
	noHook := fs.Bool("no-hook", false, "do not add the Stop-hook template")
	_ = fs.Parse(args)
	cfg := core.LoadConfig()
	if _, err := os.Stat(core.ConfigPath()); err != nil {
		if err := core.SaveConfig(cfg); err != nil {
			die("%v", err)
		}
		pr("config   %s (hooks disabled)", core.ConfigPath())
	}
	if !*noSkill {
		for _, d := range skillDirs() {
			dst := filepath.Join(d, "jevcli")
			_ = os.MkdirAll(dst, 0o755)
			if err := os.WriteFile(filepath.Join(dst, "SKILL.md"), skillMD, 0o644); err != nil {
				die("%v", err)
			}
			pr("skill    %s", dst)
		}
	}
	if !*noHook {
		added, err := core.InstallHookTemplate(core.SettingsPath())
		if err != nil {
			die("%v", err)
		}
		state := "already present"
		if added {
			state = "added (backup beside it)"
		}
		h := cfg.Hooks["stop"]
		pr("hook     Stop template %s in %s; enabled=%v (turn on: jevcli hook enable stop)", state, core.SettingsPath(), h.Enabled)
	}
}

func cmdUninstall() {
	for _, d := range skillDirs() {
		if _, err := os.Stat(filepath.Join(d, "jevcli", "SKILL.md")); err == nil {
			_ = os.RemoveAll(filepath.Join(d, "jevcli"))
			pr("skill removed  %s", filepath.Join(d, "jevcli"))
		}
	}
	n, err := core.RemoveHookTemplate(core.SettingsPath())
	if err != nil {
		die("%v", err)
	}
	pr("hook entries removed: %d (other hooks untouched)", n)
}

// ------------------------------------------------------------------ hooks

func cmdHook(args []string) {
	if len(args) == 0 {
		die("usage: jevcli hook enable|disable stop | mode shadow|block | profile NAME | status | review [N] | run stop")
	}
	cfg := core.LoadConfig()
	h := cfg.Hooks["stop"]
	save := func() {
		cfg.Hooks["stop"] = h
		if err := core.SaveConfig(cfg); err != nil {
			die("%v", err)
		}
	}
	switch args[0] {
	case "enable":
		h.Enabled = true
		save()
		_, p, _ := cfg.Profile(h.Profile)
		warn := ""
		if p.KeyEnv != "" {
			warn = "  WARNING: profile " + h.Profile + " uses an API key: every agent turn is sent to that endpoint (and may be billed)."
		}
		pr("stop hook enabled (mode %s, profile %s)%s", h.Mode, h.Profile, warn)
		if !core.HookInstalled(core.SettingsPath()) {
			pr("note: the hook template is not in %s yet: run `jevcli install`", core.SettingsPath())
		}
	case "disable":
		h.Enabled = false
		save()
		pr("stop hook disabled (template stays installed, inert)")
	case "mode":
		if len(args) < 2 || (args[1] != "shadow" && args[1] != "block") {
			die("usage: jevcli hook mode shadow|block")
		}
		h.Mode = args[1]
		save()
		pr("mode %s", h.Mode)
	case "profile":
		if len(args) < 2 {
			die("usage: jevcli hook profile NAME")
		}
		if _, _, err := cfg.Profile(args[1]); err != nil {
			die("%v", err)
		}
		h.Profile = args[1]
		save()
		pr("hook profile %s", h.Profile)
	case "status":
		pr("template  %v in %s", core.HookInstalled(core.SettingsPath()), core.SettingsPath())
		pr("enabled   %v\nmode      %s\nprofile   %s", h.Enabled, h.Mode, h.Profile)
		scored, would, errs := 0, 0, 0
		if b, err := os.ReadFile(filepath.Join(core.DataDir(), "verdicts.jsonl")); err == nil {
			day := time.Now().Add(-24 * time.Hour).Format("2006-01-02T15:04:05")
			for _, l := range strings.Split(string(b), "\n") {
				var r map[string]any
				if json.Unmarshal([]byte(l), &r) != nil || fmt.Sprint(r["ts"]) < day {
					continue
				}
				if _, ok := r["accept"]; ok {
					scored++
				}
				if r["would_block"] == true {
					would++
				}
				if _, ok := r["error"]; ok {
					errs++
				}
			}
		}
		pr("24 h      %d scored, %d would block, %d errors", scored, would, errs)
	case "review":
		cmdReview(args[1:])
	case "run":
		hookRun(cfg, h, args[1:])
	default:
		die("unknown hook command %q", args[0])
	}
}

// hookRun is what the agent's Stop hook executes. Disabled: exit at once. Shadow: hand the input to a detached copy and
// return (the agent waits ~ms). Block: score synchronously and print {"decision":"block",...} when warranted. Fails open.
func hookRun(cfg core.Config, h core.HookCfg, args []string) {
	if !h.Enabled {
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}
	raw, _ := io.ReadAll(os.Stdin)
	if len(args) > 1 && args[1] == "--scored" { // the detached shadow worker
		var in core.HookInput
		if json.Unmarshal(raw, &in) == nil {
			if rec, _ := core.Verdict(cfg, h, in); rec != nil {
				core.AppendLog(rec)
			}
		}
		return
	}
	if h.Mode != "block" {
		f, err := os.CreateTemp("", "jevcli-hook-*.json")
		if err == nil {
			_, _ = f.Write(raw)
			f.Close()
			_ = core.Detach([]string{"hook", "run", "stop", "--scored"}, f.Name())
			go func() { time.Sleep(3 * time.Second); os.Remove(f.Name()) }()
			time.Sleep(50 * time.Millisecond)
		}
		return
	}
	var in core.HookInput
	if json.Unmarshal(raw, &in) != nil {
		return
	}
	rec, reason := core.Verdict(cfg, h, in)
	if rec != nil {
		core.AppendLog(rec)
	}
	if reason != "" {
		b, _ := json.Marshal(map[string]string{"decision": "block", "reason": reason})
		pr("%s", b)
	}
}

func cmdReview(args []string) {
	n := 40
	if len(args) > 0 {
		fmt.Sscan(args[0], &n)
	}
	b, err := os.ReadFile(filepath.Join(core.DataDir(), "verdicts.jsonl"))
	if err != nil {
		die("no verdicts yet (%v)", err)
	}
	var rows []map[string]any
	for _, l := range strings.Split(string(b), "\n") {
		var r map[string]any
		if json.Unmarshal([]byte(l), &r) == nil && r["accept"] != nil {
			rows = append(rows, r)
		}
	}
	if len(rows) > n {
		rows = rows[len(rows)-n:]
	}
	would := 0
	for _, r := range rows {
		if r["would_block"] == true {
			would++
		}
	}
	pr("%d scored stops, %d would have been blocked\n", len(rows), would)
	for _, r := range rows {
		st, _ := r["state"].(string)
		req := strings.TrimPrefix(strings.SplitN(st, "\n", 2)[0], "Request: ")
		nxt := nextUserMessage(fmt.Sprint(r["transcript"]), req)
		flag := "           "
		if r["would_block"] == true {
			flag = "WOULD-BLOCK"
		}
		pr("%s acc %.2f more %.2f %s | req: %q\n%44s next: %q", fmt.Sprint(r["ts"])[5:16], r["accept"], r["wanted_more"], flag, trunc(req, 40), "", trunc(nxt, 140))
	}
}

func trunc(s string, n int) string {
	if len([]rune(s)) > n {
		return string([]rune(s)[:n])
	}
	return s
}

// nextUserMessage finds the user's first message after the scored request in that transcript.
func nextUserMessage(path, reqPrefix string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "(no transcript)"
	}
	seen := false
	for _, l := range strings.Split(string(b), "\n") {
		var r struct {
			Type    string `json:"type"`
			IsMeta  bool   `json:"isMeta"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(l), &r) != nil || r.Type != "user" || r.IsMeta {
			continue
		}
		var s string
		if json.Unmarshal(r.Message.Content, &s) != nil {
			var bl []map[string]any
			_ = json.Unmarshal(r.Message.Content, &bl)
			human := true
			var parts []string
			for _, x := range bl {
				if x["type"] == "tool_result" {
					human = false
				}
				if x["type"] == "text" {
					t, _ := x["text"].(string)
					parts = append(parts, t)
				}
			}
			if !human {
				continue
			}
			s = strings.Join(parts, "\n")
		}
		if strings.HasPrefix(strings.TrimSpace(s), "<") {
			continue
		}
		if seen {
			return s
		}
		if strings.HasPrefix(strings.TrimSpace(s), strings.TrimSpace(trunc(reqPrefix, 30))) {
			seen = true
		}
	}
	return "(no reply yet)"
}

// ------------------------------------------------------------------ config

func cmdConfig(args []string) {
	cfg := core.LoadConfig()
	if len(args) == 0 || args[0] == "show" {
		b, _ := json.MarshalIndent(cfg, "", "  ")
		pr("# %s\n%s", core.ConfigPath(), b)
		return
	}
	switch args[0] {
	case "set-endpoint":
		if len(args) < 3 {
			die("usage: jevcli config set-endpoint PROFILE URL [MODEL] [KEY_ENV]")
		}
		p := cfg.Profiles[args[1]]
		p.Endpoint = args[2]
		if len(args) > 3 {
			p.Model = args[3]
		}
		if len(args) > 4 {
			p.KeyEnv = args[4]
		}
		if p.Model == "" {
			p.Model = "default"
		}
		cfg.Profiles[args[1]] = p
	case "default":
		if len(args) < 2 {
			die("usage: jevcli config default PROFILE")
		}
		if _, _, err := cfg.Profile(args[1]); err != nil {
			die("%v", err)
		}
		cfg.Default = args[1]
	default:
		die("usage: jevcli config show | set-endpoint PROFILE URL [MODEL] [KEY_ENV] | default PROFILE")
	}
	if err := core.SaveConfig(cfg); err != nil {
		die("%v", err)
	}
	pr("saved %s", core.ConfigPath())
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: jevcli ask|judge|install|uninstall|hook|config  (see the header of cmd/jevcli/main.go)")
		os.Exit(2)
	}
	a := os.Args[2:]
	switch os.Args[1] {
	case "ask":
		cmdAsk(a)
	case "judge":
		cmdJudge(a)
	case "install":
		cmdInstall(a)
	case "uninstall":
		cmdUninstall()
	case "hook":
		cmdHook(a)
	case "config":
		cmdConfig(a)
	default:
		die("unknown command %q", os.Args[1])
	}
}
