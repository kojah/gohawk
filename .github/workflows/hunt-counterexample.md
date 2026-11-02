---
description: |
  On a maintainer's /hunt comment, try to break an analyzer pull request with
  inputs its author did not test, comparing the merge base against the head.

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

timeout-minutes: 45
---

# Counterexample hunt

Pull request #${{ github.event.issue.number || inputs.pull_request }} in
${{ github.repository }} changes gohawk. Its fixtures show the rule works on
the cases the author thought of; your job is to find a case they did not.

Follow section 5.1, "Hunt a counterexample", of
`.agents/skills/gohawk-pr-review/SKILL.md`, using the binaries built during
setup. Read sections 1 and 2 first to load the analyzer's accepted boundary,
and treat the base-branch copy of the skill as authoritative.

Extra instructions from the comment, if any: "${{ steps.sanitized.outputs.text }}"

1. Read the pull request description and the diff. Identify the decision the
   change moves and the analyzers that consume it.
2. Write minimal Go programs under `/tmp/gh-aw/hunt/probe/` that exercise
   that decision along the shapes section 5.1 lists, plus any the diff
   suggests. Run each through both binaries with only the changed analyzers
   enabled.
3. Classify every probe by the before/after table in section 5.1. For each
   regression, confirm the mechanism with `-gohawk-trace` so the report says
   why it happens.
4. Post one comment with `add-comment`:
   - a `Probe:` line in the section 10 format;
   - each regression with its minimal program, both binaries' output, and
     the mechanism;
   - gaps (wrong both before and after), listed separately and marked as not
     caused by this pull request;
   - which shapes you tried and which you did not.

If the pull request changes no analyzer or engine code, call `noop` with that
reason. Finding nothing is still a result: post the shapes you tried, since
that bounds the risk statement.
