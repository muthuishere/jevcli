---
title: Introduction
description: What jevx is, what problem it solves for coding agents, and what it deliberately does not do.
---

**jevx** lets a coding agent hand small decisions to a Jev-style decision model and get back a typed verdict with a probability. Is this urgent? Which team owns this? Is this log line an error? The agent gets `yes 0.92` or `billing 0.99` and an exit code it can branch on, and keeps its own reasoning for the real work.

It is two things:

- **A CLI**, `jevx`, a single Go binary with no runtime dependencies.
- **An agent skill**, `SKILL.md`, installed into Claude Code, Codex and `~/.agents`, that tells the agent when to reach for the CLI and how to read its answers.

Any **System One** endpoint works: the hosted Jev model, a self-hosted one, or a personal model on `127.0.0.1`. A profile is a URL, a model name and headers.

## Why hand a decision to another model

Because the small calls add up. Deciding whether each of 300 log lines is an error costs the agent context and tokens for every line. One batch call gives a number per line that the agent can filter on, and a number is something an `if` can use.

```bash title="Terminal"
jevx filter "Is this a failure an on-call engineer would act on?" < app.log
```

The agent then works on the lines that matched instead of reading the whole file, and says so in its report ("jevx flagged 1 of 3 lines").

## Repeat calls are free

jevx keeps the model's answer, keyed by a hash of the endpoint, model, input and question. Asking the same thing about the same input again returns in milliseconds (about 7 ms against 350 ms) and sends nothing. It is on by default; `--fresh` asks again and `jevx cache disable` turns it off. Only answers are stored, never the input. See [the answer cache](/jevx/reference/privacy/#the-answer-cache).

## The shape of an answer

| question type | you ask | you get |
| --- | --- | --- |
| yes/no (`noul`) | `--noul NAME="QUESTION"` | `yes`, `no` or `unsure`, plus P(yes) |
| pick-one | `--choice NAME="QUESTION\|key=desc;key2=desc"` | one key, or `unsure` |
| rating | `--score NAME="QUESTION\|low;mid;high"` | one level, or `unsure` |

"noul" is the endpoint's name for a yes/no question, and the CLI keeps it.

## What it deliberately is not

- Not a safety gate: permissions, money, legal and irreversible actions stay with you.
- Not a generator: it never writes code, replies or files.
- Not an explanation: you get a verdict and a number, not a reason.
- Not calibrated for you yet: the thresholds are defaults; check a few verdicts before you trust it in a loop.

Next: [Getting started](/jevx/start/getting-started/).
