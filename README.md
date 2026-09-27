# jevcli

A small, vendor-neutral CLI for **System One / Jev-style decision models**: models that read a state and answer typed
questions (Noul = a probability, Choice = one of several options, Score = a level) instead of writing text. Point it at any
compatible endpoint (hosted, self-hosted or a personal model) and use it from the shell or from coding agents.

## Install
```bash
curl -fsSL https://muthuishere.github.io/jevcli/install.sh | sh                                   # macOS / Linux
curl -fsSLo install.cmd https://muthuishere.github.io/jevcli/install.cmd && install.cmd          # Windows
```
This installs the binary, puts the agent skill in `~/.claude/skills` and `~/.agents/skills` (and `~/.codex/skills` if
present), and adds the Claude Code Stop-hook template, which stays disabled. `JEVCLI_NO_HOOK=1` installs the skills only.
Releases are cut by pushing a `v*` tag (`.github/workflows/release.yml`). The site is `docs/`, published by `pages.yml`.

```bash
jevcli config set-endpoint default https://your-endpoint/v1/systemone MODEL [KEY_ENV]
jevcli ask "Which should the agent do next?" --context "..." --option "stop=Stop and report" --option "run=Go ahead"
jevcli ask "The agent wants to force-push to main." --context "..." --true "The user allows it." --false "The user stops it."
jevcli judge --request "the user's request" --proposal "the agent's final message" --action "Bash: go test ./..."
```

The native call, one state and many named questions, answered in one request:

```bash
jevcli query --state '{"role": "You are playing Super Smash Bros and receive messages from a teammate", "message": "go for the ledge"}' \
  --noul is_appropriate="Does the message contain inappropriate language or topics?" \
  --noul does_this_help="Does this help donkey kong win?" --raw
# {"answers": {"does_this_help": {"noul": 0.75, ...}, "is_appropriate": {"noul": 0.06, ...}}, "model": "jev-1.13.0", "usage": {...}}
```
`--choice NAME="INSTRUCTIONS|key=desc;key2=desc"` and `--score NAME="INSTRUCTIONS|level0;level1"` mix in other types.

### An if statement, a switch, and a batch
```bash
if jevcli feels urgent < email.txt; then claude -p "Draft a reply" < email.txt; fi   # exit 0 = yes, 1 = no
case $(jevcli match billing="about money" bug="a defect report" other="anything else" < msg.txt) in
  billing) ... ;; bug) ... ;; esac
jevcli query --states tickets.jsonl --parallel 8 --noul urgent="Is this urgent?" | jq -c 'select(.answers.urgent.noul > .7)'
```
`--states` takes one JSON object or JSON string per line and prints one JSONL answer line per state, in input order
(20 tickets: 8.2 s sequential, 1.5 s at `--parallel 8`). `--questions set.json` reuses a saved `{NAME: question}` set.

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
