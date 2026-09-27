// Package core is jevcli: a vendor-neutral client for any System One (Jev-contract) decision endpoint (hosted,
// self-hosted or personal; set per profile), plus agent integration: a skill and config-gated hook templates that
// `jevcli install` writes into Claude Code / Codex.
package core

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

//go:embed questions.json
var questionsJSON []byte

// Questions are the texts jevcli asks. The embedded pack is neutral ("the user"); a profile may point to its own pack (a
// model trained on specific wording should be asked in exactly that wording). Keys: noul accepts|wanted_more,
// choice reaction, score satisfaction.
type Questions struct {
	Noul   map[string]struct{ Instructions, True, False string } `json:"noul"`
	Choice map[string]struct {
		Instructions string            `json:"instructions"`
		Options      map[string]string `json:"options"`
	} `json:"choice"`
	Score map[string]struct {
		Instructions string   `json:"instructions"`
		Levels       []string `json:"levels"`
	} `json:"score"`
}

func questions(p Profile) Questions {
	p = p.Expanded()
	raw := questionsJSON
	if p.Questions != "" {
		b, err := os.ReadFile(expand(p.Questions))
		if err != nil {
			fmt.Fprintf(os.Stderr, "jevcli: question pack %s: %v (using the built-in pack)\n", p.Questions, err)
		} else {
			raw = b
		}
	}
	var q Questions
	if err := json.Unmarshal(raw, &q); err != nil {
		panic(fmt.Sprintf("question pack: %v", err))
	}
	return q
}

func expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(Home(), p[2:])
	}
	return p
}

// ---------------------------------------------------------------- config

// Profile is one endpoint: url, model and request headers. Secrets are never stored: a header value names an env var
// ("Bearer $JEV_API_KEY") that is expanded only when a request is sent.
type Profile struct {
	URL         string            `json:"url"`
	Model       string            `json:"model"`
	Headers     map[string]string `json:"headers,omitempty"`      // values may reference env vars ("Bearer $JEV_API_KEY"), expanded per request
	Questions   string            `json:"questions,omitempty"`    // optional question pack (JSON) for a model trained on specific wording
	Context     string            `json:"context,omitempty"`      // standing context prepended to every state sent to this profile
	ContextFile string            `json:"context_file,omitempty"` // same, read from a file
	Note        string            `json:"note,omitempty"`
	Endpoint    string            `json:"endpoint,omitempty"` // legacy name for url
	KeyEnv      string            `json:"key_env,omitempty"`  // legacy: same as headers {"Authorization": "Bearer $KEY_ENV"}
}

// Norm folds the legacy fields into url + headers.
func (p Profile) Norm() Profile {
	if p.URL == "" {
		p.URL = p.Endpoint
	}
	p.Endpoint = ""
	if p.KeyEnv != "" {
		if p.Headers == nil {
			p.Headers = map[string]string{}
		}
		if _, ok := p.Headers["Authorization"]; !ok {
			p.Headers["Authorization"] = "Bearer $" + p.KeyEnv
		}
		p.KeyEnv = ""
	}
	return p
}

type HookCfg struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`    // shadow (log only) | block (send the agent back)
	Profile string `json:"profile"` // which endpoint judges the turn
}

type Config struct {
	Default     string             `json:"default_profile"`
	Context     string             `json:"context,omitempty"` // standing context for every profile (before the profile's own)
	ContextFile string             `json:"context_file,omitempty"`
	Profiles    map[string]Profile `json:"profiles"`
	Hooks       map[string]HookCfg `json:"hooks"`
}

func Home() string { h, _ := os.UserHomeDir(); return h }
func ConfigPath() string {
	if p := os.Getenv("JEVCLI_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(Home(), ".config/jevcli/config.json")
}
func DataDir() string { return filepath.Join(Home(), ".local/share/jevcli") }

func DefaultConfig() Config {
	return Config{
		Default: "default",
		Profiles: map[string]Profile{
			"default": {URL: "", Model: "", Note: "set with: jevcli profile add NAME URL --model M --header 'Authorization: Bearer $VAR'"},
		},
		Hooks: map[string]HookCfg{"stop": {Enabled: false, Mode: "shadow", Profile: "default"}},
	}
}

// LoadConfig reads the user's config; the built-in profiles only seed a config that does not exist yet.
func LoadConfig() Config {
	b, err := os.ReadFile(ConfigPath())
	if err != nil {
		return DefaultConfig()
	}
	var c Config
	if json.Unmarshal(b, &c) != nil {
		fmt.Fprintf(os.Stderr, "jevcli: %s is not valid JSON; using built-in defaults\n", ConfigPath())
		return DefaultConfig()
	}
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	for k, p := range c.Profiles {
		c.Profiles[k] = p.Norm()
	}
	if c.Hooks == nil {
		c.Hooks = DefaultConfig().Hooks
	}
	return c
}

func SaveConfig(c Config) error {
	_ = os.MkdirAll(filepath.Dir(ConfigPath()), 0o700)
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(ConfigPath(), append(b, '\n'), 0o600)
}

func (c Config) Profile(name string) (string, Profile, error) {
	if name == "" {
		name = c.Default
	}
	p, ok := c.Profiles[name]
	if !ok {
		return name, p, fmt.Errorf("no profile %q (have: %s)", name, strings.Join(keys(c.Profiles), ", "))
	}
	if p.URL == "" {
		return name, p, fmt.Errorf("profile %q has no url: jevcli profile add %s URL --model M [--header 'K: V']", name, name)
	}
	return name, p, nil
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// WithContext prepends the configured standing context (global, then the profile's) to a state. Context is the user's
// own data in their config: jevcli ships none.
func (c Config) WithContext(p Profile, state string) string {
	p = p.Expanded()
	c.Context, c.ContextFile = os.ExpandEnv(c.Context), os.ExpandEnv(c.ContextFile)
	var parts []string
	for _, pair := range [][2]string{{c.Context, c.ContextFile}, {p.Context, p.ContextFile}} {
		if pair[0] != "" {
			parts = append(parts, strings.TrimSpace(pair[0]))
		}
		if pair[1] != "" {
			if b, err := os.ReadFile(expand(pair[1])); err == nil {
				parts = append(parts, strings.TrimSpace(string(b)))
			}
		}
	}
	if len(parts) == 0 {
		return state
	}
	return "Context: " + strings.Join(parts, "\n") + "\n\n" + state
}

// ---------------------------------------------------------------- System One client

type Answer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Probability   *float64           `json:"probability,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

var ErrNeedKey = errors.New("need key")

var envRef = regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?`)

// fields are every profile value that may hold $VAR references (expanded at runtime, stored raw).
func (p Profile) fields() []string {
	f := []string{p.URL, p.Model, p.Questions, p.Context, p.ContextFile}
	for _, v := range p.Headers {
		f = append(f, v)
	}
	return f
}

// MissingEnv returns the first env var any profile value references that is not set ("" = all set).
func MissingEnv(p Profile) string {
	for _, v := range p.fields() {
		for _, m := range envRef.FindAllStringSubmatch(v, -1) {
			if os.Getenv(m[1]) == "" {
				return m[1]
			}
		}
	}
	return ""
}

// Expanded returns the profile with every $VAR expanded from the environment (runtime only; never saved).
func (p Profile) Expanded() Profile {
	p = p.Norm()
	e := os.ExpandEnv
	p.URL, p.Model, p.Questions, p.Context, p.ContextFile = e(p.URL), e(p.Model), e(p.Questions), e(p.Context), e(p.ContextFile)
	h := map[string]string{}
	for k, v := range p.Headers {
		h[k] = e(v)
	}
	p.Headers = h
	return p
}

// Ask posts one System One request to the profile's url with its headers (env vars expanded here, never stored).
func Ask(p Profile, state string, qs map[string]any, timeout time.Duration) (map[string]Answer, error) {
	ans, raw, err := AskRaw(p, state, qs, timeout)
	if err == nil {
		LastRaw = raw
	}
	return ans, err
}

// AskRaw is Ask without the shared LastRaw: it returns the response body too, so concurrent callers stay race-free.
func AskRaw(p Profile, state string, qs map[string]any, timeout time.Duration) (map[string]Answer, []byte, error) {
	if MissingEnv(p.Norm()) != "" {
		return nil, nil, ErrNeedKey
	}
	p = p.Expanded()
	body, _ := json.Marshal(map[string]any{"model": p.Model, "state": state, "questions": qs})
	req, _ := http.NewRequest("POST", p.URL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, nil, fmt.Errorf("%s: HTTP %d: %s", p.URL, resp.StatusCode, strings.TrimSpace(string(raw))[:min(300, len(strings.TrimSpace(string(raw))))])
	}
	var out struct {
		Answers map[string]Answer `json:"answers"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, nil, fmt.Errorf("unreadable answer: %v", err)
	}
	return out.Answers, raw, nil
}

// LastRaw is the full body of the last successful response (model, answers, usage, id), for commands that print it as is.
var LastRaw []byte

// P returns an answer's probability of yes (noul / Vercel-style "probability").
func (a Answer) P() float64 {
	if a.Noul != nil {
		return *a.Noul
	}
	if a.Probability != nil {
		return *a.Probability
	}
	return -1
}

// NoulQ and friends build question objects from the profile's question pack.
func NoulQ(p Profile, id string) map[string]any {
	q := questions(p).Noul[id]
	return map[string]any{"type": "noul", "instructions": q.Instructions, "criteria": map[string]string{"true": q.True, "false": q.False}}
}
func ChoiceQ(p Profile, id string) map[string]any {
	q := questions(p).Choice[id]
	return map[string]any{"type": "choice", "instructions": q.Instructions, "criteria": q.Options}
}
func ScoreQ(p Profile, id string) map[string]any {
	q := questions(p).Score[id]
	return map[string]any{"type": "score", "instructions": q.Instructions, "criteria": q.Levels}
}

// ---------------------------------------------------------------- the turn: request + proposal, in the training format

var secretRx = []*regexp.Regexp{
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\b(?:sk|pk|rk)-(?:proj-|ant-|live-|test-)?[A-Za-z0-9_\-]{20,}`),
	regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr|github_pat)_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9\-]{10,}`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`),
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._\-]{20,}`),
	regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:KEY|TOKEN|SECRET|PASSWORD|_PW|PASSWD)[A-Z0-9_]*\s*[=:]\s*)['"]?[^\s'"$]{8,}`),
}

// Redact removes common secret shapes (private keys, API tokens, JWTs, KEY=value) before anything leaves the machine.
func Redact(t string) string {
	for _, rx := range secretRx {
		t = rx.ReplaceAllStringFunc(t, func(m string) string {
			sub := rx.FindStringSubmatch(m)
			if len(sub) > 1 {
				return sub[1] + "[SECRET]"
			}
			return "[SECRET]"
		})
	}
	return t
}

func cut(t string, n int) string {
	t = Redact(t)
	if len([]rune(t)) <= n {
		return t
	}
	return string([]rune(t)[:n]) + " …[cut]"
}

// State renders one agent turn as the premise: request, agent text, actions, trimmed to ~440 tokens by a character budget
// (actions first, then the agent text, then the request; the server also truncates a long premise).
func State(req, text string, actions []string) string {
	const budget = 1600
	if text == "" {
		text = "(no text)"
	}
	acts := append([]string(nil), actions...)
	total := len(actions)
	for i := 0; i < 40; i++ {
		s := "Request: " + req + "\nAgent: " + text + "\n"
		if len(acts) > 0 {
			s += "Actions:\n- " + strings.Join(acts, "\n- ")
		} else {
			s += "Actions: none"
		}
		if len(s) <= budget {
			return s
		}
		switch {
		case len(acts) > 3:
			acts = append(acts[:len(acts)/2], fmt.Sprintf("(+%d more)", total-len(acts)/2))
		case len([]rune(text)) > 400:
			text = string([]rune(text)[:len([]rune(text))*7/10]) + " …"
		case len([]rune(req)) > 300:
			req = string([]rune(req)[:len([]rune(req))*7/10]) + " …"
		default:
			for j := range acts {
				if len(acts[j]) > 120 {
					acts[j] = acts[j][:120]
				}
			}
			s = s[:min(len(s), budget)]
			return s
		}
	}
	return "Request: " + req + "\nAgent: " + text
}

var (
	wrapperRx   = regexp.MustCompile(`^\s*<(command-name|command-message|local-command|system-reminder|task-notification|cross-session-message|bash-)`)
	interruptRx = regexp.MustCompile(`^\[Request interrupted by user`)
)

func actionOf(name string, in map[string]any) string {
	s := func(k string) string { v, _ := in[k].(string); return v }
	switch name {
	case "Edit", "Write", "NotebookEdit":
		d := s("new_string")
		if d == "" {
			d = s("content")
		}
		return fmt.Sprintf("%s %s: %s", name, s("file_path"), d[:min(len(d), 400)])
	case "Bash":
		return "Bash: " + s("command")
	case "Skill":
		return fmt.Sprintf("Skill %s %s", s("skill"), s("args"))
	case "Agent", "SendMessage":
		p := s("prompt")
		if p == "" {
			p = s("message")
		}
		d := s("description")
		if d == "" {
			d = s("to")
		}
		return fmt.Sprintf("%s %s: %s", name, d, p[:min(len(p), 200)])
	}
	b, _ := json.Marshal(in)
	return name + " " + string(b[:min(len(b), 200)])
}

// LastTurn reads a Claude Code transcript (JSONL) and returns the last genuine human request with everything the agent
// said and did after it. Only the tail of a long transcript is read.
func LastTurn(path string) (req, text string, actions []string, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	if st, _ := f.Stat(); st != nil && st.Size() > 4_000_000 {
		_, _ = f.Seek(st.Size()-4_000_000, io.SeekStart)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var r struct {
			Type        string          `json:"type"`
			IsSidechain bool            `json:"isSidechain"`
			IsMeta      bool            `json:"isMeta"`
			Message     json.RawMessage `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.IsSidechain {
			continue
		}
		var m struct {
			Content json.RawMessage `json:"content"`
		}
		_ = json.Unmarshal(r.Message, &m)
		var str string
		var blocks []map[string]any
		isStr := json.Unmarshal(m.Content, &str) == nil
		if !isStr {
			_ = json.Unmarshal(m.Content, &blocks)
		}
		switch {
		case r.Type == "user" && !r.IsMeta:
			t, human := str, isStr
			if !isStr {
				human = true
				var parts []string
				for _, b := range blocks {
					if b["type"] == "tool_result" {
						human = false
						break
					}
					if b["type"] == "text" {
						s, _ := b["text"].(string)
						parts = append(parts, s)
					}
				}
				t = strings.Join(parts, "\n")
			}
			if !human || wrapperRx.MatchString(t) || interruptRx.MatchString(t) {
				continue
			}
			req, text, actions = t, "", nil
		case r.Type == "assistant" && req != "":
			for _, b := range blocks {
				switch b["type"] {
				case "text":
					if s, _ := b["text"].(string); strings.TrimSpace(s) != "" {
						text = s
					}
				case "tool_use":
					name, _ := b["name"].(string)
					in, _ := b["input"].(map[string]any)
					actions = append(actions, cut(actionOf(name, in), 400))
				}
			}
		}
	}
	if req == "" || (text == "" && len(actions) == 0) {
		return
	}
	if len(actions) > 25 {
		actions = actions[:25]
	}
	return cut(req, 2500), cut(text, 3000), actions, true
}

// ---------------------------------------------------------------- the Stop hook

// HookInput is what Claude Code sends a Stop hook on stdin.
type HookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	StopHookActive bool   `json:"stop_hook_active"`
}

const acceptMin, moreMax = 0.35, 0.65

// Verdict scores one turn; the hook logs it and, in block mode, returns a reason that sends the agent back.
func Verdict(cfg Config, h HookCfg, in HookInput) (logRec map[string]any, blockReason string) {
	name, p, err := cfg.Profile(h.Profile)
	rec := map[string]any{"ts": time.Now().Format("2006-01-02T15:04:05"), "session": in.SessionID, "cwd": in.Cwd,
		"transcript": in.TranscriptPath, "profile": name, "mode": h.Mode}
	if err != nil {
		rec["error"] = err.Error()
		return rec, ""
	}
	req, text, acts, ok := LastTurn(in.TranscriptPath)
	if !ok {
		return nil, ""
	}
	st := State(req, text, acts)
	t0 := time.Now()
	ans, err := Ask(p, cfg.WithContext(p, st), map[string]any{"accepts": NoulQ(p, "accepts"), "wanted_more": NoulQ(p, "wanted_more")}, 12*time.Second)
	rec["ms"] = time.Since(t0).Milliseconds()
	if err != nil {
		rec["error"] = err.Error()
		return rec, ""
	}
	acc, more := ans["accepts"].P(), ans["wanted_more"].P()
	var why []string
	if acc < acceptMin {
		why = append(why, "not accepted")
	}
	if more > moreMax {
		why = append(why, "stopped short")
	}
	rec["accept"], rec["wanted_more"], rec["would_block"], rec["why"] = acc, more, len(why) > 0, why
	rec["state"] = st
	if len(p.Headers) > 0 {
		rec["third_party"] = true // answered by a hosted Jev: never use as training data (its terms)
	}
	block := len(why) > 0 && h.Mode == "block" && !in.StopHookActive
	rec["blocked"] = block
	if !block {
		return rec, ""
	}
	var tips []string
	if acc < acceptMin {
		tips = append(tips, fmt.Sprintf("the user would likely NOT accept this as it is (P(accept) %.2f): verify the result and show the evidence (command output, diff, test run)", acc))
	}
	if more > moreMax {
		tips = append(tips, fmt.Sprintf("the user would likely want MORE (P %.2f): finish the missing part yourself instead of handing steps back, unless it truly needs the user's decision", more))
	}
	return rec, "jevcli (" + name + ", the user's decision model): " + strings.Join(tips, "; ") + ". If this is already right, say so briefly with the evidence and stop."
}

func AppendLog(rec map[string]any) {
	_ = os.MkdirAll(DataDir(), 0o700)
	f, err := os.OpenFile(filepath.Join(DataDir(), "verdicts.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(rec)
	_, _ = f.Write(append(b, '\n'))
}

// Detach re-runs this binary with args in its own session, stdin from a file, so a shadow hook costs the agent nothing.
func Detach(args []string, stdinFile string) error {
	self, _ := os.Executable()
	cmd := exec.Command(self, args...)
	in, err := os.Open(stdinFile)
	if err != nil {
		return err
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, nil, nil
	detachAttr(cmd)
	return cmd.Start()
}

// ---------------------------------------------------------------- agent settings (Claude Code hooks)

const HookMarker = "jevcli hook run"

// SettingsPath is the REAL Claude Code user settings file (~/.claude/settings.json may be a symlink).
func SettingsPath() string {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		dir = filepath.Join(Home(), ".claude")
	}
	p := filepath.Join(dir, "settings.json")
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func readJSON(p string) (map[string]any, error) {
	m := map[string]any{}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	return m, json.Unmarshal(b, &m)
}

func writeJSON(p string, m map[string]any) error {
	if _, err := os.Stat(p); err == nil {
		b, _ := os.ReadFile(p)
		_ = os.WriteFile(fmt.Sprintf("%s.bak-jevcli-%s-%d", p, time.Now().Format("20060102-150405"), os.Getpid()), b, 0o600)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp-jevcli"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	var check map[string]any
	if b2, _ := os.ReadFile(tmp); json.Unmarshal(b2, &check) != nil {
		return errors.New("refusing to write invalid JSON")
	}
	return os.Rename(tmp, p)
}

func isOurs(entry any) bool {
	e, _ := entry.(map[string]any)
	hs, _ := e["hooks"].([]any)
	for _, h := range hs {
		hm, _ := h.(map[string]any)
		if c, _ := hm["command"].(string); strings.Contains(c, HookMarker) {
			return true
		}
	}
	return false
}

// InstallHookTemplate adds jevcli's Stop hook entry (idempotent). Installed is not enabled: the entry runs
// `jevcli hook run stop`, which does nothing unless hooks.stop.enabled is true in the jevcli config.
func InstallHookTemplate(settings string) (bool, error) {
	m, err := readJSON(settings)
	if err != nil {
		return false, err
	}
	hooks, _ := m["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		m["hooks"] = hooks
	}
	stop, _ := hooks["Stop"].([]any)
	for _, e := range stop {
		if isOurs(e) {
			return false, nil
		}
	}
	self, _ := os.Executable()
	stop = append(stop, map[string]any{"hooks": []any{map[string]any{"type": "command", "command": self + " hook run stop", "timeout": 15}}})
	hooks["Stop"] = stop
	return true, writeJSON(settings, m)
}

func RemoveHookTemplate(settings string) (int, error) {
	m, err := readJSON(settings)
	if err != nil {
		return 0, err
	}
	hooks, _ := m["hooks"].(map[string]any)
	stop, _ := hooks["Stop"].([]any)
	var keep []any
	for _, e := range stop {
		if !isOurs(e) {
			keep = append(keep, e)
		}
	}
	n := len(stop) - len(keep)
	if n == 0 {
		return 0, nil
	}
	if len(keep) == 0 {
		delete(hooks, "Stop")
	} else {
		hooks["Stop"] = keep
	}
	return n, writeJSON(settings, m)
}

func HookInstalled(settings string) bool {
	m, _ := readJSON(settings)
	hooks, _ := m["hooks"].(map[string]any)
	stop, _ := hooks["Stop"].([]any)
	for _, e := range stop {
		if isOurs(e) {
			return true
		}
	}
	return false
}
