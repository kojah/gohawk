---
description: Use when writing or editing gohawk documentation, including analyzer pages, the website, design notes, and decision records, and whenever an analyzer change needs its behavior documented.
metadata:
    source: project
name: gohawk-documentation
---

# Writing gohawk documentation

gohawk has two kinds of documentation.

| | User docs | Dev docs |
|---|---|---|
| Where | `docs/`, except `docs/development/` | `docs/development/` |
| Published | on the website | no, read in the repository |
| Reader | a Go programmer using gohawk | someone changing gohawk |
| Written from | the user's point of view | the implementation's point of view |
| Length | short | as long as it needs to be |

## The rules

1. **User docs use simple English and stay short.** Say what gohawk does for
   the reader's code. Don't ramble, hedge, or explain how the analyzer works
   inside.
2. **If you feel the need to ramble, write it in the dev docs.** Edge cases,
   proof details, evidence, measurements, and the reasons behind a rule all
   belong there. Dev docs are allowed to be long and detailed.
3. **User docs talk about the user's code, not gohawk's internals.** Write
   "a file you open and never close", not "an acquisition whose obligation is
   unowned on a return path". Dev docs may name SSA forms, facts, classifiers,
   budgets, and any other implementation detail.

Before writing a sentence, decide who reads it. Most bad documentation here
came from implementation detail written onto a user page because that is where
the analyzer happened to be described.

## Where things go

| Content | Where |
|---|---|
| What a check reports, why it matters, how to fix it, its options | the analyzer page, `docs/analyzers/<group>/<name>.mdx` |
| A case gohawk deliberately doesn't report, in one plain sentence | the analyzer page |
| Why that case is left alone, what evidence decides it, which fixtures pin it | the design note, `docs/development/analyzers/<name>.md` |
| Links to the real projects that motivated a rule | the design note, and the rationale comment in code |
| How the engine, facts, or models work; measurements | `docs/development/*.md` |
| Why a policy exists and what it rules out | `docs/development/decisions/<date>-<slug>.md` |

An analyzer change updates its design note together with its fixtures. Change
the user page only when what a user sees changes: a new check, message, or
option, or a case that is now reported or now left alone.

## Rewriting for users

| Too much for a user page (move to the dev docs) | User page |
|---|---|
| "The summary path declines branching or opaque helper bodies and uncertain identities." | "Closes and sends inside helpers count too." |
| "Parent cancellation makes child cleanup uncertain rather than proving a leak." | "Canceling a parent context can stand in for the child's own cancel." |
| "Commands built by helpers have uncertain ownership and are not reported." | "Commands built by helpers are not checked." |

Words like *uncertain*, *opaque*, *boundary*, *declines*, *conservative*,
*does not prove*, *is not proof*, and *does not establish* mean you are
explaining the analyzer, not the user's code. Move that sentence to the dev
docs.

End a trimmed analyzer page's detection section with a link to its design
note, as the existing pages do.

## Writing dev docs

Be complete. For each boundary, say which shapes are accepted and which are
reported, the evidence that tells them apart, the fixture file that pins it,
and a commit-pinned link to the real-world code that motivated it. Remove the
text in the same change that removes the boundary.

## Checks

The architecture tests check user docs in CI and in `make verify`:

- analyzer pages: at most 130 lines, at most 40 lines of hand-written prose
  outside generated blocks, at most 8 lines per paragraph, and none of the
  words above;
- other user pages: at most 200 lines, with a baseline for pages already over;
- no user page links to pinned source.

Run them with `go test ./internal/architecture -run 'PublicDocumentation|AnalyzerProse'`.
Use `make site-check`, `make site-build`, and `make site-links` to check the
site, and `make site-review` to look at it. If a check fails, move the text to
the dev docs. Don't raise a budget or add to a baseline to make it pass.
