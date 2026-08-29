---
title: AI policy
description: Requirements for AI-assisted contributions to gohawk.
---

gohawk was developed with assistance from AI tools. AI-assisted contributions
are permitted, but contributors must clearly disclose how AI was used in their
pull request.

Using AI does not change the project's quality requirements. Contributors
remain responsible for understanding their changes and ensuring they follow
the repository's contribution guidelines. In particular, analyzer changes
must meet gohawk's standards for precision, testing, documentation, and
supported analyzer scope.

AI-generated output should be reviewed and validated before submission. Pull
requests must include appropriate tests and demonstrate that reported
diagnostics are actionable and have a low risk of false positives.

Human readability is a priority. AI-assisted code must remain clear,
idiomatic, and maintainable by contributors without relying on AI to explain
it. Avoid unnecessary abstractions, excessive indirection, and generated
complexity that makes the implementation harder to review or modify.

The project owner may close low-effort or insufficiently reviewed pull requests
that do not follow the contribution guidelines, regardless of whether AI was
used.

## How AI was used to develop gohawk

gohawk has been developed with substantial assistance from LLM-based coding
tools, primarily OpenAI Codex. AI assistance has been used to:

- Explore analyzer designs and implementation approaches.
- Write and refactor portions of the codebase.
- Draft tests, documentation, and diagnostic wording.
- Review suspected bugs and potential false positives.
- Construct reproductions and investigate behavior in real-world projects.
- Run and interpret validation, dogfooding, and integration tests.

AI-generated output is not accepted without review. The project owner
determines the project's direction, reviews changes, evaluates whether
diagnostics are sufficiently precise, and remains responsible for the
resulting code.

Analyzer behavior must be supported by focused fixtures, appropriate
regression tests, representative dogfooding, and CI validation regardless of
whether AI contributed to the implementation.
