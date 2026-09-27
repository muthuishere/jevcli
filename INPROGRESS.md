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
| `cmd/jevcli/recipes.go` | cookbook recipes: verify, same, rank, find, extract, tree, score, pick-skill, pick-func, run; `ask --route`, `ask --samples` |
| `cmd/jevcli/skill.md` | the agent skill `jevcli install` writes into Claude Code and Codex skill dirs |

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

## Next
- Parallel query ergonomics: `--state` from a JSONL file with one state per line (same questions for many states,
  concurrent requests with a limit), and a question-set file (`--questions set.json`) so a batch reuses one set.
- `query --json-in`: accept a full System One request body and stream answers as JSONL.
- Tests: table tests for `State` trimming, `Redact`, `$VAR` expansion / `MissingEnv`, the transcript parser, settings.json
  install/uninstall (backup + only our entry), and a fake System One server for every command.
- Hooks beyond Stop (PreToolUse guardrail via a noul, UserPromptSubmit routing), each a template, all disabled by default.
- Codex hooks (only the skill is installed for Codex today).
- Release: goreleaser (darwin/linux, arm64/amd64), `jevcli version`, install script.
- README: a short "recipes cookbook" page with one real example per recipe.

## Known gaps
- `judge` trims turns by a character budget (~1600 chars), not exact tokens; the server truncates a long premise too.
- `find` and `rank` send one request per line / candidate (sequential): slow on big files until the parallel batch above.
- `run` with more than 32 questions returns merged answers but `--raw` only shows the last chunk's body.
- `extract --kind` regexes are simple (no relative dates like "next Friday").
- The secret scanner of this machine flags the word used by a DB setting (`disable`) as a live value: a false positive.
