---
description: |
  On a maintainer's /hunt-for-regressions comment, try to break an analyzer
  pull request with inputs its author did not test, comparing the merge base
  against the head.

on:
  slash_command:
    name: hunt-for-regressions
    events: [pull_request_comment]
  workflow_dispatch:
    inputs:
      pull_request:
        description: Pull request number
        required: false
  reaction: eyes

permissions: read-all

concurrency:
  job-discriminator: ${{ github.event.issue.number || inputs.pull_request || github.run_id }}

engine:
  id: copilot
  model: claude-sonnet-5

imports:
  - shared/hunt-setup.md

safe-outputs:
  add-comment:
    max: 1
    target: ${{ github.event.issue.number || inputs.pull_request }}
    hide-older-comments: true
  add-labels:
    max: 1
    target: ${{ github.event.issue.number || inputs.pull_request }}
    allowed: [hunt:regression-found]
  remove-labels:
    max: 1
    target: ${{ github.event.issue.number || inputs.pull_request }}
    allowed: [hunt:regression-found]

timeout-minutes: 20
max-ai-credits: 200
---

# Counterexample hunt

Pull request #${{ github.event.issue.number || inputs.pull_request }} in
${{ github.repository }} changes gohawk. Its fixtures show the rule works on
the cases the author thought of; your job is to find a case they did not.

Follow section 5.1, "Hunt a counterexample", of
`.agents/skills/gohawk-pr-review/SKILL.md`, using the binaries built during
setup. Read sections 1 and 2 first to load the analyzer's accepted boundary,
and treat the base-branch copy of the skill as authoritative.

## Time and length budget

Run `date` when you start. Stop probing after 15 minutes and post what you
have; the job is cut off at 20, and a cut-off run posts nothing.

Keep the comment under 40 lines. Write for a reviewer who will reread the
diff, not for the record: no restated diff, no shape-by-shape list of cases
that behaved the same, no tables.

Extra instructions from the comment, if any: "${{ steps.sanitized.outputs.text }}"

1. Read the pull request description and the diff. Identify the decision the
   change moves and the analyzers that consume it.
2. Write minimal Go programs under `/tmp/gh-aw/hunt/probe/` that exercise
   that decision along the shapes section 5.1 lists, plus any the diff
   suggests. Run each through both binaries with only the changed analyzers
   enabled.
3. Classify every probe by the before/after table in section 5.1. Judge
   "correct" from what the program actually does on the reported path, not
   from which binary reports: trace which locks, goroutines or resources are
   live there. A diagnostic that disappears is a regression only if it was
   true; a false one disappearing is the intended fix. For each regression,
   confirm the mechanism with `-gohawk-trace` so the report says why it
   happens.
4. Post one comment with `add-comment`, targeting this pull request's
   number, in this shape:
   - a one-line `Probe:` summary in the section 10 format;
   - per regression, at most 15 lines: the smallest program that shows it,
     one line per binary with its output, and the mechanism in at most two
     sentences;
   - gaps (wrong before and after) as one line each, marked as not caused by
     this pull request;
   - one line naming the shapes that showed no change, and one naming those
     not tried.

Label the pull request `hunt:regression-found`, passing its number, when you found a regression; remove that
label when a rerun finds none.

If the pull request changes no analyzer or engine code, call `noop` with that
reason. Finding nothing is still a result: post the `Probe:` line and the two
shape lines, since they bound the risk statement.
