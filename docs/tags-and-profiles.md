---
title: Tags and profiles
description: Understand why a check reports a finding and when its analyzer runs.
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

Profiles answer a different question: whether an analyzer runs automatically.
The **default** profile contains broadly applicable analyzers, while an
**opt-in** analyzer runs only when selected explicitly or when all analyzers are
enabled. A profile does not indicate how important a finding is, and an opt-in
analyzer is not necessarily a policy analyzer.

Use `gohawk list` to see every analyzer's profile and group.

```sh
gohawk list
```

See [Configuring gohawk](../configuration/) to select analyzers and profiles.
