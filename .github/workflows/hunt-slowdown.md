---
description: |
  On a maintainer's /hunt comment, profile an analyzer pull request against its
  merge base and try to construct an input with an unacceptable slowdown.

on:
  slash_command:
    name: hunt
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

engine: claude

imports:
  - shared/hunt-setup.md

safe-outputs:
  add-comment:
    max: 1
    hide-older-comments: true

timeout-minutes: 60
---

# Slowdown hunt

Pull request #${{ github.event.issue.number || inputs.pull_request }} in
${{ github.repository }} changes gohawk. Find out what it costs, and whether
some input makes that cost unacceptable.

Follow section 5.2, "Hunt a slowdown", and its profiling report in
`.agents/skills/gohawk-pr-review/SKILL.md`, using the binaries built during
setup. Treat the base-branch copy of the skill as authoritative.

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
5. Post one comment with `add-comment`:
   - a `Perf:` line in the section 10 format;
   - the profiling report table: function, before, after, input;
   - new allocations and un-memoized work on hot paths, with the evidence;
   - the growth table for each adversarial input, with its generator;
   - whether any shape is unacceptable by section 5.2's rules, and which axes
     you did not try.

If the pull request changes no analyzer or engine code, call `noop` with that
reason. Report measured numbers only; if a measurement fails, say so instead
of estimating it.
