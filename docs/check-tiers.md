---
title: Check tiers
description: What core, extended, and experimental mean, how a tier changes what runs, and how a check moves between tiers.
---

Every gohawk check carries a tier. The tier records how much trust the check
has earned and therefore whether it runs without being asked for. It is
separate from a check's kind, which says what a finding means: a defect, a
hazard, or a policy violation.

| Tier | Runs by default | What it promises |
| --- | --- | --- |
| core | yes | precision demonstrated on the repository audit, a bounded proof model, and labels the precision replay guards on every change |
| extended | no | the same engineering bar, but the check encodes a house rule a reasonable team may decline |
| experimental | no | a heuristic audit that may change, split, or be retired; its findings are worth reading, not worth blocking a build on |

An analyzer has no tier of its own. It runs whenever one of its checks is
selected, and `gohawk list` shows the most trusted tier among its checks. Most
analyzers sit in one tier; a few default analyzers also carry an experimental
audit, such as the `detached` checks that report fire-and-forget goroutine and
process launches.

## How a tier changes what runs

Selection starts from a tier ceiling, which defaults to core, and the flags
in [Configuration](../configuration/) add to or remove from that base.

- `-tier=extended` or `-tier=experimental` raises the ceiling, so every check
  at or below it runs.
- `-enable=NAME` admits the named analyzer's core and extended checks. Its
  experimental checks join only when the ceiling is experimental, because
  asking for an analyzer by name should not silently switch on an audit.
- `-enable-checks=ID` admits one check whatever its tier.
- `-enable-all` means every check at every tier.

```sh
# Core checks only.
gohawk ./...

# Core and extended checks.
gohawk -tier=extended ./...

# One extended analyzer alongside the core checks.
gohawk -enable=wirepolicy ./...

# One experimental audit alongside the core checks.
gohawk -enable-checks=goroutineownership/detached ./...
```

JSON output carries each diagnostic's category, and `gohawk list -checks`
maps every category to its tier, so downstream tooling can treat experimental
findings differently from core ones.

## Moving between tiers

Promotion is decided by evidence from the
[repository audit](https://github.com/kojah/gohawk/tree/main/benchmarks/precision),
not by opinion.

- A new check starts **experimental**.
- It moves to **extended** once it has fixtures for both its diagnostic and
  accepted forms, a documentation page, and one audit batch in which its
  findings were reviewed without a false-positive class.
- It moves to **core** after consecutive audit batches with no false
  positives and a proof model that reports nothing when ownership or lifecycle
  is uncertain, rather than guessing.
- A check moves down the same way. A core check that produces a
  false-positive class the model cannot bound structurally is demoted rather
  than patched with exceptions, and an experimental audit whose findings keep
  failing review is retired.

The precision replay runs with every tier enabled, so a change that makes an
experimental audit noisy still fails the gate; the difference between tiers
is what users see by default, not how carefully each tier is tested.
