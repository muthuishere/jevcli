---
title: Using it from an agent
description: What the installed SKILL.md tells Claude Code, Codex and any ~/.agents reader to do with jevx, and what it tells them never to do.
---

The skill (`SKILL.md`) is what the agent reads. It says when to reach for jevx, how to word a question so it stands alone, what each exit code means, and what never to do with it.

## Where it goes

`jevx install --skills` copies `SKILL.md` to:

- `~/.claude/skills/jevx/` (or `$CLAUDE_CONFIG_DIR/skills/jevx/` when that variable is set)
- `~/.agents/skills/jevx/`, the vendor-neutral location
- `~/.codex/skills/jevx/`, if `~/.codex` exists

Claude Code and Codex load a skill when its `description` matches the task. `jevx skill` prints the installed copy; the published one is at [skill.md](/jevx/skill.md).

## What the agent is told

- **When to reach for it:** classifying, filtering, triaging, routing, deduping, ranking or checking many items; when a branch depends on meaning; before asking the user "A or B?"; before handing back a turn.
- **The forms:** `is` / `pick` / `filter` / `rank`, then `ask` with saved or inline questions over one input or a batch.
- **Exit codes:** `0` act, `1` other branch, `3` check or ask, `4` say it failed. An error is never a no.
- **Context:** read `## Jev` from the user's agent files; pass facts with `--context`; tell the user which file to edit, never edit it.
- **Repeat calls:** answers are cached (on by default), so the same call again is instant and free; use `--fresh` when an answer must be re-asked, and say in the report when a result came from the cache.
- **Saved questions:** check `question list` first; save per repo with `--local` when asked.
- **Reporting:** word the question so it stands alone, and report when a verdict changed what it did: "jevx flagged 12 of 300 lines".
- **Never:** use it as a safety gate, put secrets into `--in` or `--proposal`, or train another model on a hosted endpoint's answers unless its terms allow it.

## The table the agent uses

| you are about to… | do this instead |
| --- | --- |
| read 300 lines to find the failures | `ask --lines log --noul fail="Is this a failure an on-call engineer would act on?"` |
| sort tickets into buckets | `ask --states t.jsonl --choice team="Which team?\|web=UI;api=backend;ops=infra"` |
| decide if two records are one entity | `printf 'A: %s\nB: %s' "$a" "$b" \| jevx ask --noul same="Are A and B the same customer?"` |
| check a claim against a source | `jevx ask --noul ok="Does the text say payment is due in October?" < invoice.txt` |
| rank search results | `ask --lines results.txt --noul hit="Does this answer: how do I get a refund?"`, then sort by `p` |
| ask the user "A or B?" | `ask --in "<the facts>" --choice pick="Which would the user choose?\|a=<A>;b=<B>"`; ask only on exit 3 |
| hand back your turn | `jevx judge --request "<request>" --proposal "<final message>"` |

## A session, end to end

The user asks an agent to find what broke in last night's log. Instead of reading it, the agent runs one batch:

```bash title="Terminal"
jevx filter "Is this a failure an on-call engineer would act on?" < app.log
```

On the three-line sample from [Ask](/jevx/guides/ask/), the same kind of batch picks line 2 (the gateway timeout, P 0.98) and nothing else. The agent reads that line, fixes the cause, and says in its report that jevx picked 1 of 3 lines and it acted on that one. If the call had exited `4`, the skill says to report the failure, not to guess.

## Other agents

Anything that reads `~/.agents/skills` gets the same file. [Plugins](/jevx/guides/plugins/) (hooks) are Claude Code only for now. `jevx uninstall --skills` removes exactly the copies it wrote and leaves other skills untouched.
