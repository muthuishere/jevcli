# jevcli cookbook

Every recipe below is a single shell command. It works against any Jev-like endpoint, meaning anything that speaks the
System One API: hosted Jev, a self-hosted model, or your own. Point a profile at it with
`jevcli profile add NAME URL --model M --header "Authorization: Bearer $VAR"` and choose it with `--profile NAME`.
The outputs shown are real, from the hosted Jev on 2026-09-27. A smaller model can answer differently.
Text inputs take `TEXT`, `@file` or `-` (stdin).

## Control flow
| need | command | output |
|---|---|---|
| fuzzy if | `if jevcli feels urgent < email.txt; then ...; fi` | exit 0 yes / 1 no (outage 0.98, newsletter 0.06) |
| fuzzy if, any question | `jevcli feels "Is the build green?" --input @status.txt --threshold 0.7` | exit 0/1 |
| fuzzy switch | `case $(jevcli match billing="about money" bug="a defect" other="anything else" < msg) in billing) ...;; esac` | `billing` |
| fuzzy while | `while ! jevcli feels done --input @status.txt -q; do ...; done` | loop until yes |
| route by confidence | `jevcli ask Q --context C --option push="push now" --option wait="wait" --route 0.8,0.5` | `act push (0.84)`, exit 0 act / 10 confirm / 20 escalate |

## Many at once (fast, in parallel)
```bash
jevcli query --state '{"role":"...","message":"..."}' --noul a="..." --noul b="..." --choice k="Q|x=desc;y=desc" --score s="Q|low;mid;high" --raw
jevcli query --states items.jsonl --parallel 8 --questions set.json | jq -c 'select(.answers.urgent.noul > .7)'
jevcli run request.json      # {"state": "...", "questions": {...}}: a raw System One request
```
`items.jsonl` holds one JSON object or JSON string per line. `set.json` is `{"NAME": {"type": "noul|choice|score",
"instructions": "...", "criteria": ...}}`. 20 states take 1.5 s at `--parallel 8`, against 8.2 s one at a time.

## Recipes
| need | command | real output |
|---|---|---|
| fact / citation check | `jevcli verify --claim "Payment is due in October" --source @invoice.txt` | `supported (P 0.99)` |
| dedupe / same entity | `jevcli same --a "Jon Smith, 12 Baker St" --b "John Smith, 12 Baker Street" --what "customer record"` | `same (P 0.88)` |
| re-rank search results | `jevcli rank --query "get my money back" --candidates docs.txt --top 2 [--dimension ...]` | `0.910 Refund policy ...` |
| semantic grep | `jevcli find "a failure" app.log [--min 0.6]` | `app.log:2 0.89 ERROR payment gateway timeout` |
| extract a value | `jevcli extract --text @invoice.txt --what "the due date" --kind date` | `2026-10-15 (confidence 1.00)` |
| ordinal rating | `jevcli score --text "App crashes on open" --question "How severe?" --level low --level medium --level high` | `1.99 / 2 (high)` |
| classify into a tree | `jevcli tree --text @ticket.txt --taxonomy tax.json` (`{"label": {"child": "desc"}}`) | the path, level by level |
| function calling | `jevcli pick-func "I want my money back for order 88" --functions funcs.json` | `refund_payment (0.99)` |
| which skill to load | `jevcli pick-skill "deploy the worker"` | the skill name |
| self-consistency | `jevcli ask "Is this spam?" --context "..." --option spam=spam --option ham=legit --samples 3` | `spam (agreement 100%)` |
| judge your own turn | `jevcli judge --request "fix the failing test" --proposal "fixed, go test passes"` | `accept 0.47, wanted more 0.73` |
| yes/no | `jevcli ask Q --context C --true "yes means" --false "no means"` | P(true) |

## Tips
- Put the context in the question: say whose decision it is and what counts as yes. The model sees nothing else.
- For hundreds of items, use `query --states` rather than `find` or `rank`, which still send one request per line.
- Jev judges, it does not write. Pair it with an LLM for the text: `jevcli feels urgent < m && claude -p "Draft a reply" < m`.
