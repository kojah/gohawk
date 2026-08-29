---
title: Tags and profiles
description: Understand why a check reports a finding and when analyzers and checks run.
---

Tags belong to individual checks and explain why a finding matters:

<!-- gohawk:generated-tags:start -->
- <strong id="correctness">Correctness</strong> — Strong evidence that the program can behave incorrectly.
- <strong id="reliability">Reliability</strong> — Code that may work but is vulnerable to meaningful lifecycle, concurrency, or operational failures.
- <strong id="policy">Policy</strong> — A project convention on which reasonable teams may differ.
<!-- gohawk:generated-tags:end -->

Tags are composable: one check can have more than one tag. An analyzer may run
several checks with different tags, but the tags remain properties of those
checks rather than of the analyzer as a whole. Tags describe the nature of
findings; they are not severity levels.

Profiles answer a different question: whether something runs automatically.
Analyzer profiles are the outer gate: a **default** analyzer participates in an
ordinary run, while an **opt-in** analyzer must be selected explicitly. Check
profiles are the inner gate: a **default** check runs whenever its analyzer is
selected, while an **opt-in** check must be named explicitly or included by
`-enable-all`.

These two levels make it possible for a broadly useful analyzer to contain one
more opinionated check without enabling that check for everyone. Profiles do
not indicate severity, and an opt-in analyzer or check is not necessarily a
policy rule.

Use `gohawk list` to see analyzer profiles, or `gohawk list -checks` to see
check profiles.

```sh
gohawk list
gohawk list -checks
```

See [Configuring gohawk](../configuration/) to select analyzers and profiles.
