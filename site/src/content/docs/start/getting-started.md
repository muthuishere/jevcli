---
title: Getting started
description: Install jevx, point it at an endpoint, and ask your first question. About two minutes.
---

## 1. Install

```bash title="npm (macOS, Linux, Windows)"
npm i -g @muthuishere/jevx
```

```bash title="macOS / Linux"
curl -fsSL https://muthuishere.github.io/jevx/install.sh | sh
```

```bat title="Windows"
curl -fsSLo install.cmd https://muthuishere.github.io/jevx/install.cmd && install.cmd
```

Each one puts the `jevx` binary on your PATH, copies the agent skill into `~/.claude/skills`, `~/.agents/skills` and `~/.codex/skills` (if that folder exists), and adds the Claude Code hook entries, which stay disabled. `JEVX_NO_HOOK=1` installs the skills only. Details in [Install](/jevx/start/install/).

## 2. Point it at an endpoint

A profile is a URL, a model name and headers. `$VAR` inside any of them is expanded from your environment at request time and never written to disk, so the key stays out of the config file.

```bash title="Terminal" "$YOUR_KEY_VAR"
jevx profile add jev https://your-endpoint/v1/systemone --model MODEL --header "Authorization: Bearer $YOUR_KEY_VAR"
```

```console
$ jevx profile list
* jev        https://api.typesafe.ai/v1/systemone  model=jev-latest  Authorization: Bearer $TYPESAFE_API_KEY
  myjev      http://127.0.0.1:21131/v1/systemone  model=myjev
(* = default; change with: jevx profile use NAME)
```

The first profile you add becomes the default. `jevx profile use NAME` changes it; `--profile NAME` overrides it for one call. A local model needs no header at all (the `myjev` line above). The key is listed as the literal `$TYPESAFE_API_KEY`, never its value.

## 3. Ask something

```console
$ echo "Prod is down" | jevx is "Is this urgent?"; echo "exit $?"
yes 0.92
exit 0
```

The word is the verdict, the number is P(yes). Exit `0` means yes, `1` no, `3` unsure, `4` an error. See [Ask](/jevx/guides/ask/) for every form.

## 4. Let the agent use it

Nothing more to do. The skill is installed, and Claude Code or Codex loads it when a task matches its description (classify, triage, filter, rank, "is this X", "which of these"). [Using it from an agent](/jevx/guides/agents/) shows what the agent is told.

## Where to go next

- [Shortcuts](/jevx/guides/shortcuts/): `is`, `pick`, `filter`, `rank`.
- [Saved questions](/jevx/guides/questions/): write a question once, reuse it by name.
- [Context](/jevx/guides/context/): tell the model whose decision it is.
- [CLI reference](/jevx/reference/cli/): every command on one page.
