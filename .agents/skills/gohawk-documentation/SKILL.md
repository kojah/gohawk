---
description: Use when writing or editing gohawk documentation, including analyzer pages, the website, design notes, and decision records, and whenever an analyzer change needs its behavior documented.
metadata:
    source: project
name: gohawk-documentation
---

# Writing gohawk documentation

gohawk has two documentation audiences, and they live in two places.

| Audience | Location | Published |
|---|---|---|
| Users deciding whether to trust a diagnostic | `docs/` except `docs/development/` | on the website |
| Contributors changing an analyzer | `docs/development/` | no, read in the repository |

Most documentation damage comes from writing for the wrong audience: proof
mechanics written onto a public page because that is where the analyzer is
described. The public page then grows by one precision boundary per change
until no user reads it. Decide the audience before writing a sentence.

## Where each kind of content goes

| Content | Destination |
|---|---|
| What a check reports, why it matters, how to fix it, its options | the analyzer page, `docs/analyzers/<group>/<name>.mdx` |
| A case deliberately not reported, in one plain sentence | the analyzer page |
| Why a boundary holds, what evidence it needs, which shapes it declines | `docs/development/analyzers/<name>.md` |
| Commit-pinned links to dogfooded repositories | the design note, next to the boundary they justify, and the rationale comment in code |
| Proof engines, fact encodings, measurements | `docs/development/*.md` |
| Why a policy exists and what it rules out | `docs/development/decisions/<date>-<slug>.md` |

An analyzer change updates its design note together with its fixtures. It
touches the public page only when what a user sees changes: a new check, a new
message, a new option, or a case that is now reported or now left alone.

## Writing the public page

Write the contract, not the proof. For each behavior, say what is reported or
what is deliberately left alone, in terms a Go programmer uses about their own
code.

| Venting (design note) | Contract (public page) |
|---|---|
| "The summary path declines branching or opaque helper bodies and uncertain identities." | "Closes and sends inside helpers count too." |
| "Parent cancellation makes child cleanup uncertain rather than proving a leak." | "Canceling a parent context can stand in for the child's own cancel." |
| "Commands built by helpers have uncertain ownership and are not reported." | "Commands built by helpers are not checked." |

Words that signal a proof boundary written for the author: *uncertain*,
*opaque*, *boundary*, *declines*, *conservative*, *does not prove*, *is not
proof*, *does not establish*. When you reach for one, the sentence belongs in
the design note. The architecture tests reject these words on analyzer pages.

End each trimmed analyzer page's detection section with the link to its design
note, as the existing pages do. Never add a commit-pinned repository link to a
public page.

## Writing a design note

Design notes are where precision reasoning is kept, so be complete rather than
short. For each boundary, state the accepted and reported shapes, the evidence
that separates them, the fixture file that pins it, and a commit-pinned link to
the real-world pattern when one motivated it. Remove a boundary's text in the
same change that removes the boundary.

## Checks

The architecture tests enforce the public side deterministically, in CI and in
`make verify`:

- analyzer pages: at most 130 lines, 40 lines of hand-written prose outside
  generated blocks, 8 lines per paragraph, and none of the words above;
- other public pages: at most 200 lines, with a baseline for pages already
  over;
- no public page links pinned source.

Run them directly with
`go test ./internal/architecture -run 'PublicDocumentation|AnalyzerProse'`.
`make site-check`, `make site-build`, and `make site-links` validate the site,
and `make site-review` serves it for review. A failure means the text belongs
in `docs/development/`; do not raise a budget or extend a baseline to make it
pass.
