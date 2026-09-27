---
name: jevcli
description: Fast yes/no, pick-one and 0-1 judgements from a Jev-style decision model (any System One endpoint), instead of reasoning them out yourself. Use when you must classify, filter, triage, route, dedupe, rank or check many items (files, lines, tickets, messages, records), when a branch in your work depends on meaning ("is this urgent", "which kind is it"), when you are about to ask the user to pick between options, and before handing back a turn. Trigger: is this X, which of these, triage, classify, filter these, rank these, would the user accept this, what would the user pick, ask jev.
---

# jevcli: judgements in milliseconds

jevcli sends a question to a decision model and gets back a **number or a key, never prose**. One call takes about 0.4 s,
and a batch runs in parallel. Use it where you would otherwise spend your own tokens deciding something small: whether
each of 200 log lines is an error, which team a ticket belongs to, whether two records are the same person.

It only judges. You still write the code, the reply or the fix.

## The three calls

```bash
jevcli is "Is this urgent?" < email.txt            # prints P(yes) e.g. 0.98; exit 0 if >= 0.5, else 1
jevcli which bug="a software defect" billing="about money" other="anything else" < msg.txt   # prints the key
jevcli ask --lines app.log --is error="Is this line an error?" --parallel 8                  # JSONL, one per line
```

- The input is **stdin**, or `--in @file`, or `--in "text"`.
- `is` exits 0/1, so it works as an `if`. `which` prints one key, so it works in a `case`.
- `ask` takes many questions over many inputs. `--lines FILE` makes one input per line, `--states FILE.jsonl` one per JSON
  line, and `--in` gives a single input. Questions come from `--is NAME="Q"`, `--which NAME="Q|key=desc;key=desc"`,
  `--score NAME="Q|low;mid;high"` or `--questions set.json`. The output is one JSON line per input, in input order:
  `{"line":3,"input":"...","answers":{"error":{"noul":0.91}}}`.

## When to reach for it

| you are about to... | do this instead |
|---|---|
| read 300 lines to find the failures | `jevcli ask --lines log --is fail="Is this a failure?" \| jq 'select(.answers.fail.noul>.7)'` |
| sort tickets or messages into buckets | `jevcli ask --states t.jsonl --which team="Which team?\|web=frontend;api=backend;ops=infra"` |
| decide if two records are one entity | `printf 'A: %s\nB: %s' "$a" "$b" \| jevcli is "Are A and B the same customer?"` |
| check a claim against a source | `jevcli is "Does the text say payment is due in October?" < invoice.txt` |
| rank search results | `jevcli ask --lines results.txt --is hit="Does this answer: how do I get a refund?"`, then sort by `noul` |
| ask the user "A or B?" | `jevcli which --in "<the facts>" a="<option A>" b="<option B>"`; ask only if confidence < 0.6 |
| hand back your turn | `jevcli judge --request "<user's request>" --proposal "<your final message>"` (below) |

**Write the question so it stands alone.** The model sees only the question and the input, not your conversation. Say
whose decision it is and what counts as yes: "Would a senior on-call engineer page someone for this line?" works better
than "bad?".

**Trust rule.** Act on `is` when P ≥ 0.8 or ≤ 0.2, and on `which` when confidence ≥ 0.6 (printed on stderr). In between,
treat the answer as a lean and check it yourself, or ask the user. When an answer changed what you did, say so in your
report: "jevcli flagged 12 of 300 lines (P > 0.8)".

## Judge your own turn before handing back

```bash
jevcli judge --request "<the user's request, verbatim>" --proposal "<your final message>" --action "Bash: go test ./..."
```
If `accept` < 0.35, the user would likely push back: verify and show evidence. If `wanted more` > 0.65, you stopped short:
finish the missing part unless it truly needs the user.

## Setup (once)

```bash
curl -fsSL https://muthuishere.github.io/jevcli/install.sh | sh      # installs this skill for Claude Code, Codex and ~/.agents
jevcli profile add jev https://your-endpoint/v1/systemone --model MODEL --header "Authorization: Bearer $YOUR_KEY_VAR"
```
If a call fails with "no url" or "needs $VAR", tell the user which profile or variable is missing. Do not guess one.
`--profile NAME` picks a different endpoint for one call.

## Never
- Use it as a safety gate. Permissions, money, legal and irreversible calls stay with the user.
- Put secrets into `--in` or `--proposal`.
- Train another model on answers from a hosted third-party endpoint unless its terms allow it.
