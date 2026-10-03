---
title: Memory
description: Judge an item against your own notes. jevx retrieves the best-matching sections of a folder of markdown (BM25, no embeddings) and sends them after the item. Measured numbers, the weak spot, and the strict rule that covers it.
---

`jevx memory` points a question at a folder of distilled, cited markdown: a wiki, runbooks, release notes. For each item it retrieves the best-matching sections and sends them **after** the item, so a server that keeps only its first N tokens can trim notes but never the item. Retrieval is BM25 over heading sections: no embeddings, no vector database, and nothing leaves your machine to build the index.

## Set it up

```bash title="Terminal"
jevx memory add docs ./wiki --pin NOTICES.md --repo app=~/src/app   # register a folder; cites like app:path:L10-L20 are hashed
jevx memory index docs                                              # (re)build after the pages change
jevx memory check docs                                              # pages whose cited lines changed are marked stale and skipped
jevx memory show docs "does v2 drop the 512-token limit?"           # what would be retrieved, with page#heading and lines
jevx memory list
```

Retrieval order for each item: pages the item names first, then pinned pages (`--pin`), then the best BM25 sections, up to `--memory-k` sections and `--memory-budget` characters.

## Ask with it

```bash title="Terminal"
jevx ask --memory docs --lines claims.txt --noul true="Is this claim correct per the notes?"
jevx ask --memory docs --memory-strict --lines claims.txt --noul true="Is this claim correct per the notes?"
```

| flag | default | meaning |
| --- | --- | --- |
| `--memory NAME` | | append the best-matching notes of that memory after each item |
| `--memory-k N` | 3 | BM25 sections after named and pinned pages |
| `--memory-budget N` | 2000 | characters of notes |
| `--memory-strict` | off | a number, `` `code` `` or name in the item that the retrieved notes lack turns a yes/no answer into `no` |

`--json` and `--raw` carry the retrieved notes (`memory`, with page, heading and lines) and the missing facts (`memory_diff`).

## Freshness

Every cite in a page (`app:path:L10-L20`) is hashed when the page is indexed. `jevx memory check` re-hashes them: a page whose cited lines changed is marked stale and is not retrieved until you fix it and re-index. Lines that only moved (lines inserted above them) are recognised and not flagged.

## How well it works

Measured on held-out claims, with the configuration frozen before the test (k 3, budget 2000, item first):

| setup | without notes | with notes |
| --- | --- | --- |
| local 0.4B model, n=68 claims (34 true, 34 with one changed number or word) | AUC 0.30, accuracy 47% | **AUC 0.86**, accuracy 71% (McNemar p=0.023) |
| hosted model, n=48 | AUC 0.62, accuracy 42% | AUC 0.83, accuracy 81% (p=0.0005) |

**The weak spot is a changed number.** A note that matches except for one figure pulls the answer toward "true": with notes alone, the local model caught 29 of 40 claims that had one number changed.

**`--memory-strict` covers it.** It compares the numbers, `` `code` `` and names in the item with those in the retrieved notes, and fails a yes/no claim when one is missing. On 80 unseen rows (40 page sentences, 40 number mutants):

| | changed numbers caught | accuracy | true claims wrongly failed |
| --- | --- | --- | --- |
| notes alone | 29 / 40 | 75% | 9 / 40 |
| notes + `--memory-strict` | **40 / 40** | **86%** (p=0.022) | 11 / 40 |

The rule costs two more true claims wrongly failed. Use it when a wrong "true" is worse than a wrong "false".

## Limits

- **It adds missing facts, not missing reasoning.** On a judgement task (not a fact check), notes did not help the small local model.
- **The strict rule checks presence, not negation.** Notes that say "not MIT" contain "MIT".
- **Samples are small** (n = 48 to 80). These are the measured numbers for these sets; run your own claims before trusting a threshold.
