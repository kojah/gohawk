---
title: Tags and profiles
description: Understand why an analyzer reports a finding and when it runs.
---

Tags explain why a finding matters:

- **Correctness** marks strong evidence that the program can behave incorrectly.
- **Reliability** marks code that may work but is vulnerable to meaningful
  lifecycle, concurrency, or operational failures.
- **Policy** marks a project convention on which reasonable teams may differ.

Tags are composable. An analyzer may cover more than one kind of concern when
it reports several related failure modes. Tags describe the nature of those
findings; they are not severity levels.

Profiles answer a different question: whether an analyzer runs automatically.
The **default** profile contains broadly applicable analyzers, while an
**opt-in** analyzer runs only when selected explicitly or when all analyzers are
enabled. A profile does not indicate how important a finding is, and an opt-in
analyzer is not necessarily a policy analyzer.

Use `gohawk list` to see every analyzer's tags and profile.

```sh
gohawk list
```

See [Configuring gohawk](../configuration/) to select analyzers and profiles.
