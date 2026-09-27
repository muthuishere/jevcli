package main

// The Jev cookbook as commands. Each recipe is plain System One questions; the context lives in the question texts.
//
//	jevcli verify  --claim TEXT --source TEXT|@file                     citation / fact check (P(supported))
//	jevcli same    --a TEXT --b TEXT [--what "customer record"]         entity alignment, duplicate detection
//	jevcli rank    --query TEXT --candidates FILE|- [--dimension "..."]  re-ranking; composite with several --dimension
//	jevcli find    QUERY FILE [--min 0.5]                               semantic grep, one question per line
//	jevcli extract --text TEXT|@file --what "the due date" [--kind date|number|email|url|money] [--candidate X ...]
//	jevcli tree    --text TEXT|@file --taxonomy FILE.json               hierarchical classification (walks the tree)
//	jevcli score   --text TEXT|@file --question Q --level L0 --level L1 ...   ordinal rating
//	jevcli pick-skill TASK [--dir DIR ...]                              which agent skill to load first
//	jevcli pick-func  REQUEST --functions FILE.json                     function calling from natural language
//	jevcli run     REQUEST.json|-                                       raw request: batching, speculative fan-out, structure recovery
//
// Also on `ask`: --route ACT,CONFIRM (confidence-gated routing / cascade: prints act|confirm|escalate, exit 0|10|20) and
// --samples N (self-consistency: N asks with shuffled option order, reports agreement).

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/muthuishere/jevcli/core"
)

// text reads a flag value: "@path" reads a file, "-" reads stdin.
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

func profileFlag(fs *flag.FlagSet) *string { return fs.String("profile", "", "endpoint profile") }

func resolve(prof string) (string, core.Profile) {
	name, p, err := core.LoadConfig().Profile(prof)
	if err != nil {
		die("%v", err)
	}
	return name, p
}

// askMany sends questions for ONE state in chunks the server accepts (System One servers cap questions per request).
func askMany(name string, p core.Profile, state string, qs map[string]any) map[string]core.Answer {
	const chunk = 32
	ks := make([]string, 0, len(qs))
	for k := range qs {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	out := map[string]core.Answer{}
	for i := 0; i < len(ks); i += chunk {
		part := map[string]any{}
		for _, k := range ks[i:min(i+chunk, len(ks))] {
			part[k] = qs[k]
		}
		for k, v := range ask(name, p, state, part) {
			out[k] = v
		}
	}
	return out
}

// askManyErr is askMany for concurrent callers: the context is already applied, errors come back instead of exiting, and
// the model name comes from the response body rather than the shared LastRaw.
func askManyErr(p core.Profile, state string, qs map[string]any) (map[string]core.Answer, any, error) {
	const chunk = 32
	ks := keys(qs)
	out, model := map[string]core.Answer{}, any(nil)
	for i := 0; i < len(ks); i += chunk {
		part := map[string]any{}
		for _, k := range ks[i:min(i+chunk, len(ks))] {
			part[k] = qs[k]
		}
		ans, raw, err := core.AskRaw(p, state, part, 60*time.Second)
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

func noul(instr, t, f string) map[string]any {
	return map[string]any{"type": "noul", "instructions": instr, "criteria": map[string]string{"true": t, "false": f}}
}

func recipe(cmd string, args []string) {
	switch cmd {
	case "verify":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		claim, src, prof := fs.String("claim", "", "the claim or citation"), fs.String("source", "", "the source text (@file)"), profileFlag(fs)
		_ = fs.Parse(args)
		if *claim == "" || *src == "" {
			die("usage: jevcli verify --claim TEXT --source TEXT|@file")
		}
		name, p := resolve(*prof)
		a := ask(name, p, text(*src), map[string]any{"q": noul("Does the source support this claim? Claim: "+*claim,
			"The source states or directly implies the claim.", "The source does not support the claim, or contradicts it.")})["q"]
		pr("%s  (P(supported) %.2f, %s)", map[bool]string{true: "supported", false: "not supported"}[a.P() >= 0.5], a.P(), name)

	case "same":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		ra, rb, what, prof := fs.String("a", "", "record A"), fs.String("b", "", "record B"), fs.String("what", "record", "what the records describe"), profileFlag(fs)
		_ = fs.Parse(args)
		if *ra == "" || *rb == "" {
			die("usage: jevcli same --a TEXT --b TEXT [--what \"customer record\"]")
		}
		name, p := resolve(*prof)
		st := fmt.Sprintf("Record A: %s\nRecord B: %s", text(*ra), text(*rb))
		a := ask(name, p, st, map[string]any{"q": noul(fmt.Sprintf("Two %ss.", *what),
			"Record A and record B refer to the same real-world entity.", "Record A and record B refer to different entities.")})["q"]
		pr("%s  (P(same) %.2f, %s)", map[bool]string{true: "same", false: "different"}[a.P() >= 0.5], a.P(), name)

	case "rank":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		query, cands, prof, top := fs.String("query", "", "what is being searched for"), fs.String("candidates", "", "FILE (one per line) or -"), profileFlag(fs), fs.Int("top", 10, "print the top N")
		var dims multi
		fs.Var(&dims, "dimension", "extra scoring dimension (repeat) for composite ranking; default: relevance")
		_ = fs.Parse(args)
		if *query == "" || *cands == "" {
			die("usage: jevcli rank --query TEXT --candidates FILE|- [--dimension TEXT ...]")
		}
		name, p := resolve(*prof)
		lines := nonEmpty(text(map[bool]string{true: "-", false: "@" + *cands}[*cands == "-"]))
		if len(dims) == 0 {
			dims = multi{"It is relevant to what is being searched for: " + *query}
		}
		type row struct {
			s    float64
			line string
		}
		var rows []row
		for _, c := range lines { // one state per candidate; each dimension a noul; composite = mean
			qs := map[string]any{}
			for i, d := range dims {
				qs[fmt.Sprint("d", i)] = noul("Search: "+*query, d, "It is not.")
			}
			ans := askMany(name, p, c, qs)
			sum := 0.0
			for _, a := range ans {
				sum += a.P()
			}
			rows = append(rows, row{sum / float64(len(ans)), c})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].s > rows[j].s })
		for i, r := range rows {
			if i >= *top {
				break
			}
			pr("%.3f  %s", r.s, trunc(r.line, 160))
		}

	case "find":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		minP, prof := fs.Float64("min", 0.5, "print lines at or above this"), profileFlag(fs)
		q, rest := firstPositional(args)
		file, rest2 := firstPositional(rest)
		_ = fs.Parse(rest2)
		if q == "" || file == "" {
			die("usage: jevcli find QUERY FILE [--min 0.5]")
		}
		name, p := resolve(*prof)
		b, err := os.ReadFile(file)
		if err != nil {
			die("%v", err)
		}
		lines := strings.Split(string(b), "\n")
		for i, l := range lines {
			if strings.TrimSpace(l) == "" {
				continue
			}
			a := ask(name, p, l, map[string]any{"q": noul("Does this line match the search? Search: "+q, "The line matches what is searched for.", "The line does not match.")})["q"]
			if a.P() >= *minP {
				pr("%s:%d  %.2f  %s", file, i+1, a.P(), trunc(strings.TrimSpace(l), 160))
			}
		}

	case "extract":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		src, what, kind, prof := fs.String("text", "", "TEXT|@file|-"), fs.String("what", "", `what to extract, e.g. "the invoice due date"`), fs.String("kind", "", "date|number|email|url|money: parse candidates from the text"), profileFlag(fs)
		var cands multi
		fs.Var(&cands, "candidate", "an explicit candidate value (repeat)")
		_ = fs.Parse(args)
		if *src == "" || *what == "" {
			die("usage: jevcli extract --text TEXT|@file --what TEXT [--kind date|number|email|url|money] [--candidate X ...]")
		}
		t := text(*src)
		if *kind != "" { // pre-parsed extraction: code enumerates, the model picks (the cookbook's answer to its numeric/date weakness)
			cands = append(cands, parse(*kind, t)...)
		}
		cands = uniq(cands)
		if len(cands) == 0 {
			die("no candidates: give --candidate or a --kind that occurs in the text")
		}
		if len(cands) == 1 {
			pr("%s  (only candidate)", cands[0])
			return
		}
		name, p := resolve(*prof)
		crit := map[string]string{"none": "None of these is " + *what + "."}
		for i, c := range cands {
			crit[fmt.Sprint("c", i)] = fmt.Sprintf("%s is %s.", c, *what)
		}
		a := ask(name, p, t, map[string]any{"q": map[string]any{"type": "choice", "instructions": "Which value is " + *what + "?", "criteria": crit}})["q"]
		val := "none"
		if strings.HasPrefix(a.Choice, "c") {
			var i int
			fmt.Sscan(a.Choice[1:], &i)
			val = cands[i]
		}
		pr("%s  (confidence %.2f, %s)", val, conf(a), name)

	case "tree":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		src, taxf, prof := fs.String("text", "", "TEXT|@file|-"), fs.String("taxonomy", "", `JSON: {"label": {"child": {...}} | "description"}`), profileFlag(fs)
		_ = fs.Parse(args)
		if *src == "" || *taxf == "" {
			die("usage: jevcli tree --text TEXT|@file --taxonomy FILE.json")
		}
		var tax map[string]any
		b, err := os.ReadFile(*taxf)
		if err != nil || json.Unmarshal(b, &tax) != nil {
			die("taxonomy %s: not a JSON object", *taxf)
		}
		name, p := resolve(*prof)
		t, node, path := text(*src), tax, []string{}
		for len(node) > 1 || (len(node) == 1 && path == nil) {
			crit := map[string]string{}
			for k, v := range node {
				if d, ok := v.(string); ok && d != "" {
					crit[k] = d
				} else {
					crit[k] = k
				}
			}
			if len(crit) < 2 {
				break
			}
			a := ask(name, p, t, map[string]any{"q": map[string]any{"type": "choice", "instructions": "Which category fits best" + under(path) + "?", "criteria": crit}})["q"]
			path = append(path, a.Choice)
			pr("%-40s confidence %.2f", strings.Join(path, " > "), conf(a))
			child, ok := node[a.Choice].(map[string]any)
			if !ok {
				break
			}
			node = child
		}

	case "score":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		src, q, prof := fs.String("text", "", "TEXT|@file|-"), fs.String("question", "", "what is rated"), profileFlag(fs)
		var levels multi
		fs.Var(&levels, "level", "a level, lowest first (repeat)")
		_ = fs.Parse(args)
		if *src == "" || *q == "" || len(levels) < 2 {
			die("usage: jevcli score --text TEXT --question TEXT --level L0 --level L1 [...]")
		}
		name, p := resolve(*prof)
		a := ask(name, p, text(*src), map[string]any{"q": map[string]any{"type": "score", "instructions": *q, "criteria": []string(levels)}})["q"]
		s := 0.0
		if a.Score != nil {
			s = *a.Score
		}
		pr("%.2f / %d  (%s; confidence %.2f, %s)", s, len(levels)-1, levels[min(len(levels)-1, int(s+0.5))], conf(a), name)

	case "pick-skill":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		prof := profileFlag(fs)
		var dirs multi
		fs.Var(&dirs, "dir", "skills directory (repeat); default: the agent skill dirs jevcli installs into")
		task, rest := firstPositional(args)
		_ = fs.Parse(rest)
		if task == "" {
			die("usage: jevcli pick-skill TASK [--dir DIR ...]")
		}
		if len(dirs) == 0 {
			dirs = skillDirs()
		}
		crit := map[string]string{"none": "No skill fits this task; the agent works without one."}
		for _, d := range dirs {
			ms, _ := filepath.Glob(filepath.Join(d, "*", "SKILL.md"))
			for _, m := range ms {
				if n, desc := skillMeta(m); n != "" {
					crit[n] = "Load the " + n + " skill: " + trunc(desc, 220)
				}
			}
		}
		if len(crit) < 2 {
			die("no skills found in %v", dirs)
		}
		name, p := resolve(*prof)
		a := askMany(name, p, "Task: "+task, map[string]any{"q": map[string]any{"type": "choice", "instructions": "Which skill should the agent load first for this task?", "criteria": crit}})["q"]
		pr("%s  (confidence %.2f of %d skills, %s)", a.Choice, conf(a), len(crit)-1, name)
		top := sortedProbs(a.Probabilities)
		for _, x := range top[:min(5, len(top))] {
			pr("  %.2f  %s", x.v, x.k)
		}

	case "feels":
		// A shell if statement: `if jevcli feels urgent < email.txt; then ...`. Exit 0 = yes, 1 = no, 2 = error.
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		in, prof, th := fs.String("in", "-", "the input: TEXT, @file or - (stdin)"), profileFlag(fs), fs.Float64("threshold", 0.5, "P(yes) needed to exit 0")
		quiet := fs.Bool("q", false, "print nothing, only the exit code")
		adj, rest := firstPositional(args)
		_ = fs.Parse(rest)
		if adj == "" {
			die("usage: jevcli feels ADJECTIVE|\"QUESTION\" [--input TEXT|@file|-] [--threshold 0.5]   (exit 0 yes, 1 no)")
		}
		instr := adj
		if !strings.ContainsAny(adj, " ?") {
			instr = "Does this feel " + adj + "?"
		}
		name, p := resolve(*prof)
		a := ask(name, p, text(*in), map[string]any{"q": map[string]any{"type": "noul", "instructions": instr}})["q"]
		if !*quiet {
			pr("%.2f", a.P())
		}
		if a.P() < *th {
			os.Exit(1)
		}

	case "match":
		// A switch: `case $(jevcli match billing="about money" bug="a defect report" < msg) in billing) ...`.
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		in, prof, instr := fs.String("in", "-", "the input: TEXT, @file or - (stdin)"), profileFlag(fs), fs.String("question", "Which description fits best?", "the question")
		var arms []string
		for len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			arms, args = append(arms, args[0]), args[1:]
		}
		_ = fs.Parse(args)
		arms = append(arms, fs.Args()...)
		if len(arms) < 2 {
			die("usage: jevcli match KEY=\"description\" KEY2=\"description\" [...] [--input TEXT|@file|-]   (prints the key)")
		}
		crit := map[string]string{}
		for _, a := range arms {
			k, v, ok := strings.Cut(a, "=")
			if !ok || k == "" {
				die("want KEY=description, got %q", a)
			}
			crit[k] = v
		}
		name, p := resolve(*prof)
		a := ask(name, p, text(*in), map[string]any{"q": map[string]any{"type": "choice", "instructions": *instr, "criteria": crit}})["q"]
		fmt.Fprintf(os.Stderr, "match: %s (confidence %.2f, %s)\n", a.Choice, conf(a), name)
		pr("%s", a.Choice)

	case "pick-func":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		funcs, prof := fs.String("functions", "", `JSON: [{"name":..,"description":..}]`), profileFlag(fs)
		req, rest := firstPositional(args)
		_ = fs.Parse(rest)
		if req == "" || *funcs == "" {
			die("usage: jevcli pick-func REQUEST --functions FILE.json")
		}
		var fl []struct{ Name, Description string }
		b, err := os.ReadFile(*funcs)
		if err != nil || json.Unmarshal(b, &fl) != nil || len(fl) == 0 {
			die("functions %s: need a JSON list of {name, description}", *funcs)
		}
		crit := map[string]string{"none": "None of these functions handles the request."}
		for _, f := range fl {
			crit[f.Name] = "Call " + f.Name + ": " + f.Description
		}
		name, p := resolve(*prof)
		a := ask(name, p, "Request: "+req, map[string]any{"q": map[string]any{"type": "choice", "instructions": "Which function should handle this request?", "criteria": crit}})["q"]
		pr("%s  (confidence %.2f, %s)", a.Choice, conf(a), name)

	case "run":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		prof := profileFlag(fs)
		f, rest := firstPositional(args)
		_ = fs.Parse(rest)
		if f == "" {
			die(`usage: jevcli run REQUEST.json|-   ({"state": "...", "questions": {...}}: many questions for one state)`)
		}
		var body struct {
			State     string         `json:"state"`
			Questions map[string]any `json:"questions"`
		}
		src := map[bool]string{true: "-", false: "@" + f}[f == "-"]
		if json.Unmarshal([]byte(text(src)), &body) != nil || body.State == "" || len(body.Questions) == 0 {
			die("request needs state and questions")
		}
		name, p := resolve(*prof)
		out, _ := json.MarshalIndent(map[string]any{"profile": name, "answers": askMany(name, p, body.State, body.Questions)}, "", "  ")
		pr("%s", out)
	}
}

// ---------------------------------------------------------------- helpers

type kv struct {
	k string
	v float64
}

func sortedProbs(m map[string]float64) []kv {
	var l []kv
	for k, v := range m {
		l = append(l, kv{k, v})
	}
	sort.Slice(l, func(i, j int) bool { return l[i].v > l[j].v })
	return l
}

func conf(a core.Answer) float64 {
	if a.Confidence != nil {
		return *a.Confidence
	}
	return 0
}

func under(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return " within " + strings.Join(path, " > ")
}

func nonEmpty(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

func uniq(l []string) []string {
	seen, out := map[string]bool{}, []string{}
	for _, x := range l {
		if x = strings.TrimSpace(x); x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

var kinds = map[string]*regexp.Regexp{
	"date":   regexp.MustCompile(`\b(?:\d{4}-\d{2}-\d{2}|\d{1,2}[/.-]\d{1,2}[/.-]\d{2,4}|\d{1,2} (?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]* \d{4}|(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]* \d{1,2},? \d{4})\b`),
	"number": regexp.MustCompile(`-?\b\d[\d,]*(?:\.\d+)?\b`),
	"money":  regexp.MustCompile(`(?:[$€£₹]\s?\d[\d,]*(?:\.\d+)?|\b\d[\d,]*(?:\.\d+)?\s?(?:USD|EUR|GBP|INR)\b)`),
	"email":  regexp.MustCompile(`\b[\w.+-]+@[\w-]+\.[\w.-]+\b`),
	"url":    regexp.MustCompile(`https?://[^\s)>"']+`),
}

func parse(kind, t string) []string {
	rx, ok := kinds[kind]
	if !ok {
		die("unknown --kind %q (date|number|money|email|url)", kind)
	}
	return rx.FindAllString(t, 40)
}

// skillMeta reads a SKILL.md front matter name + description.
func skillMeta(path string) (string, string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	var name, desc string
	for _, l := range strings.Split(string(b), "\n")[:min(20, len(strings.Split(string(b), "\n")))] {
		if v, ok := strings.CutPrefix(l, "name:"); ok {
			name = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(l, "description:"); ok {
			desc = strings.TrimSpace(v)
		}
	}
	return name, desc
}

// selfConsistency asks a choice N times with shuffled option order and reports agreement (cookbook: self-consistency).
func selfConsistency(name string, p core.Profile, state string, q map[string]any, n int) {
	crit, _ := q["criteria"].(map[string]string)
	votes, sum := map[string]int{}, map[string]float64{}
	keysL := make([]string, 0, len(crit))
	for k := range crit {
		keysL = append(keysL, k)
	}
	for i := 0; i < n; i++ {
		rand.Shuffle(len(keysL), func(a, b int) { keysL[a], keysL[b] = keysL[b], keysL[a] })
		ordered := map[string]string{}
		for _, k := range keysL {
			ordered[k] = crit[k]
		}
		q2 := map[string]any{}
		for k, v := range q {
			q2[k] = v
		}
		q2["criteria"] = ordered
		a := ask(name, p, state, map[string]any{"q": q2})["q"]
		votes[a.Choice]++
		for k, v := range a.Probabilities {
			sum[k] += v / float64(n)
		}
	}
	best := sortedProbs(sum)
	agree := float64(votes[best[0].k]) / float64(n)
	pr("%s  (agreement %.0f%% over %d samples, mean P %.2f, %s)", best[0].k, 100*agree, n, best[0].v, name)
	for _, x := range best {
		pr("  %.2f  %s  (%d votes)", x.v, x.k, votes[x.k])
	}
}
