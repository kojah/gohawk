---
description: Use when working out why gohawk did or did not report a diagnostic, reading an SSA dump or evidence trace, or mapping a finding back to the classifier label and flow verdict that produced it.
metadata:
    source: project
name: gohawk-debugging
---

# Debugging a gohawk diagnostic

The analyzers work on SSA, not on source, and the mapping between the two is
not obvious: defers are lowered, closures become explicit, loops and
short-circuit operators introduce phi nodes. Reasoning about that mapping in
your head is slow and error-prone. Dump the real thing instead.

## The three dumps

| question | command |
|---|---|
| What does the analyzer see? | `gohawk ssa [-func NAME] [-tests] ./pkg` |
| What did lifecyclefacts conclude about exported functions? | `gohawk facts [-func NAME] [-tests] ./pkg` |
| What did the analyzer decide, and why? | `gohawk -gohawk-trace=ANALYZER[,CHECK] ./pkg` |

Trace scoping flags: `-gohawk-trace-source=path[:line]`,
`-gohawk-trace-function=TEXT`, `-gohawk-trace-file=FILE` (append JSONL to a
file instead of stderr). Use `-gohawk-trace=all` to trace every analyzer.

Full detail in the
[debugging reference](../../../docs/development/debugging-reference.md).

## Reading a trace

Every event carries a phase and a stable kebab-case reason code:

- `candidate` — a construct the analyzer might report.
- `evidence` — a fact that supports accepting or rejecting it.
- `decision` — the final outcome: reported, suppressed, or unknown.
- `fix` — whether a suggested edit was offered or rejected.

Events include the SSA text they are about, so a trace reads as an annotated
SSA walk. Follow one candidate's events in order to see which instruction the
classifier labelled `unknown` and the reason it gave.

## Mapping a finding to its cause

1. Find the `candidate` event: this is the obligation the analyzer found.
2. Find the `evidence` events that consumed the tracked value: each is one
   classifier label. An `unknown` label ends the proof conservatively.
3. Read the `decision`: honored, unknown, or violated, with its reason code.

If the decision is `unknown`, the analyzer is declining to report because some
consumption is opaque to it. That is the intended behaviour, not a bug, unless
the opaque consumption is a pattern the classifier should recognise; if so,
switch to [gohawk-analyzer-change](../gohawk-analyzer-change/SKILL.md) and
apply the failure ladder.

## When a fact looks wrong

`gohawk facts` shows only functions that were summarized. A function with no
fact is `unknown` to consumers, not disproven; do not read absence as "does
nothing". See [the fact model](../../../docs/development/fact-model.md).
