# Numeric classification domains

Bead: `gohawk-dho.23.32`. Parent revision: `4176f44`.

## Scope and representation

The remaining directly declared, named string classification domains found in
maintained Go source were catalog CheckKind/CheckTier, their independent public
analyzer definitions, SSA EvidenceProvenance and trace Outcome. They now use
numeric domains with an unset zero. The public analyzer names alias the catalog
owners; tier ordering and option parsing retain one implementation. The shared
`internal/engine/enumtext` codec handles numeric-to-text boundaries. The four remaining
direct string types are analyzer, group and check identities, rather than closed
decision domains.

Valid text/JSON labels, including empty unset labels, are preserved. Unknown
labels and invalid numeric wire values are rejected; failed text decoding retains
the previous value. Public Go callers must use the named constants or parsing
methods instead of string literals or string casts. This changes the Go enum
representation while preserving supported CLI and serialized labels.

No proof, classifier, reportability, fact producer or fact consumer policy changed.
EvidenceProvenance still identifies local SSA versus imported-fact evidence at
the proof boundary. The lifecycle Fact/MustClaims/MayClaims source review found
no provenance field in the exported fact schema; provenance remains attached to
proofs after evidence is obtained. This is a bounded source observation, not a
claim that every transitive fact path has been re-audited.

## Architecture protection

`TestNoNamedStringEnums` scans authored production and test files through the
shared source inventory. It rejects direct string definitions and direct string
aliases for Kind, Tier, Outcome, Provenance, Reason, State, Mode and Action
classification names. Matcher controls cover numeric definitions, owning-domain
aliases, textual IDs and opaque embedded fixture strings. The existing raw kind
guard remains active, and the two guards share their scan assertion.

This AST guard does not resolve indirect aliases or infer every arbitrary
string's semantic purpose. Codebase Memory tools were unavailable; evidence
comes from exact source reads, a direct declaration census, and authored-source
AST checks rather than a graph completeness claim.

## Validation and compatibility

Serialization controls cover every valid label and zero value of all four
numeric domains, round trips, unknown labels, invalid numeric values, numeric
JSON rejection and receiver preservation on failed decoding. Published manifest
classification decoder controls and the existing generated manifest check pass.
The shared SSA helper reference was regenerated for the new provenance methods.

The stable `make verify VERIFY_TIMINGS=1` run passed all eight gates:
generate 2s, module verification 1s, vet 2s, formatting 5s, dogfood 4s,
deadcode 8s, lint 7s, tests 12s. Earlier attempts exposed remaining literal
initializers/error returns, stale helper references, redundant alias casts,
duplicate architecture scans, and deadcode reflection reachability for catalog
decoders. Those were corrected before the stable run.

Ten parent/current CLI comparisons preserve exit status, stdout and stderr:
listing, check listing, missing/repeated selections, removed/unknown/missing tier
values and help. Four all-check fixture scans preserve complete diagnostic JSON:
producer 22, capture 15, goroutine ownership 126, cancellation ownership 37.
Every scan exits 0 with empty stderr. Their traced scans preserve complete
payloads, event contents and ordered event sequences per candidate: respectively
216, 74, 3248 and 1224 events per version. Global order also matches in this run;
per-candidate order is the required comparison.

Parent binary SHA-256:
`a06b3e2d8547fe8764d57036d55eb7fad691c0711f7986b631e222ff03b2cc76`.
Reviewed binary SHA-256 (also matches the canonical build):
`a067ea32d69d250859d1a1312059b93550d01ed80b60d3d2cb26c10dc348217e`.
Local receipts live under `.build/goal-numeric-enums-cli/` and
`.build/goal-numeric-enums-scans/`; verification logs use the same prefix.

No full precision replay, local race run or FP correction credit is claimed.
The five-site FP queue and broader transitive semantic consolidation review
remain open. This closes the named-domain enum task only.
