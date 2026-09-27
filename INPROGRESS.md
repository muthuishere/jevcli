# INPROGRESS: jevcli live state

Last updated: 2026-09-27. Read `README.md` for usage; this file is what exists, what is verified, and what is next.

## The core
**The parallel query**: one state (text or a JSON object), many named questions (noul / choice / score) answered in ONE
System One request. Everything else is built on it.

```bash
jevcli query --state '{"role": "...", "message": "..."}' \
  --noul is_appropriate="Does the message contain inappropriate language?" \
  --noul does_this_help="Does this help donkey kong win?" \
  --choice next="What should happen?|a=desc;b=desc" --score risk="How risky?|low;medium;high" --raw
```
Verified 2026-09-27 against the hosted Jev: `model jev-1.13.0`, `is_appropriate 0.06`, `does_this_help 0.75`, `usage`
returned. `askMany` splits a request into chunks of 32 questions (servers cap questions per request).

## Layout
| path | what |
|---|---|
| `core/core.go` | config + profiles, System One client (`Ask`, `LastRaw`), `$VAR` runtime expansion, standing context, secret redaction, turn parser for Claude Code transcripts, Stop-hook verdict, settings.json hook template |
| `core/questions.json` | built-in neutral question pack for `judge` (ids: accepts, wanted_more, reaction, satisfaction) |
| `cmd/jevcli/main.go` | ask, judge, query, install / uninstall, hook, config, profile |
| `cmd/jevcli/ask.go` | ask (named + inline questions, one input or a parallel batch, verdicts, exit codes), question, defaults |
| `cmd/jevcli/skill/SKILL.md` | **the product**: the agent skill (is / which / ask / judge, when to reach for it, the trust rule). `jevcli install` writes it to ~/.claude, ~/.agents, ~/.codex; `jevcli skill` prints it |

Build: `go build -o bin/jevcli ./cmd/jevcli` (Go 1.26, stdlib only). Installed copy on the Mac:
`~/.local/share/jevcli/jevcli`, linked from `~/.local/bin/jevcli`.

## Config (`~/.config/jevcli/config.json`, user data, not in this repo)
- A profile = `url`, `model`, `headers`, optional `questions` (pack), `context` / `context_file`.
- `$VAR` / `${VAR}` is expanded at request time in every profile field and never written back; a missing var fails with
  its name. Keys live in the environment only (`--header "Authorization: Bearer $SOME_KEY"`); no secret broker needed.
- `default_profile`, `jevcli profile list | add | use | remove | show`.
- Global `context` / `context_file` + the profile's are prepended to every state as `Context: ...`.
- Legacy fields `endpoint` / `key_env` are still read (folded into `url` / `headers`).

## Hooks
`jevcli install` writes the skill and a Stop-hook template into Claude Code settings. **Disabled by default**
(`hooks.stop.enabled: false`): `hook run stop` exits at once. Verified: disabled 0.04 s no-op; shadow 0.07 s (scores in a
detached process, logs to `~/.local/share/jevcli/verdicts.jsonl`); block prints `{"decision":"block",...}`, never twice in
a row (`stop_hook_active`), fails open. `jevcli hook review` pairs verdicts with the user's next message.

## Verified recipes (2026-09-27, hosted Jev)
same (0.93 same), extract --kind date (due date, confidence 1.00), pick-func (refund_payment, 1.00), find (exactly the
two failure lines), verify, rank, tree, score, route (act/confirm/escalate, exit 0/10/20), samples (agreement), run.
A small personal model answered several of these wrongly: the recipes are only as good as the model behind the profile.

## Folder context + shortcuts (v0.6.0, 2026-09-27)
- `.jevcli/` (nearest from cwd up, like .git): `context.md` + `questions.json`; `--local` on `context set|add|clear` and
  `question add|remove`. Order: global, folder, profile, question, call.
- Shortcut verbs, all one question through cmdAsk (`cmdVerb` in ask.go): `is`, `pick`, `filter` (-v), `rank` (--top).
  Checked live in a nested folder: folder question crit -> yes 0.96; filter returned exactly the 2 failure lines.

## Context layers (v0.5.0, 2026-09-27)
`jevcli context show|set|clear [--profile P]`, `question add --context`, `ask|judge --context TEXT|@file` (repeat).
JSON states get a merged `"context"` field instead of a text prefix (the old prefix broke JSON). Live: the same message
0.84 no context / 0.94 "meeting in 20 min" / 0.64 "in three months".

## One command, all config (v0.4.0, 2026-09-27)
Owner direction: no fixed verbs (is / which / feels / match / recipes all removed). One `jevcli ask` with questions
inline (`--noul/--choice/--score`) or by NAME from config (`jevcli question add|list|show|remove`), every knob in
`core.Settings` with defaults (`jevcli defaults`, precedence builtin < config < profile < question < flag). Code:
`cmd/jevcli/ask.go`. Verdicts: yes/no/unsure, key/unsure, level/unsure. Checked live: urgent,team,sev on one ticket ->
yes 0.95 / billing 0.87 / high 0.99, exit 0.

## Reliability (v0.3.0, 2026-09-27), from the ecosystem scan
- Exit codes: 0 yes, 1 no, 3 unsure (`is` band 0.2/0.8, `which --min 0.6`), 4 any error. `die` exits 4, never 1.
- `core.AskRaw`: retries 429/5xx/network (3 tries, backoff), fail-closed validation (exact question ids, probs in
  [0,1] summing to 1, choice in offered keys), ledger `~/.local/share/jevcli/calls.jsonl` (no content), `jevcli stats`.
- Stop hook local pre-gate: judge only turns that edited files with no test/build/check after (`gate: "all"` = every
  turn). LastTurn keeps the LAST 25 actions. Tests: `go test ./...` (validation, retries, gate) with a fake server.
- Ecosystem notes (2026-09-27 scan): the leaders are jkudish/jev-mcp, Nasrallah-AL/jev-cli, shaharia-lab/jev-cli,
  valentynkit/jev-belay, tamaratran/fast-jev-compaction. Negative evidence: Jev second-guessing a strong model lost
  accuracy (statsguysam, 194 -> 173 / 200), so `judge` claims need our own eval first.

## Skill-first surface (v0.2.0, 2026-09-27)
The skill is the product and the site sells the skill. The CLI teaches three verbs: `is` (P, exit 0/1), `which` (a key)
and `ask` (JSONL over `--in` / `--lines` / `--states`, with `--is` / `--which` / `--score`), plus `judge`. The older names
(query, feels, match, recipes, `ask --option`) still work but are undocumented in the skill.

## Fuzzy control flow + batch (done 2026-09-27, verified on hosted Jev)
- `jevcli feels ADJ|QUESTION < text`: an if statement, exit 0/1 (outage 0.98 -> 0, newsletter 0.06 -> 1).
- `jevcli match KEY="desc" ... < text`: a switch, prints the key ("charged twice" -> billing, 1.00).
- `jevcli query --states FILE.jsonl --parallel N [--questions set.json]`: 20 states in 8.2 s sequential vs 1.5 s at
  --parallel 8; JSONL out in input order, a bad line reports its error and the exit code is 1. `core.AskRaw` is race-free.

## Distribution (2026-09-27)
Repo public. Site https://muthuishere.github.io/jevcli/ (`docs/`, `pages.yml`). Tag `v*` -> `release.yml` builds 6 binaries.
`install.sh` / `install.cmd` -> binary + `jevcli install` (skills in ~/.claude, ~/.agents, ~/.codex if present; Stop hook
template, disabled). Verified end to end: macOS arm64 (curl|sh), Linux (alpine docker), Windows arm64 (agentbus, a home
path with a space; reinstall idempotent, uninstall clean).

## Next
- `query --json-in`: accept a full System One request body and stream answers as JSONL.
- Next up: `jevcli eval` (replay transcripts / labelled JSONL -> pick thresholds), frozen-answer offline tests, `ask --resume` + cache, redaction before send, PreToolUse guard template, fallback profiles.
- Tests (more): table tests for `State` trimming, `Redact`, `$VAR` expansion / `MissingEnv`, the transcript parser, settings.json
  install/uninstall (backup + only our entry), and a fake System One server for every command.
- Hooks beyond Stop (PreToolUse guardrail via a noul, UserPromptSubmit routing), each a template, all disabled by default.
- Codex hooks (only the skill is installed for Codex today).
- Release: goreleaser (darwin/linux, arm64/amd64), `jevcli version`, install script.
- README: a short "recipes cookbook" page with one real example per recipe.

## Known gaps
- `judge` trims turns by a character budget (~1600 chars), not exact tokens; the server truncates a long premise too.
- `find` and `rank` still send one request per line / candidate sequentially; port them onto `askManyErr` + a worker pool.
- `run` with more than 32 questions returns merged answers but `--raw` only shows the last chunk's body.
- `extract --kind` regexes are simple (no relative dates like "next Friday").
- The secret scanner of this machine flags the word used by a DB setting (`disable`) as a live value: a false positive.
