---
name: jevcli
description: Ask a Jev-style decision model (any System One endpoint configured in jevcli) what the user would decide, before bothering them. Use when you are about to ask the user to pick between options, when you wonder whether the user would accept your turn as it is, or to check you did not stop short. Trigger: what would the user pick, would the user accept this, ask jev, decide like me, should I ask the user, what do you say.
---

# jevcli: ask a decision model before asking the user

`jevcli` asks a Jev-style decision model over the System One API. It **judges and chooses**; it never writes text, so
give it options, a yes/no, or a turn to judge. Put the context in the question itself: the instructions and each
option / criterion say whose decision it is and what is being judged; the model reads nothing else. Endpoints are profiles (`jevcli config show`); use `--profile NAME` to pick
one, otherwise the default profile is used.

## Judge your own turn before handing back

```bash
jevcli judge --request "<the user's request, verbatim>" --proposal "<your final message>" \
  --action "Bash: go test ./..." --action "Edit src/app.py: <excerpt>"
```
Prints `accept`, `wanted more`, `reaction`, `satisfaction` (0-4).
- `accept < 0.35`: the user would likely push back. Verify and show evidence (output, diff, test run).
- `wanted more > 0.65`: you stopped short. Finish the missing part yourself unless it truly needs the user's decision.

## "What would the user say?" before asking them

```bash
jevcli ask "<the decision as one question>" --context "<the facts the user would look at>" \
  --option "stop=Stop and report, wait for the user" --option "run=Go ahead now"
jevcli ask "<yes/no question>" --context "<facts>" --true "<what yes means>" --false "<what no means>"   # -> P(true)
```
**Trust rule**: act on a choice only if `confidence >= 0.6`; on a yes/no only if `P(yes) >= 0.8` or `<= 0.2`. Otherwise
ask the user and include the lean in one line ("jevcli leans stop, 0.51"). When you act on it, say so in your report.

## Cookbook recipes (all use the same profiles)

| command | use it for |
|---|---|
| `jevcli ask Q --option k=desc ... [--route 0.8,0.5] [--samples 5]` | classify / route; `--route` prints act, confirm or escalate (exit 0/10/20); `--samples` checks self-consistency |
| `jevcli verify --claim TEXT --source @file` | is a claim or citation supported by the source |
| `jevcli same --a TEXT --b TEXT --what "customer record"` | are two records the same entity (dedupe, alignment) |
| `jevcli rank --query TEXT --candidates FILE [--dimension TEXT ...]` | re-rank search results; several dimensions = composite score |
| `jevcli find QUERY FILE [--min 0.6]` | semantic grep, one question per line |
| `jevcli extract --text @file --what "the due date" --kind date` | pick a value among candidates parsed from the text (date, number, money, email, url) |
| `jevcli tree --text TEXT --taxonomy tax.json` | hierarchical classification, level by level |
| `jevcli score --text TEXT --question Q --level L0 --level L1 ...` | ordinal rating |
| `jevcli pick-skill TASK` / `jevcli pick-func REQUEST --functions f.json` | which skill to load / which function to call |
| `jevcli run request.json` | any raw System One request (many questions for one state) |

Profiles: `jevcli profile list | use NAME | add NAME URL --model M --header "Authorization: Bearer $VAR"`. `$VAR` is
expanded at runtime in every profile field (url, model, headers, context, paths); a profile's `context` is prepended to
every state it sends.

## Never
- As a safety gate: permission prompts, the user's rules and money / legal / irreversible calls stay with the user.
- Pasting secrets into `--context` / `--proposal` (jevcli redacts common key shapes; do not rely on it).
- Using answers from a hosted third-party model as training data for another model, unless its terms allow it.

## Hooks (off unless enabled)
`jevcli install` adds a Stop-hook template that does nothing until `jevcli hook enable stop`. `jevcli hook status`;
`jevcli hook mode shadow|block`; `jevcli hook review` lines logged verdicts up with what the user said next.
