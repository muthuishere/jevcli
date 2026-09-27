// jevcli: ask any System One (Jev-contract) endpoint what the user would decide, and wire it into agents.
//
//	jevcli ask QUESTION [--context TEXT|-] [--option KEY=DESC ... | --true DESC --false DESC] [--profile P] [--json]
//	jevcli query --state TEXT|JSON|@file|- --noul NAME=INSTRUCTIONS ... [--choice NAME="INSTR|k=desc;k2=desc"] [--score NAME="INSTR|L0;L1;L2"] [--raw]
//	jevcli judge --request TEXT --proposal TEXT [--action LINE ...] [--profile P] [--json]
//	jevcli install [--no-skill] [--no-hook]     skill into Claude Code + Codex, Stop-hook template (inert until enabled)
//	jevcli uninstall                            remove the skill links and the hook template
//	jevcli hook enable|disable stop | mode shadow|block | profile NAME | status | review [N] | run stop
//	jevcli config show | set-endpoint PROFILE URL [MODEL] | default PROFILE
//	jevcli profile list | add NAME URL [--model M] [--header 'K: V' ...] [--questions FILE] [--context TEXT] | use NAME | remove NAME | show NAME
package main

import (
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"sync"

	"path/filepath"
	"sort"
	"strings"

	"time"

	"github.com/muthuishere/jevcli/core"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

//go:embed skill
var skillFS embed.FS

// die exits 4: an error is never a "no" (1) or "unsure" (3), so `if jevcli is ...` cannot mistake an outage for an answer.
func die(f string, a ...any) { fmt.Fprintf(os.Stderr, "jevcli: "+f+"\n", a...); os.Exit(4) }

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

// ask posts one System One request; a profile's API key comes from the environment variable it names.
func ask(name string, p core.Profile, state string, qs map[string]any) map[string]core.Answer {
	ans, err := core.Ask(p, core.LoadConfig().WithContext(p, state), qs, 60*time.Second)
	if errors.Is(err, core.ErrNeedKey) {
		die("profile %s needs %s in the environment (the profile references it): export %s=...", name, core.MissingEnv(p), core.MissingEnv(p))
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
	route := fs.String("route", "", "ACT,CONFIRM thresholds: prints act|confirm|escalate and exits 0|10|20 (confidence-gated routing)")
	samples := fs.Int("samples", 0, "self-consistency: ask N times with shuffled option order, report agreement")
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
	if *samples > 1 && len(opts) > 0 {
		selfConsistency(name, p, state, qq, *samples)
		return
	}
	a := ask(name, p, state, map[string]any{"q": qq})["q"]
	if *route != "" { // act when confident, confirm in the middle band, escalate below
		var actT, confT float64
		if _, err := fmt.Sscanf(*route, "%g,%g", &actT, &confT); err != nil {
			die("--route ACT,CONFIRM, e.g. 0.8,0.5")
		}
		c := a.P()
		label := a.Choice
		if len(opts) > 0 {
			c = conf(a)
		} else if c < 0.5 {
			c, label = 1-c, "false"
		} else {
			label = "true"
		}
		switch {
		case c >= actT:
			pr("act       %s  (%.2f >= %.2f, %s)", label, c, actT, name)
		case c >= confT:
			pr("confirm   %s  (%.2f, %s)", label, c, name)
			os.Exit(10)
		default:
			pr("escalate  %s  (%.2f < %.2f, %s)", label, c, confT, name)
			os.Exit(20)
		}
		return
	}
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

// skillDirs are the global skill dirs: Claude Code (~/.claude, $CLAUDE_CONFIG_DIR) and the cross-agent ~/.agents are
// created if missing; Codex only if ~/.codex exists.
func skillDirs() []string {
	h := core.Home()
	var out []string
	for _, d := range []string{filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "skills"), filepath.Join(h, ".claude/skills"), filepath.Join(h, ".agents/skills"), filepath.Join(h, ".codex/skills")} {
		if strings.HasPrefix(d, "skills") { // CLAUDE_CONFIG_DIR unset
			continue
		}
		if strings.HasSuffix(d, filepath.Join(".codex", "skills")) {
			if _, err := os.Stat(filepath.Dir(d)); err != nil {
				continue
			}
		}
		_ = os.MkdirAll(d, 0o755)
		if r, err := filepath.EvalSymlinks(d); err == nil {
			d = r
		}
		if !contains(out, d) {
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
			_ = os.RemoveAll(dst) // a reinstall leaves no stale files from an older skill
			err := iofs.WalkDir(skillFS, "skill", func(p string, e iofs.DirEntry, _ error) error {
				out := filepath.Join(dst, strings.TrimPrefix(strings.TrimPrefix(p, "skill"), "/"))
				if e.IsDir() {
					return os.MkdirAll(out, 0o755)
				}
				b, _ := skillFS.ReadFile(p)
				return os.WriteFile(out, b, 0o644)
			})
			if err != nil {
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
		if len(p.Headers) > 0 {
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
			die("usage: jevcli config set-endpoint PROFILE URL [MODEL]  (headers: jevcli profile add … --header 'K: V')")
		}
		p := cfg.Profiles[args[1]]
		p.URL = args[2]
		if len(args) > 3 {
			p.Model = args[3]
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
		fmt.Fprintln(os.Stderr, "usage: jevcli is|which|ask|judge|profile|install|uninstall|hook|skill|version")
		os.Exit(2)
	}
	a := os.Args[2:]
	switch os.Args[1] {
	case "ask":
		if hasAny(a, "--option", "-option", "--true", "--route", "--samples") { // the older options form
			cmdAsk(a)
		} else {
			cmdQuery(a)
		}
	case "is":
		recipe("feels", a)
	case "which":
		recipe("match", a)
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
	case "stats":
		cmdStats(a)
	case "version", "--version", "-v":
		pr("jevcli %s", version)
	case "skill", "cookbook":
		b, _ := skillFS.ReadFile("skill/SKILL.md")
		os.Stdout.Write(b)
	case "query", "q":
		cmdQuery(a)
	case "profile", "profiles":
		cmdProfile(a)
	case "feels", "match", "verify", "same", "rank", "find", "extract", "tree", "pick-skill", "pick-func", "run", "score":
		recipe(os.Args[1], a)
	default:
		die("unknown command %q", os.Args[1])
	}
}

func cmdProfile(args []string) {
	cfg := core.LoadConfig()
	if len(args) == 0 || args[0] == "list" {
		for _, n := range keys(cfg.Profiles) {
			p := cfg.Profiles[n]
			mark := " "
			if n == cfg.Default {
				mark = "*"
			}
			hs := ""
			for _, h := range keys(p.Headers) {
				hs += "  " + h + ": " + p.Headers[h]
			}
			pr("%s %-10s %s  model=%s%s", mark, n, p.URL, p.Model, hs)
		}
		pr("(* = default; change with: jevcli profile use NAME)")
		return
	}
	need := func(n int, u string) {
		if len(args) < n {
			die("usage: jevcli profile %s", u)
		}
	}
	switch args[0] {
	case "add":
		need(3, "add NAME URL [--model M] [--header 'K: V' ...] [--questions FILE] [--context TEXT]")
		fs := flag.NewFlagSet("profile add", flag.ExitOnError)
		m, q, c := fs.String("model", "default", "model name"), fs.String("questions", "", "question pack JSON"), fs.String("context", "", "standing context for this profile")
		var hdr multi
		fs.Var(&hdr, "header", `request header "Name: value" (repeat); reference env vars for secrets: "Authorization: Bearer $JEV_API_KEY"`)
		_ = fs.Parse(args[3:])
		hm := map[string]string{}
		for _, h := range hdr {
			k, v, ok := strings.Cut(h, ":")
			if !ok {
				die("header %q: want \"Name: value\"", h)
			}
			hm[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
		cfg.Profiles[args[1]] = core.Profile{URL: args[2], Model: *m, Headers: hm, Questions: *q, Context: *c}
		if ph, ok := cfg.Profiles["default"]; ok && ph.URL == "" && args[1] != "default" {
			delete(cfg.Profiles, "default") // the seeded placeholder is replaced by the first real profile
		}
		if cfg.Default == "" || cfg.Profiles[cfg.Default].URL == "" {
			cfg.Default = args[1]
		}
	case "use":
		need(2, "use NAME")
		if _, ok := cfg.Profiles[args[1]]; !ok {
			die("no profile %q", args[1])
		}
		cfg.Default = args[1]
	case "remove":
		need(2, "remove NAME")
		delete(cfg.Profiles, args[1])
		if cfg.Default == args[1] {
			cfg.Default = ""
		}
	case "show":
		need(2, "show NAME")
		b, _ := json.MarshalIndent(cfg.Profiles[args[1]], "", "  ")
		pr("%s", b)
		return
	default:
		die("usage: jevcli profile list | add | use | remove | show")
	}
	if err := core.SaveConfig(cfg); err != nil {
		die("%v", err)
	}
	pr("saved %s (default: %s)", core.ConfigPath(), cfg.Default)
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// cmdQuery is the native System One call: one state (plain text or a JSON object, sent as its JSON text), many named
// questions. It prints the answers the way the API returns them; --raw prints the whole response (model, usage, id).
// batchIn holds --lines input, already turned into JSONL.
var batchIn string

func cmdQuery(args []string) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	st, prof, raw := fs.String("state", "", "the state: text, a JSON object, @file or -"), fs.String("profile", "", "endpoint profile"), fs.Bool("raw", false, "print the full response")
	states, qfile := fs.String("states", "", "batch: a JSONL file (or -) with one state per line; prints one JSONL answer line per state"), fs.String("questions", "", "a question-set file: JSON object of NAME -> {type, instructions, criteria}")
	par := fs.Int("parallel", 4, "batch: concurrent requests")
	var nouls, choices, scores multi
	fs.Var(&nouls, "noul", `NAME=INSTRUCTIONS (repeat); optional criteria: NAME="INSTR|true text|false text"`)
	fs.Var(&choices, "choice", `NAME="INSTRUCTIONS|key=desc;key2=desc2" (repeat)`)
	fs.Var(&scores, "score", `NAME="INSTRUCTIONS|level0;level1;level2" (repeat, lowest first)`)
	fs.Var(&nouls, "is", `NAME="QUESTION" (repeat): a yes/no, answered as P(yes)`)
	fs.Var(&choices, "which", `NAME="QUESTION|key=desc;key2=desc2" (repeat): pick one key`)
	fs.StringVar(st, "in", "", "one input: text, a JSON object, @file or -")
	linesF := fs.String("lines", "", "batch: a text file (or -), one input per line")
	_ = fs.Parse(args)
	if *linesF != "" {
		in := *linesF
		if in != "-" {
			in = "@" + strings.TrimPrefix(in, "@")
		}
		var b strings.Builder
		for _, l := range strings.Split(text(in), "\n") {
			if strings.TrimSpace(l) != "" {
				j, _ := json.Marshal(l)
				b.Write(append(j, '\n'))
			}
		}
		*states, *linesF = "-", ""
		batchIn = b.String()
	}
	if (*st == "") == (*states == "") || len(nouls)+len(choices)+len(scores) == 0 && *qfile == "" {
		die("usage: jevcli query --state TEXT|JSON|@file | --states FILE.jsonl  --noul NAME=INSTRUCTIONS [...] [--questions set.json]")
	}
	split := func(spec string) (string, []string) {
		name, rest, ok := strings.Cut(spec, "=")
		if !ok || strings.TrimSpace(name) == "" {
			die("want NAME=INSTRUCTIONS, got %q", spec)
		}
		return strings.TrimSpace(name), strings.Split(rest, "|")
	}
	qs := map[string]any{}
	if *qfile != "" {
		if err := json.Unmarshal([]byte(text("@"+*qfile)), &qs); err != nil {
			die("--questions %s: want a JSON object of NAME -> question: %v", *qfile, err)
		}
	}
	for _, n := range nouls {
		name, parts := split(n)
		q := map[string]any{"type": "noul", "instructions": strings.TrimSpace(parts[0])}
		if len(parts) == 3 {
			q["criteria"] = map[string]string{"true": strings.TrimSpace(parts[1]), "false": strings.TrimSpace(parts[2])}
		}
		qs[name] = q
	}
	for _, c := range choices {
		name, parts := split(c)
		if len(parts) != 2 {
			die("--choice %s: want \"INSTRUCTIONS|key=desc;key2=desc2\"", name)
		}
		crit := map[string]string{}
		for _, o := range strings.Split(parts[1], ";") {
			k, v, _ := strings.Cut(o, "=")
			if v == "" {
				v = k
			}
			crit[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
		qs[name] = map[string]any{"type": "choice", "instructions": strings.TrimSpace(parts[0]), "criteria": crit}
	}
	for _, sc := range scores {
		name, parts := split(sc)
		if len(parts) != 2 {
			die("--score %s: want \"INSTRUCTIONS|level0;level1\"", name)
		}
		var lv []string
		for _, l := range strings.Split(parts[1], ";") {
			lv = append(lv, strings.TrimSpace(l))
		}
		qs[name] = map[string]any{"type": "score", "instructions": strings.TrimSpace(parts[0]), "criteria": lv}
	}
	name, p := resolve(*prof)
	if *states != "" {
		in := *states
		if in != "-" {
			in = "@" + strings.TrimPrefix(in, "@")
		}
		if batchIn != "" {
			queryBatch(name, p, batchIn, qs, *par)
			return
		}
		queryBatch(name, p, text(in), qs, *par)
		return
	}
	if !*raw {
		j, _ := json.Marshal(text(*st))
		queryBatch(name, p, string(j), qs, 1)
		return
	}
	state, err := normState(text(*st))
	if err != nil {
		die("--state: %v", err)
	}
	ans := askMany(name, p, state, qs)
	if *raw && len(qs) <= 32 {
		var v any
		_ = json.Unmarshal(core.LastRaw, &v)
		b, _ := json.MarshalIndent(v, "", "  ")
		pr("%s", b)
		return
	}
	var model any
	if len(core.LastRaw) > 0 {
		var v map[string]any
		_ = json.Unmarshal(core.LastRaw, &v)
		model = v["model"]
	}
	b, _ := json.MarshalIndent(map[string]any{"profile": name, "model": model, "answers": ans}, "", "  ")
	pr("%s", b)
}

// normState trims a state; a structured state travels as its compact JSON text, as the API expects.
func normState(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		return s, nil
	}
	var v any
	if json.Unmarshal([]byte(s), &v) != nil {
		return "", errors.New("looks like JSON but does not parse")
	}
	b, _ := json.Marshal(v)
	return string(b), nil
}

// queryBatch asks the same questions of every state in a JSONL input (a JSON object, or a JSON string for a text state,
// per line; blank lines skipped), par requests at a time, and prints one JSONL line per state in input order. A failed
// line prints its error and the batch goes on; the exit code is 1 if any line failed.
func queryBatch(name string, p core.Profile, in string, qs map[string]any, par int) {
	type row struct {
		Line    int                    `json:"line"`
		Input   string                 `json:"input,omitempty"`
		Model   any                    `json:"model,omitempty"`
		Answers map[string]core.Answer `json:"answers,omitempty"`
		Error   string                 `json:"error,omitempty"`
	}
	var lines []string
	var nums []int
	for i, l := range strings.Split(in, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines, nums = append(lines, l), append(nums, i+1)
		}
	}
	rows := make([]row, len(lines))
	sem := make(chan struct{}, max(1, par))
	var wg sync.WaitGroup
	for i, l := range lines {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			rows[i] = row{Line: nums[i]}
			var s string
			if strings.HasPrefix(l, `"`) && json.Unmarshal([]byte(l), &s) == nil {
				l = s
			}
			rows[i].Input = trunc(l, 120)
			state, err := normState(l)
			if err == nil {
				rows[i].Answers, rows[i].Model, err = askManyErr(p, core.LoadConfig().WithContext(p, state), qs)
			}
			if errors.Is(err, core.ErrNeedKey) {
				err = fmt.Errorf("profile %s needs %s in the environment", name, core.MissingEnv(p))
			}
			if err != nil {
				rows[i].Error = err.Error()
			}
		}()
	}
	wg.Wait()
	failed := false
	for _, r := range rows {
		b, _ := json.Marshal(r)
		pr("%s", b)
		failed = failed || r.Error != ""
	}
	if failed {
		os.Exit(1)
	}
}

func hasAny(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f || strings.HasPrefix(a, f+"=") {
				return true
			}
		}
	}
	return false
}

// cmdStats summarises the call ledger: calls, errors, tokens and latency per day and host.
func cmdStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	days := fs.Int("days", 7, "how many days back")
	_ = fs.Parse(args)
	b, err := os.ReadFile(filepath.Join(core.Home(), ".local/share/jevcli/calls.jsonl"))
	if err != nil {
		die("no ledger yet (%v)", err)
	}
	type agg struct {
		calls, errs, in, out int
		ms                   []int
	}
	from := time.Now().UTC().AddDate(0, 0, -*days).Format("2006-01-02")
	m := map[string]*agg{}
	for _, l := range strings.Split(string(b), "\n") {
		var r struct {
			TS, Host, Error string
			MS              int
			Usage           map[string]float64
		}
		if json.Unmarshal([]byte(l), &r) != nil || len(r.TS) < 10 || r.TS[:10] < from {
			continue
		}
		k := r.TS[:10] + "  " + r.Host
		if m[k] == nil {
			m[k] = &agg{}
		}
		a := m[k]
		a.calls++
		a.ms = append(a.ms, r.MS)
		a.in += int(r.Usage["input_tokens"])
		a.out += int(r.Usage["output_tokens"])
		if r.Error != "" {
			a.errs++
		}
	}
	pr("%-40s %6s %6s %10s %8s %8s", "day  host", "calls", "errors", "tokens in", "out", "p50 ms")
	for _, k := range keys(m) {
		a := m[k]
		sort.Ints(a.ms)
		pr("%-40s %6d %6d %10d %8d %8d", k, a.calls, a.errs, a.in, a.out, a.ms[len(a.ms)/2])
	}
}
