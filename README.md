# jevx

**Give your coding agent a fast, typed "gut feeling": yes/no, pick-one and rating answers from a Jev-style decision
model, as an agent skill for Claude Code, Codex and any agent, backed by one small CLI.**

[![Release](https://img.shields.io/github/v/release/muthuishere/jevx?sort=semver&color=00a86b)](https://github.com/muthuishere/jevx/releases/latest)
[![npm](https://img.shields.io/npm/v/@muthuishere/jevx?logo=npm&label=npm)](https://www.npmjs.com/package/@muthuishere/jevx)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**[muthuishere.github.io/jevx](https://muthuishere.github.io/jevx/)**: docs, guides and the full CLI reference.

Coding agents spend their own (expensive) reasoning on small calls all day: is this log line an error, which team owns
this ticket, is this command safe to run, did I actually finish what the user asked. jevx hands those calls to a
System One decision model (hosted Jev, a self-hosted model, or your own) that answers in well under a second with a
number or a key, never prose. The agent keeps its reasoning for the real work.

## Why

1. **Agent skill first.** One install puts the `jevx` skill into Claude Code, Codex and `~/.agents`, so agents know when
   to reach for it (triage, classify, route, dedupe, rank, check a claim, "would the user accept this?").
2. **Honest answers.** Every answer is `yes` / `no` / **`unsure`**, a key or a level, with its probability. Exit codes
   are the contract: `0` yes, `1` no, `3` unsure, `4` error. A network failure is never read as "no".
3. **Your questions, your context.** Save a question once (with its own thresholds) and ask it by name from any agent,
   any repo. Context comes from a `## Jev` section in the `AGENTS.md` / `CLAUDE.md` you already keep; jevx only reads it.
4. **Hooks as config.** Plugins turn any agent event into a judgement: refuse `rm -rf` before it runs, check a turn
   before it is handed back, flag prompt injection in fetched pages, route simple requests to a smaller model. A plugin
   is three lines of JSON; they ship disabled and start in shadow mode.
5. **Any endpoint, nothing hidden.** Any System One URL works (keys stay in environment variables, expanded only when a
   request is sent). Replies are validated, 429/5xx are retried, and every call is logged without its content.

## Install

```bash
npm i -g @muthuishere/jevx                                              # any OS with Node 18+
curl -fsSL https://muthuishere.github.io/jevx/install.sh | sh             # macOS / Linux
curl -fsSLo install.cmd https://muthuishere.github.io/jevx/install.cmd && install.cmd   # Windows
go install github.com/muthuishere/jevx/cmd/jevx@latest                  # from source
```

Every channel installs the binary from [the latest release](https://github.com/muthuishere/jevx/releases/latest)
(macOS / Linux / Windows, amd64 + arm64), then runs `jevx install`: the skill goes into `~/.claude/skills`,
`~/.agents/skills` (and `~/.codex/skills` if present), and hook entries are registered with every plugin **off**.
`JEVX_NO_HOOK=1` installs the skills only. Later: `jevx install|uninstall [--skills] [--hooks]`.

Point it at your endpoint (the key stays in your environment):

```bash
jevx profile add jev https://your-endpoint/v1/systemone --model MODEL --header "Authorization: Bearer $YOUR_KEY_VAR"
```

## Quickstart

```console
$ echo "Checkout is down, customers are being charged twice" | jevx is "Is this urgent?"
yes 0.98

$ jevx pick "Which team?" web=frontend api=backend billing=money --in "I was charged twice"
billing 0.97

$ jevx filter "Is this an error or failure?" < app.log
ERROR payment gateway timeout after 30s
FATAL db connection refused

$ jevx rank "Is this about money?" --top 2 < docs.txt
0.97  Refund policy for annual plans
0.86  Pricing of the enterprise plan
```

`is`, `pick`, `filter` and `rank` are shortcuts over one command, `jevx ask`, which takes many questions at once, over
one input or a parallel batch:

```console
$ jevx question add urgent --noul "Is this urgent for the person receiving it?" --yes 0.85
$ jevx question add team --choice "Which team?|web=frontend;api=backend;billing=money"
$ jevx question add sev --score "How severe is this?|low;medium;high"

$ echo "Checkout is down, customers are being charged twice" | jevx ask urgent,team,sev
sev              high       0.99
team             billing    0.87
urgent           yes        0.95

$ jevx ask --lines app.log --noul err="Is this an error?" --parallel 8     # JSONL, one line per input, in order
```

## Context: from the files you already keep

```markdown
<!-- AGENTS.md or CLAUDE.md, in your home folder (global) or in a repo (folder) -->
## Jev

This repo is a payments service; any money-movement bug is critical.
```

jevx sends the global section, then the nearest folder's, then `--context` on the call (`--no-context` skips the
sections, `--cwd DIR` picks the folder). `jevx context` shows exactly what is sent and from which file. The same
"refund issued twice" report went from `unsure 0.74` to `yes 0.93` once the repo's section said money bugs are critical.

## Plugins: judgements at agent events

```json
"bash-guard": {"on": "PreToolUse:Bash", "ask": "destroys,remote,irreversible",
               "deny": "destroys >= 0.8 && irreversible >= 0.6", "warn": "destroys >= 0.5 || remote >= 0.8",
               "enabled": false}
```

```console
$ jevx plugin list                       # stop-judge, bash-guard, injection-screen, route: all off
$ jevx plugin test bash-guard "rm -rf / --no-preserve-root"     # decision: deny (destroys 0.91, irreversible 0.87)
$ jevx plugin enable bash-guard          # shadow: logs what it would do
$ jevx plugin mode bash-guard act        # now it refuses for real
$ jevx plugin log bash-guard             # every decision, with the answers
```

In a real Claude Code session the guard refused `rm -rf ./junk` and Claude reported: *"The command was blocked by the
jevx bash-guard PreToolUse hook... I did not retry."* `exec` hands an event to any other tool instead, so a check
someone else built can be plugged in without a jevx release. Plugins speak Claude Code's hook protocol today.

## What it is not

- **Not a text generator.** Jev picks and scores; your agent (or any LLM) still writes the code and the replies.
- **Not a safety boundary.** Guards reduce accidents; permissions, money and irreversible calls stay with you.
- **Not calibrated truth.** The thresholds are sensible defaults. Run plugins in shadow mode and read
  `jevx plugin log` before letting them act. Every output above is real, from hosted Jev (`jev-1.13.0`); a smaller
  model can answer differently.

## Develop

`go build -o bin/jevx ./cmd/jevx` (Go 1.26, standard library only) · `go test ./...` · releases: push a `v*` tag, and
the binaries go to GitHub Releases and the npm package is published with provenance.

## License

[MIT](LICENSE)
