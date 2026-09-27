---
name: jevcli
description: Fast yes/no, pick-one and rating judgements from a Jev-style decision model (any System One endpoint), instead of reasoning them out yourself. Use when you must classify, filter, triage, route, dedupe, rank or check many items (files, lines, tickets, messages, records), when a branch in your work depends on meaning ("is this urgent", "which team owns this"), when you are about to ask the user to pick between options, and before handing back a turn. Trigger: is this X, which of these, triage, classify, filter these, rank these, would the user accept this, what would the user pick, ask jev.
---

# jevcli: judgements in milliseconds

jevcli asks a decision model questions about an input and gets back a **verdict, never prose**. One call takes about
0.4 s, and batches run in parallel. Use it where you would otherwise spend your own tokens deciding something small:
whether each of 300 log lines is an error, which team owns a ticket, whether two records are the same customer.

It only judges. You still write the code, the reply or the fix.

## Shortcuts for the everyday cases

```bash
if jevcli is "Is this urgent?" < email.txt; then ...; fi      # yes 0.95 · exit 0 yes, 1 no, 3 unsure, 4 error
jevcli pick "Which team?" web=frontend api=backend billing=money --in "charged twice"   # billing 0.97
jevcli filter "Is this an error or failure?" < app.log         # grep by meaning: prints the matching lines (-v: the rest)
jevcli rank "Is this about refunds?" --top 5 < results.txt     # every line with P(yes), best first
```
Each is one question through `ask`, with the same saved questions (`jevcli is urgent`), context, settings and exit codes.
Other checks are just a well-worded `is` or `pick`: `is "Does the source support this claim: …?"`,
`is "Are A and B the same customer?"`, `pick "Which function handles this?" refund=… invoice=…`.

## The full command: `jevcli ask`

```bash
jevcli ask --noul urgent="Is this urgent for the person receiving it?" < email.txt
# urgent           yes        0.95

jevcli ask urgent,team < ticket.txt                    # named questions, saved once in config (see below)
jevcli ask --lines app.log --noul err="Is this line an error?"        # batch: JSONL, one line per input
jevcli ask --states tickets.jsonl team,sev --parallel 8                # batch over JSON records
```

- **Questions**, as many as you like in one call:
  - A saved name: `urgent`, `urgent,team`.
  - Inline: `--noul NAME="Q"` (yes/no), `--choice NAME="Q|key=desc;key=desc"` (pick one) or `--score NAME="Q|low;mid;high"`
    (rating).
  - A file: `--questions set.json`.
- **Input**: stdin, `--in "text" | @file | '{json}'`, `--lines FILE` (one input per line) or `--states FILE.jsonl` (one JSON
  object or string per line).
- **Output for one input**: one line per question, `NAME  VERDICT  P`. The verdict is `yes` / `no` / `unsure` for a
  noul, the key or `unsure` for a choice, the level or `unsure` for a score. `--json` gives the same as JSON, and `--raw`
  gives the server's full reply.
- **Output for a batch**: JSONL, in input order:
  `{"line":2,"input":"...","answers":{"err":{"verdict":"yes","p":0.98,...}}}`. Filter it with
  `jq 'select(.answers.err.verdict=="yes")'`.
- **Exit codes** (one input): **0** all yes or decided, **1** a "no", **3** an unsure, **4** an error (network, auth,
  invalid reply). An error is never a "no". On 4, say the call failed; on 3, check it yourself or ask the user. A batch
  exits 0, or 4 if any input failed (that line carries `"error"`).

## Context: what the model should know

The model sees only the question and the input, so give it the background. The layers are sent in this order: global, folder, profile, question, call.

```bash
jevcli context set "We are a 5-person SaaS; on-call is one engineer."        # global, every call
jevcli context set @~/notes/team.md --profile jev                            # one profile (a file is re-read each call)
jevcli question add urgent --noul "Is this urgent?" --context "The reader is the CFO."   # one saved question
jevcli ask urgent --context "The board meeting starts in 20 minutes." < msg.txt           # this call (TEXT or @file, repeatable)
jevcli context set --local "This repo is a payments service."                # this folder (.jevcli/context.md)
jevcli context                                                                # show what is set, and where
```
The same message scored 0.84 with no context, 0.94 with "the meeting is in 20 minutes" and 0.64 (unsure) with "the
meeting is in three months". Pass the facts that decide it. A JSON input keeps its shape: the context goes into its
`"context"` field.

## Save questions once, reuse everywhere

```bash
jevcli question add urgent --noul "Is this urgent for the person receiving it?" --yes 0.85
jevcli question add team --choice "Which team should handle this?|web=frontend or UI;api=backend;billing=money"
jevcli question add sev --score "How severe is this?|low;medium;high"
jevcli question list
```
Add `--local` to save a question in this folder's `.jevcli/questions.json` instead. It applies to every call made inside
the folder, wins over a global question of the same name, and can be committed with the repo. Check `jevcli question
list` before writing a question inline: the user may already have one tuned.

## Everything is configurable, with defaults

`jevcli defaults` shows every setting and where its value comes from. `jevcli defaults set KEY VALUE [--profile P]`
changes one. The order of precedence is built-in < config < profile < the question's own thresholds < flags.

| key | default | meaning |
|---|---|---|
| `yes` / `no` | 0.8 / 0.2 | a noul P at or above `yes` is yes, at or below `no` is no, anything between is unsure |
| `min_confidence` | 0.6 | a choice or score below this is unsure |
| `parallel` | 8 | concurrent requests in a batch |
| `retries` / `timeout_s` / `chunk` | 3 / 60 / 32 | tries on 429/5xx, seconds per request, questions per request |
| `ledger` | true | log each call without content to `~/.local/share/jevcli/calls.jsonl` (`jevcli stats`) |
| `accept_min` / `more_max` | 0.35 / 0.65 | Stop hook thresholds |

Flags override for one call: `--yes`, `--no`, `--min`, `--parallel`, `--profile`.

## When to reach for it

| you are about to... | do this instead |
|---|---|
| read 300 lines to find the failures | `ask --lines log --noul fail="Is this a failure an on-call engineer would act on?"` |
| sort tickets or messages into buckets | `ask --states t.jsonl --choice team="Which team?\|web=UI;api=backend;ops=infra"` |
| decide if two records are one entity | `printf 'A: %s\nB: %s' "$a" "$b" \| jevcli ask --noul same="Are A and B the same customer?"` |
| check a claim against a source | `jevcli ask --noul ok="Does the text say payment is due in October?" < invoice.txt` |
| rank search results | `ask --lines results.txt --noul hit="Does this answer: how do I get a refund?"`, then sort by `p` |
| ask the user "A or B?" | `ask --in "<the facts>" --choice pick="Which would the user choose?\|a=<A>;b=<B>"`; ask only on exit 3 |
| hand back your turn | `jevcli judge --request "<user's request>" --proposal "<your final message>"` |

**Write the question so it stands alone.** The model sees only the question and the input, not your conversation. Say
whose decision it is and what counts as yes: "Would a senior on-call engineer page someone for this line?" works better
than "bad?". When a verdict changed what you did, say so in your report: "jevcli flagged 12 of 300 lines as errors".

## Judge your own turn before handing back

```bash
jevcli judge --request "<the user's request, verbatim>" --proposal "<your final message>" --action "Bash: go test ./..."
```
If `accept` is below 0.35, the user would likely push back: verify and show evidence. If `wanted more` is above 0.65,
you stopped short: finish the missing part unless it truly needs the user.

## Setup (once)

```bash
curl -fsSL https://muthuishere.github.io/jevcli/install.sh | sh      # this skill for Claude Code, Codex and ~/.agents
jevcli profile add jev https://your-endpoint/v1/systemone --model MODEL --header "Authorization: Bearer $YOUR_KEY_VAR"
```
If a call fails with "no url" or "needs $VAR", tell the user which profile or variable is missing. Do not guess one.

## Never
- Use it as a safety gate. Permissions, money, legal and irreversible calls stay with the user.
- Put secrets into `--in` or `--proposal`.
- Train another model on answers from a hosted third-party endpoint unless its terms allow it.
