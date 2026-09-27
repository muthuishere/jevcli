# jevx

**An agent skill for Claude Code, Codex and any agent: fast judgements from a Jev-style decision model.** The agent hands
small calls (is this urgent, which bucket, same customer, is this line an error) to a model that answers in about 0.4 s
with a number or a key, and saves its own reasoning for the real work. The skill lives in
[`cmd/jevx/skill/SKILL.md`](cmd/jevx/skill/SKILL.md). It calls a small, vendor-neutral CLI:

```bash
jevx is "Is this urgent?" < email.txt                                   # yes 0.95  (exit 0 yes, 1 no, 3 unsure, 4 error)
jevx pick "Which team?" web=frontend api=backend --in "charged twice"    # api 0.9
jevx filter "Is this an error?" < app.log                                # grep by meaning
jevx rank "Is this about refunds?" --top 5 < results.txt                 # best first
jevx ask --noul urgent="Is this urgent?" < email.txt                     # the full command: many questions, batches
jevx question add team --choice "Which team?|web=frontend;api=backend"   # save a question once
jevx ask urgent,team < ticket.txt                                        # ask saved questions by name
jevx ask --lines app.log --noul err="Is this an error?"                  # batch: JSONL, one line per input
jevx defaults                                                            # every setting, and where it comes from
jevx judge --request "..." --proposal "..."                             # would the user accept this turn?
```
Site: https://muthuishere.github.io/jevx/

## Install
```bash
npm i -g @muthuishere/jevx                                                                                     # any OS with Node 18+
curl -fsSL https://muthuishere.github.io/jevx/install.sh | sh                                   # macOS / Linux
curl -fsSLo install.cmd https://muthuishere.github.io/jevx/install.cmd && install.cmd          # Windows
```
This installs the binary, puts the agent skill in `~/.claude/skills` and `~/.agents/skills` (and `~/.codex/skills` if
present), and adds the Claude Code Stop-hook template, which stays disabled. `JEVX_NO_HOOK=1` installs the skills only. Later: `jevx install [--skills] [--hooks]` and `jevx uninstall
[--skills] [--hooks]` (neither flag = both).
Releases are cut by pushing a `v*` tag (`.github/workflows/release.yml`). The site is `docs/`, published by `pages.yml`.

## ask
Questions: saved names (`jevx question add|list|show|remove`), inline `--noul NAME="Q"`, `--choice NAME="Q|k=d;k2=d"`,
`--score NAME="Q|low;mid;high"`, or `--questions set.json`. Inputs: stdin, `--in`, `--lines FILE` or `--states FILE.jsonl`.
One input prints `NAME VERDICT P` per question (`--json`, `--raw`) and exits 0 yes/decided, 1 no, 3 unsure, 4 error. A
batch prints JSONL in input order (20 tickets: 8.2 s sequentially, 1.5 s at 8 parallel).

## Context
jevx only reads context; you edit it. It sends, in order:
1. The `## Jev` section of your global agent files (`$CLAUDE_CONFIG_DIR/CLAUDE.md`, `~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md`, `~/.agents/AGENTS.md`,
   `~/AGENTS.md`), merged with repeats dropped (synced copies count once).
2. The `## Jev` section of the nearest folder's `AGENTS.md` and/or `CLAUDE.md` from the working directory up (both merged,
   repeats dropped; `--cwd DIR` to choose). `#jev`, `## Jev` and `### Jev notes` all count; an empty section is fine.
3. `--context TEXT|@file` on the call (repeatable).

`--no-context` skips 1 and 2. `jevx context` shows what is sent and from which file. A saved question can carry its own
`--context`. A JSON input keeps its shape: the context is merged into its `"context"` field. The heading name and files
are configurable (`local_context_section`, `local_context_file`, `global_context_file`).

## Plugins (hooks as config)
`plugins` in the config (or a repo's `.jevx/plugins.json`): `{"on": "PreToolUse:Bash", "ask": "destroys,remote",
"deny": "destroys >= 0.8", "warn": "remote >= 0.5"}`. One generic runner (`jevx hook run EVENT`) serves every event;
`install --hooks` writes one settings entry per event. Shipped, all disabled: `stop-judge`, `bash-guard`,
`injection-screen`, `route`. `jevx plugin list | show | add | remove | enable [--act] | disable | mode | test | log`.
`exec` hands an event to any command. Claude Code's hook protocol only, for now.

## Settings
`jevx defaults [set|unset KEY VALUE] [--profile P]`: `yes` 0.8, `no` 0.2, `min_confidence` 0.6, `parallel` 8,
`retries` 3, `timeout_s` 60, `chunk` 32, `ledger` true, `accept_min` 0.35, `more_max` 0.65. The order of precedence is
built-in < config < profile < question < flag. Every call is logged without content (`jevx stats`).

The context lives in the question: the instructions and each option or criterion say whose decision it is and what is
judged. `judge` asks four turn-level questions from a **question pack** (JSON). The built-in pack is neutral; a profile can
point to its own pack (`"questions": "/path/pack.json"`), which matters for a model trained on specific wording.

## Profiles
`~/.config/jevx/config.json`: named profiles and a `default_profile`. A profile is `url`, `model`, `headers`, plus
optional `questions` (a question pack) and `context` / `context_file` (standing context prepended to every state).
`$VAR` / `${VAR}` in any of these is expanded from the environment at request time and never written back, so secrets
stay in the environment: `--header "Authorization: Bearer $JEV_API_KEY"`.

```bash
jevx profile add hosted https://example.com/v1/systemone --model some-model --header "Authorization: Bearer $JEV_API_KEY"
jevx profile add local 'http://$JEV_HOST/v1/systemone' --model my-model --context "Decisions are for the platform team."
jevx profile use local        # the default profile
jevx profile list
```

## Cookbook recipes
`ask` (classify / route; `--route ACT,CONFIRM` for confidence-gated routing, `--samples N` for self-consistency),
`verify` (citations, fact checks), `same` (entity alignment, dedupe), `rank` (re-ranking, composite scores), `find`
(semantic grep), `extract` (pre-parsed value extraction: dates, numbers, money, emails, urls), `tree` (hierarchical
classification), `score` (ordinal rating), `pick-skill` (agent skill suggestion), `pick-func` (function calling),
`run` (raw requests: batching, speculative fan-out, structure recovery). Each is plain System One questions.

## Agents
`jevx install` writes a skill for Claude Code and Codex, and adds a Stop-hook template to Claude Code's settings.
**Hooks are disabled by default** (`hooks.stop.enabled: false`): the template does nothing until `jevx hook enable stop`.
`jevx hook mode shadow|block`: shadow scores each finished agent turn in the background and logs it; block sends the
agent back with a reason when the turn would likely not be accepted (never twice in a row; fails open).
`jevx hook review` lines logged verdicts up with what the user said next. `jevx uninstall` removes the skill and the
hook template and leaves other hooks untouched.

## Build
`go build -o bin/jevx ./cmd/jevx` (Go 1.26+, no dependencies).
