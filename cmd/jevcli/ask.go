package main

// jevcli ask: the one call. Questions come by NAME from config (`jevcli question add`) and/or inline
// (--noul / --choice / --score); inputs from stdin, --in, --lines or --states. Every threshold and knob comes from
// config with defaults (core.Builtin < config "defaults" < profile "defaults" < flags).
//
// Output: one input -> one line per question "NAME  VERDICT  P" (or --json); a batch -> JSONL, one line per input.
// Exit (one input): 0 all yes/decided, 1 a "no", 3 an "unsure", 4 an error. A batch exits 0, or 4 if any input failed.

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/muthuishere/jevcli/core"
)

func text(v string) string {
	switch {
	case v == "-":
		b, _ := io.ReadAll(os.Stdin)
		return string(b)
	case strings.HasPrefix(v, "@"):
		b, err := os.ReadFile(v[1:])
		if err != nil {
			die("%v", err)
		}
		return string(b)
	}
	return v
}

func resolve(prof string) (string, core.Profile) {
	name, p, err := core.LoadConfig().Profile(prof)
	if err != nil {
		die("%v", err)
	}
	return name, p
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

// askChunks asks every question about one state, in chunks the server accepts; the model name comes from the reply.
func askChunks(p core.Profile, state string, qs map[string]core.Question, set core.Settings) (map[string]core.Answer, any, error) {
	ks := keys(qs)
	out, model := map[string]core.Answer{}, any(nil)
	for i := 0; i < len(ks); i += *set.Chunk {
		part := map[string]any{}
		for _, k := range ks[i:min(i+*set.Chunk, len(ks))] {
			part[k] = qs[k].Wire()
		}
		ans, raw, err := core.AskRaw(p, state, part, time.Duration(*set.TimeoutS)*time.Second)
		if err != nil {
			return nil, nil, err
		}
		var v map[string]any
		if json.Unmarshal(raw, &v) == nil {
			model = v["model"]
		}
		for k, a := range ans {
			out[k] = a
		}
	}
	return out, model, nil
}

// verdict turns an answer into yes | no | unsure (noul), the key or unsure (choice), the level or unsure (score).
func verdict(q core.Question, a core.Answer, set core.Settings) string {
	yes, no, minC := *set.Yes, *set.No, *set.MinConfidence
	if q.Yes != nil {
		yes = *q.Yes
	}
	if q.No != nil {
		no = *q.No
	}
	if q.MinConfidence != nil {
		minC = *q.MinConfidence
	}
	conf := 1.0
	if a.Confidence != nil {
		conf = *a.Confidence
	}
	switch q.Type {
	case "noul":
		switch p := a.P(); {
		case p >= yes:
			return "yes"
		case p <= no:
			return "no"
		}
		return "unsure"
	case "choice":
		if conf < minC {
			return "unsure"
		}
		return a.Choice
	case "score":
		if conf < minC || a.Score == nil {
			return "unsure"
		}
		if lv, ok := q.Criteria.([]string); ok {
			if i := int(math.Round(*a.Score)); i >= 0 && i < len(lv) {
				return lv[i]
			}
		}
		return strconv.FormatFloat(*a.Score, 'f', 2, 64)
	}
	return ""
}

// value is the number shown next to a verdict: P(yes) for a noul, the confidence otherwise.
func value(a core.Answer) float64 {
	if a.Type == "noul" {
		return a.P()
	}
	if a.Confidence != nil {
		return *a.Confidence
	}
	return 0
}

func inlineQ(kind, spec string) (string, core.Question) {
	name, rest, ok := strings.Cut(spec, "=")
	if !ok || strings.TrimSpace(name) == "" {
		die("--%s: want NAME=\"QUESTION\", got %q", kind, spec)
	}
	return strings.TrimSpace(name), parseQ(kind, rest)
}

// parseQ reads the compact question form: noul "Q" or "Q|true text|false text"; choice "Q|k=desc;k2=desc";
// score "Q|low;mid;high".
func parseQ(kind, spec string) core.Question {
	parts := strings.Split(spec, "|")
	q := core.Question{Type: kind, Instructions: strings.TrimSpace(parts[0])}
	switch kind {
	case "noul":
		if len(parts) == 3 {
			q.Criteria = map[string]string{"true": strings.TrimSpace(parts[1]), "false": strings.TrimSpace(parts[2])}
		}
	case "choice":
		if len(parts) != 2 {
			die("choice: want \"QUESTION|key=desc;key2=desc\", got %q", spec)
		}
		crit := map[string]string{}
		for _, o := range strings.Split(parts[1], ";") {
			k, v, _ := strings.Cut(o, "=")
			if v == "" {
				v = k
			}
			crit[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
		q.Criteria = crit
	case "score":
		if len(parts) != 2 {
			die("score: want \"QUESTION|level0;level1;...\", got %q", spec)
		}
		var lv []string
		for _, l := range strings.Split(parts[1], ";") {
			lv = append(lv, strings.TrimSpace(l))
		}
		q.Criteria = lv
	}
	return q
}

// normQ gives a question loaded from JSON the concrete criteria types the rest of the code expects.
func normQ(q core.Question) core.Question {
	switch c := q.Criteria.(type) {
	case []any:
		var lv []string
		for _, v := range c {
			lv = append(lv, fmt.Sprint(v))
		}
		q.Criteria = lv
	case map[string]any:
		m := map[string]string{}
		for k, v := range c {
			m[k] = fmt.Sprint(v)
		}
		q.Criteria = m
	}
	return q
}

// splitArgs separates positional NAMES from flags so both `ask urgent --in x` and `ask --in x urgent` work.
func splitArgs(args []string, valued map[string]bool) (names, flags []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			for _, n := range strings.Split(a, ",") {
				if n = strings.TrimSpace(n); n != "" {
					names = append(names, n)
				}
			}
			continue
		}
		flags = append(flags, a)
		if f := strings.TrimLeft(a, "-"); valued[f] && !strings.Contains(f, "=") && i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return
}

func cmdAsk(args []string) {
	fs := flag.NewFlagSet("ask", flag.ExitOnError)
	in := fs.String("in", "", "one input: text, a JSON object, @file or - (default: stdin)")
	lines := fs.String("lines", "", "batch: a text file (or -), one input per line")
	states := fs.String("states", "", "batch: a JSONL file (or -), one JSON object or string per line")
	qfile := fs.String("questions", "", "a question-set file: {NAME: {type, instructions, criteria}}")
	prof := fs.String("profile", "", "endpoint profile")
	asJSON := fs.Bool("json", false, "one input: print JSON instead of lines")
	raw := fs.Bool("raw", false, "one input: print the server's full response")
	yes := fs.Float64("yes", -1, "override: noul P at or above is yes")
	no := fs.Float64("no", -1, "override: noul P at or below is no")
	minC := fs.Float64("min", -1, "override: choice / score confidence below is unsure")
	par := fs.Int("parallel", 0, "override: concurrent requests in a batch")
	var nouls, choices, scores multi
	fs.Var(&nouls, "noul", `NAME="QUESTION" (repeat); or NAME="QUESTION|true means|false means"`)
	fs.Var(&choices, "choice", `NAME="QUESTION|key=desc;key2=desc" (repeat)`)
	fs.Var(&scores, "score", `NAME="QUESTION|low;mid;high" (repeat, lowest first)`)
	valued := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) {
		if b, ok := f.Value.(interface{ IsBoolFlag() bool }); !ok || !b.IsBoolFlag() {
			valued[f.Name] = true
		}
	})
	names, flags := splitArgs(args, valued)
	_ = fs.Parse(flags)

	cfg := core.LoadConfig()
	name, p, err := cfg.Profile(*prof)
	if err != nil {
		die("%v", err)
	}
	set := cfg.Settings(p)
	if *yes >= 0 {
		set.Yes = yes
	}
	if *no >= 0 {
		set.No = no
	}
	if *minC >= 0 {
		set.MinConfidence = minC
	}
	if *par > 0 {
		set.Parallel = par
	}
	core.Retries, core.LedgerOn = *set.Retries, *set.Ledger

	qs := map[string]core.Question{}
	for _, n := range names {
		q, ok := cfg.Questions[n]
		if !ok {
			die("no question named %q (have: %s). Add one: jevcli question add %s --noul \"...\"", n, strings.Join(keys(cfg.Questions), ", "), n)
		}
		qs[n] = normQ(q)
	}
	if *qfile != "" {
		var m map[string]core.Question
		if err := json.Unmarshal([]byte(text("@"+strings.TrimPrefix(*qfile, "@"))), &m); err != nil {
			die("--questions %s: %v", *qfile, err)
		}
		for k, q := range m {
			qs[k] = normQ(q)
		}
	}
	for kind, specs := range map[string]multi{"noul": nouls, "choice": choices, "score": scores} {
		for _, sp := range specs {
			n, q := inlineQ(kind, sp)
			qs[n] = q
		}
	}
	if len(qs) == 0 {
		die("usage: jevcli ask NAME[,NAME...] | --noul NAME=\"QUESTION\" ...  [--in X | --lines FILE | --states FILE.jsonl | < stdin]\n  named questions: jevcli question list")
	}

	// A batch: --lines or --states.
	if *lines != "" || *states != "" {
		var inputs []string
		var nums []int
		src := *lines + *states
		if src != "-" {
			src = "@" + strings.TrimPrefix(src, "@")
		}
		for i, l := range strings.Split(text(src), "\n") {
			if strings.TrimSpace(l) == "" {
				continue
			}
			if *states != "" {
				var s string
				if strings.HasPrefix(strings.TrimSpace(l), `"`) && json.Unmarshal([]byte(l), &s) == nil {
					l = s
				}
			}
			inputs, nums = append(inputs, l), append(nums, i+1)
		}
		os.Exit(batch(cfg, p, qs, set, inputs, nums))
	}

	// One input.
	src := *in
	if src == "" {
		src = "-"
	}
	state, err := normState(text(src))
	if err != nil {
		die("input: %v", err)
	}
	ans, model, err := askChunks(p, cfg.WithContext(p, state), qs, set)
	if errors.Is(err, core.ErrNeedKey) {
		die("profile %s needs %s in the environment", name, core.MissingEnv(p))
	}
	if err != nil {
		die("%v", err)
	}
	if *raw {
		b, _ := json.MarshalIndent(map[string]any{"model": model, "answers": ans}, "", "  ")
		pr("%s", b)
		return
	}
	code, rows := 0, map[string]any{}
	for _, k := range keys(qs) {
		v := verdict(qs[k], ans[k], set)
		switch {
		case v == "unsure":
			code = 3
		case v == "no" && code == 0:
			code = 1
		}
		if *asJSON {
			rows[k] = map[string]any{"verdict": v, "answer": ans[k]}
		} else {
			pr("%-16s %-10s %.2f", k, v, value(ans[k]))
		}
	}
	if *asJSON {
		b, _ := json.Marshal(map[string]any{"profile": name, "model": model, "answers": rows})
		pr("%s", b)
	}
	os.Exit(code)
}

// batch asks every question of every input, set.Parallel at a time, and prints one JSONL line per input in input
// order. A failed input prints its error and the rest go on; it returns 4 if any failed.
func batch(cfg core.Config, p core.Profile, qs map[string]core.Question, set core.Settings, inputs []string, nums []int) int {
	type row struct {
		Line    int            `json:"line"`
		Input   string         `json:"input"`
		Answers map[string]any `json:"answers,omitempty"`
		Error   string         `json:"error,omitempty"`
	}
	rows := make([]row, len(inputs))
	sem := make(chan struct{}, max(1, *set.Parallel))
	var wg sync.WaitGroup
	for i, l := range inputs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			rows[i] = row{Line: nums[i], Input: trunc(l, 120)}
			state, err := normState(l)
			var ans map[string]core.Answer
			if err == nil {
				ans, _, err = askChunks(p, cfg.WithContext(p, state), qs, set)
			}
			if err != nil {
				rows[i].Error = err.Error()
				return
			}
			rows[i].Answers = map[string]any{}
			for k, a := range ans {
				rows[i].Answers[k] = map[string]any{"verdict": verdict(qs[k], a, set), "p": round2(value(a)), "answer": a}
			}
		}()
	}
	wg.Wait()
	code := 0
	for _, r := range rows {
		b, _ := json.Marshal(r)
		pr("%s", b)
		if r.Error != "" {
			code = 4
		}
	}
	return code
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// ------------------------------------------------------------------ named questions + settings

func cmdQuestion(args []string) {
	cfg := core.LoadConfig()
	if len(args) == 0 || args[0] == "list" {
		if len(cfg.Questions) == 0 {
			pr("no named questions yet: jevcli question add NAME --noul \"QUESTION\"")
			return
		}
		for _, k := range keys(cfg.Questions) {
			q := cfg.Questions[k]
			pr("%-16s %-7s %s", k, q.Type, q.Instructions)
		}
		return
	}
	switch args[0] {
	case "show":
		if len(args) < 2 {
			die("usage: jevcli question show NAME")
		}
		q, ok := cfg.Questions[args[1]]
		if !ok {
			die("no question named %q", args[1])
		}
		b, _ := json.MarshalIndent(q, "", "  ")
		pr("%s", b)
		return
	case "remove", "rm":
		if len(args) < 2 {
			die("usage: jevcli question remove NAME")
		}
		delete(cfg.Questions, args[1])
	case "add":
		if len(args) < 2 || strings.HasPrefix(args[1], "-") {
			die(`usage: jevcli question add NAME --noul "QUESTION" | --choice "QUESTION|k=desc;k2=desc" | --score "QUESTION|low;mid;high" [--yes 0.85 --no 0.15 --min 0.6]`)
		}
		fs := flag.NewFlagSet("question add", flag.ExitOnError)
		nq, cq, sq := fs.String("noul", "", "a yes/no question"), fs.String("choice", "", "QUESTION|key=desc;..."), fs.String("score", "", "QUESTION|level0;level1;...")
		yes, no, minC := fs.Float64("yes", -1, "yes at or above"), fs.Float64("no", -1, "no at or below"), fs.Float64("min", -1, "min confidence")
		_ = fs.Parse(args[2:])
		var q core.Question
		switch {
		case *nq != "":
			q = parseQ("noul", *nq)
		case *cq != "":
			q = parseQ("choice", *cq)
		case *sq != "":
			q = parseQ("score", *sq)
		default:
			die("give --noul, --choice or --score")
		}
		if *yes >= 0 {
			q.Yes = yes
		}
		if *no >= 0 {
			q.No = no
		}
		if *minC >= 0 {
			q.MinConfidence = minC
		}
		if cfg.Questions == nil {
			cfg.Questions = map[string]core.Question{}
		}
		cfg.Questions[args[1]] = q
	default:
		die("usage: jevcli question list | show NAME | add NAME --noul|--choice|--score ... | remove NAME")
	}
	if err := core.SaveConfig(cfg); err != nil {
		die("%v", err)
	}
	pr("saved %s", core.ConfigPath())
}

// settingKeys maps the config keys to their Settings fields.
var settingKeys = []string{"yes", "no", "min_confidence", "parallel", "retries", "timeout_s", "chunk", "ledger", "accept_min", "more_max"}

// cmdDefaults: jevcli defaults [--profile P] | set KEY VALUE [--profile P] | unset KEY [--profile P].
func cmdDefaults(args []string) {
	cfg := core.LoadConfig()
	prof := ""
	var rest []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--profile" && i+1 < len(args) {
			prof, i = args[i+1], i+1
			continue
		}
		rest = append(rest, args[i])
	}
	target := &cfg.Defaults
	var p core.Profile
	if prof != "" {
		var ok bool
		if p, ok = cfg.Profiles[prof]; !ok {
			die("no profile %q", prof)
		}
		target = &p.Defaults
	}
	if len(rest) == 0 || rest[0] == "show" {
		eff := cfg.Settings(p)
		m, _ := json.Marshal(eff)
		own, _ := json.Marshal(*target)
		var em, om map[string]any
		_ = json.Unmarshal(m, &em)
		_ = json.Unmarshal(own, &om)
		for _, k := range settingKeys {
			src := "default"
			if _, ok := om[k]; ok {
				src = map[bool]string{true: "profile " + prof, false: "config"}[prof != ""]
			}
			pr("%-16s %-8v %s", k, em[k], src)
		}
		return
	}
	if len(rest) < 2 || (rest[0] == "set" && len(rest) < 3) {
		die("usage: jevcli defaults [show] | set KEY VALUE | unset KEY  [--profile P]   keys: %s", strings.Join(settingKeys, ", "))
	}
	b, _ := json.Marshal(*target)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m == nil {
		m = map[string]any{}
	}
	k := rest[1]
	if !contains(settingKeys, k) {
		die("unknown key %q (keys: %s)", k, strings.Join(settingKeys, ", "))
	}
	switch rest[0] {
	case "set":
		var v any
		if json.Unmarshal([]byte(rest[2]), &v) != nil {
			die("%s: want a number or true/false, got %q", k, rest[2])
		}
		m[k] = v
	case "unset":
		delete(m, k)
	default:
		die("usage: jevcli defaults set KEY VALUE | unset KEY")
	}
	b, _ = json.Marshal(m)
	var ns core.Settings
	if err := json.Unmarshal(b, &ns); err != nil {
		die("%s: %v", k, err)
	}
	*target = ns
	if prof != "" {
		cfg.Profiles[prof] = p
	}
	if err := core.SaveConfig(cfg); err != nil {
		die("%v", err)
	}
	pr("saved %s", core.ConfigPath())
}
