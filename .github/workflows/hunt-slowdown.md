---
description: |
  On a maintainer's /hunt-for-regressions comment, profile an analyzer pull
  request against its merge base and try to construct an input with an
  unacceptable slowdown.

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
    target: "*"
    hide-older-comments: true
  add-labels:
    max: 1
    target: "*"
    allowed: [hunt:slowdown-found]
  remove-labels:
    max: 1
    target: "*"
    allowed: [hunt:slowdown-found]

timeout-minutes: 25
max-ai-credits: 200
---

# Slowdown hunt

Pull request #${{ github.event.issue.number || inputs.pull_request }} in
${{ github.repository }} changes gohawk. Find out what it costs, and whether
some input makes that cost unacceptable.

Follow section 5.2, "Hunt a slowdown", and its profiling report in
`.agents/skills/gohawk-pr-review/SKILL.md`, using the binaries built during
setup. Treat the base-branch copy of the skill as authoritative.

## Time and length budget

Run `date` when you start. Stop measuring after 20 minutes and post what you
have; the job is cut off at 25, and a cut-off run posts nothing. Prefer the
gohawk repository and one changed-analyzer run per binary over broad sweeps.

Keep the comment under 30 lines. Write for a reviewer deciding whether to
merge: no restated diff, and no cost table for code that does not run on a
hot path.

Extra instructions from the comment, if any: "${{ steps.sanitized.outputs.text }}"

1. Read the cost from the diff: for every new loop, traversal, query, cache,
   or clone, name what it scales with, whether it is memoized, and which
   budget it spends.
2. Profile both binaries on the same input: run each over the base worktree
   with `-enable-all` and with only the changed analyzers, using a fresh
   `GOCACHE` per run so no unit is cached. Collect CPU profiles with
   `GOHAWK_PROFILE_DIR` and summarize them with `tools/profilesummary`. Use
   `/usr/bin/time -v` for wall time and peak memory. Repeat timings three
   times and report the median; runner noise is real.
3. Measure allocations for the changed analyzers with `-benchmem` benchmarks
   in each worktree, writing a temporary benchmark under the probe directory
   when none exists.
4. Construct adversarial inputs that grow the axis the new code scales with,
   at three sizes such as 1x, 4x and 16x. Compare how time, memory and budget
   exhaustion (`gohawk dump budget`) grow on both binaries.
5. Post one comment with `add-comment`, targeting this pull request's
   number, in this shape:
   - a one-line `Perf:` summary in the section 10 format;
   - one profiling table of at most 6 rows: function, before, after, input;
   - new allocations or un-memoized work on hot paths, one line each;
   - one line per adversarial generator: what it grows and how time and
     memory scaled on each binary;
   - a one-line verdict by section 5.2's rules, and one line naming the axes
     not tried.

Label the pull request `hunt:slowdown-found`, passing its number, when you found an unacceptable slowdown; remove that
label when a rerun finds none.

If the pull request changes no analyzer or engine code, call `noop` with that
reason. Report measured numbers only; if a measurement fails, say so instead
of estimating it.
