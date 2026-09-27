# jevcli

**An agent skill for Claude Code, Codex and any agent: fast judgements from a Jev-style decision model.** The agent hands
small calls (is this urgent, which bucket, same customer, is this line an error) to a model that answers in about 0.4 s
with a number or a key, and saves its own reasoning for the real work. The skill lives in
[`cmd/jevcli/skill/SKILL.md`](cmd/jevcli/skill/SKILL.md). It calls a small, vendor-neutral CLI:

```bash
jevcli ask --noul urgent="Is this urgent?" < email.txt                   # urgent  yes  0.95  (exit 0/1/3/4)
jevcli question add team --choice "Which team?|web=frontend;api=backend"   # save a question once
jevcli ask urgent,team < ticket.txt                                        # ask saved questions by name
jevcli ask --lines app.log --noul err="Is this an error?"                  # batch: JSONL, one line per input
jevcli defaults                                                            # every setting, and where it comes from
jevcli judge --request "..." --proposal "..."                             # would the user accept this turn?
```
Site: https://muthuishere.github.io/jevcli/

## Install
```bash
curl -fsSL https://muthuishere.github.io/jevcli/install.sh | sh                                   # macOS / Linux
curl -fsSLo install.cmd https://muthuishere.github.io/jevcli/install.cmd && install.cmd          # Windows
```
This installs the binary, puts the agent skill in `~/.claude/skills` and `~/.agents/skills` (and `~/.codex/skills` if
present), and adds the Claude Code Stop-hook template, which stays disabled. `JEVCLI_NO_HOOK=1` installs the skills only.
Releases are cut by pushing a `v*` tag (`.github/workflows/release.yml`). The site is `docs/`, published by `pages.yml`.

## ask
Questions: saved names (`jevcli question add|list|show|remove`), inline `--noul NAME="Q"`, `--choice NAME="Q|k=d;k2=d"`,
`--score NAME="Q|low;mid;high"`, or `--questions set.json`. Inputs: stdin, `--in`, `--lines FILE` or `--states FILE.jsonl`.
One input prints `NAME VERDICT P` per question (`--json`, `--raw`) and exits 0 yes/decided, 1 no, 3 unsure, 4 error. A
batch prints JSONL in input order (20 tickets: 8.2 s sequentially, 1.5 s at 8 parallel).

## Settings
`jevcli defaults [set|unset KEY VALUE] [--profile P]`: `yes` 0.8, `no` 0.2, `min_confidence` 0.6, `parallel` 8,
`retries` 3, `timeout_s` 60, `chunk` 32, `ledger` true, `accept_min` 0.35, `more_max` 0.65. The order of precedence is
built-in < config < profile < question < flag. Every call is logged without content (`jevcli stats`).

The context lives in the question: the instructions and each option or criterion say whose decision it is and what is
judged. `judge` asks four turn-level questions from a **question pack** (JSON). The built-in pack is neutral; a profile can
point to its own pack (`"questions": "/path/pack.json"`), which matters for a model trained on specific wording.

## Profiles
`~/.config/jevcli/config.json`: named profiles and a `default_profile`. A profile is `url`, `model`, `headers`, plus
optional `questions` (a question pack) and `context` / `context_file` (standing context prepended to every state).
`$VAR` / `${VAR}` in any of these is expanded from the environment at request time and never written back, so secrets
stay in the environment: `--header "Authorization: Bearer $JEV_API_KEY"`.

```bash
jevcli profile add hosted https://example.com/v1/systemone --model some-model --header "Authorization: Bearer $JEV_API_KEY"
jevcli profile add local 'http://$JEV_HOST/v1/systemone' --model my-model --context "Decisions are for the platform team."
jevcli profile use local        # the default profile
jevcli profile list
```

## Cookbook recipes
`ask` (classify / route; `--route ACT,CONFIRM` for confidence-gated routing, `--samples N` for self-consistency),
`verify` (citations, fact checks), `same` (entity alignment, dedupe), `rank` (re-ranking, composite scores), `find`
(semantic grep), `extract` (pre-parsed value extraction: dates, numbers, money, emails, urls), `tree` (hierarchical
classification), `score` (ordinal rating), `pick-skill` (agent skill suggestion), `pick-func` (function calling),
`run` (raw requests: batching, speculative fan-out, structure recovery). Each is plain System One questions.

## Agents
`jevcli install` writes a skill for Claude Code and Codex, and adds a Stop-hook template to Claude Code's settings.
**Hooks are disabled by default** (`hooks.stop.enabled: false`): the template does nothing until `jevcli hook enable stop`.
`jevcli hook mode shadow|block`: shadow scores each finished agent turn in the background and logs it; block sends the
agent back with a reason when the turn would likely not be accepted (never twice in a row; fails open).
`jevcli hook review` lines logged verdicts up with what the user said next. `jevcli uninstall` removes the skill and the
hook template and leaves other hooks untouched.

## Build
`go build -o bin/jevcli ./cmd/jevcli` (Go 1.26+, no dependencies).
