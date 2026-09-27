---
title: Compared with other Jev tools
description: What the other Jev CLIs, MCP servers, skills and hooks do, where each one is the better pick, and where jevx differs.
---

The Jev ecosystem is a few weeks old and moving fast. This page compares **what the tools do**, from their own READMEs
as of 2026-09-27. Stars are a snapshot and mostly show who got listed early. If a tool below fits you better, use it.

## At a glance

| | jevx | [jev-mcp](https://github.com/jkudish/jev-mcp) | [jev-cli (Nasrallah)](https://github.com/Nasrallah-AL/jev-cli) | [jev-cli (shaharia)](https://github.com/shaharia-lab/jev-cli) | [hermes-jev-skills](https://github.com/kerpopule/hermes-jev-skills) |
|---|---|---|---|---|---|
| Form | agent skill + CLI | MCP server | CLI + Claude Code plugin | CLI | skills + Hermes plugin |
| Stars | new | 409 | 22 | 30 | 862 |
| Installs into Claude Code, Codex, `~/.agents` | yes, one command | via MCP config | Claude Code plugin | no | Hermes, Claude Code, Codex |
| Works with no config (key in env) | yes | yes | key in OS keychain | yes | key in Keychain |
| Any System One endpoint (local, self-hosted) | yes, profiles | TypeSafe / OpenRouter / Cloudflare / Vercel | TypeSafe | TypeSafe | TypeSafe |
| `unsure` verdict and its own exit code | yes (exit 3) | act / review bands | `--fail-on` (exit 2) | abstain band (exit 11) | shadow / on |
| Error never read as "no" | yes (exit 4) | typed errors | not stated | exit 10 = false, errors separate | not stated |
| Parallel batch over files / JSONL | yes | batch tools | `batch` with a pool | batch with back-off, `--resume` | per skill |
| Saved questions with their own thresholds | yes (global or per repo) | no | no | YAML question files | no |
| Context from your AGENTS.md / CLAUDE.md | yes (`## Jev` section) | no | no | no | memory filtering |
| Hooks as config (guard, turn check, routing) | yes, plugins, off by default | `gate` / `review` tools | `compact` plugin | no | routing, screening |
| Reply validation (ids, probabilities) | yes, fails closed | yes, fails closed | not stated | not stated | not stated |
| Call log without content | yes | not stated | not stated | cost footer per call | dashboard |

"Not stated" means the README does not say; it may still exist.

## Where each one is the better pick

- **You live in MCP clients (Claude Desktop, Cursor, anything MCP):** [jkudish/jev-mcp](https://github.com/jkudish/jev-mcp).
  Eleven ready-made tools (verify, screen, rerank, extract, review, gate…), provider fallback across TypeSafe,
  OpenRouter, Cloudflare and Vercel, and strict reply validation. jevx has no MCP server.
- **You want the most task verbs in a CLI:** [Nasrallah-AL/jev-cli](https://github.com/Nasrallah-AL/jev-cli) has
  `verify`, `screen`, `classify`, `extract`, `rerank`, `route`, `compact` and more, with table / markdown / CSV output
  and the key in the OS keychain.
- **You run large batch jobs:** [shaharia-lab/jev-cli](https://github.com/shaharia-lab/jev-cli) has `--resume`,
  back-off, `--dry-run` with a cost estimate and YAML question files validated offline. jevx batches restart from zero.
- **You use Hermes:** [kerpopule/hermes-jev-skills](https://github.com/kerpopule/hermes-jev-skills) is the most
  complete skill set, with the best key handling (a one-time local page writes it to Keychain) and published notes on
  what leaves your machine.
- **You only want one guard:** [jev-airbag](https://github.com/velvetchief/jev-airbag) (PreToolUse for Bash) and
  [jev-belay](https://github.com/valentynkit/jev-belay) (Stop, with a published AUROC on 100 labelled stops) do one
  thing each, with measured results.
- **You want to shrink context:** [fast-jev-compaction](https://github.com/tamaratran/fast-jev-compaction) (7,003
  stars) prunes stale tool calls instead of summarising. jevx does not do compaction; a plugin can `exec` it.
- **You want no hosted model at all:** [jevless](https://github.com/jbpayton/jevless) serves the same API from any
  model with logprobs, and [open-jev](https://github.com/Shalimov04/open-jev) distils a small local classifier. Point
  a jevx profile at either.

## Where jevx is different

- **Agent first.** The product is the skill: one install teaches Claude Code, Codex and `~/.agents` when to use it,
  how to word a question, and what to do with each answer, with [18 real scenarios](/jevx/guides/scenarios/).
- **One engine, everything configurable.** `ask` plus four shortcuts; every threshold, retry and timeout in one
  settings system (built-in < config < profile < question < flag).
- **Your context, read only.** The `## Jev` section of the agent files you already keep, global and per repo.
- **Hooks as config.** A plugin is a few JSON fields; `exec` plugs in any tool above without a jevx release.
- **Honest by default.** `unsure` is an answer, errors never look like "no", and plugins start in shadow mode.

## What jevx does not have yet

An MCP server, `--resume` for batches, a cost estimate before a run, keychain storage for keys, an `eval` command
that picks thresholds from your own labelled data, and hook support for agents other than Claude Code.
