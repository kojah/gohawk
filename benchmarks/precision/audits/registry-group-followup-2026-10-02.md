# Embedded registry-group follow-up

Beads `gohawk-dho.31` reassesses the two FDio registered-callback findings from
the historical [18-site assessment](remaining-fp-assessment-2026-10-01.md).
Both now have successful before/after scoped receipts and are corrected.
The remaining production FP queue has 16 unresolved locations. Frozen batch
verdicts, historical precision totals and earlier replay ledgers are unchanged;
this is a correction receipt, not a new corpus audit.

## Evidence and correction

In both `Connected` callbacks, `GetPrivateData` returns an owner registered
elsewhere. The worker receives the address of its embedded WaitGroup and
registers deferred `Done`. A separate disconnect callback waits on that group.
The actual bridge SSA contains a comma-ok assertion, a captured local cell,
a load from that cell, and a field address supplying the worker's group.
`.build/goal-registry-group-parent.ssa.txt` records it; the minimized fixture
SSA is `.build/goal-registry-group-fixture.ssa.txt`.

`opaqueGroupOrigin` already declined local join ownership for a group supplied
by an unreadable factory or registry. Its reaching-value leaf now follows
embedded field addresses to their bases, after its existing storage resolution.
This is possible external ownership, not proof that a disconnect callback runs,
that the callbacks share exact private data, or that a join was completed.
The ordinary obligation walk, facts, receiver contracts and callback target
resolution are unchanged. No function-name or framework exemption is added.

The chosen direction is to widen this existing unknown boundary. A new
registration lifecycle engine is unnecessary to abstain here. A factory that
really returns an exclusively owned fresh aggregate can also become unknown;
that false negative is accepted. Local fresh owners and visible fresh factories
remain reportable. Storage resolution stays first so a fresh pointer installed
into an opaque owner's group field does not inherit its factory provenance.
Unresolved pointer loads and non-field projections are outside this extension.
A temporary imported constructor that visibly allocates a fresh owner confirmed
this recall loss: the consumer's final decision was unknown. Its accepted
false-negative fixture was removed, following the project policy; the gap is
recorded in the retained fixture header and design note. The probe source is
kept only in `.build/goal-registry-group-imported-fresh-*.go.txt`.

The minimized `registry_group_fields.go` fixture covers direct and captured
registry owners, nested fields and mixed local/registry origins. Controls retain
missing joins for local owners, visible fresh constructors, an unrelated opaque
factory call and a freshly installed pointer group. The parent overlay fails
three accepted forms and their unknown trace expectations; the current package
tests pass. Existing trace assertions consume that same fixture run.

## Scoped replay

Production-source baseline: `e4cb0ae`, the previous canonical gate's binary,
retained byte-for-byte as `.build/goal-registry-group-parent`. Its SHA-256 is
`8dba896b675a63e28030f4b01db38045e336d303ffa1d9105878cd80329fd76f`.
The corrected binary is `.build/goal-registry-group-current`, built from the
same source plus this field-origin change; SHA-256
`68ac5a420286b77de2814d0eba3cbc72e8a627ee481a37c79c88767fe0210423`.
These are pre-commit binaries; embedded Git metadata may identify their dirty
build trees. The hashes identify the exact executables used, not a clean-tree
VCS stamp. Neither executable is changed during the scoped scans.

All scans use `-enable-all -json`, `CGO_ENABLED=0`,
`GOFLAGS=-mod=readonly` and `GOWORK=off`. The FDio scopes are
`./examples/bridge ./examples/icmp_responder_poll`, from `extras/gomemif` under
the pinned repository's `extras` module. Stargz uses `./store` from its root.
Both FDio scans additionally enable `-gohawk-trace=goroutineownership` and
write JSONL to separate trace files, preserving diagnostic JSON output.

| Repository and pin | Baseline | Corrected |
| --- | --- | --- |
| FDio/govpp, `c71484d8c74da940abbd70407b53894fa4c56f01` | Exit 3; both original reports present, final decisions `unowned-return`/rejected. | Exit 0; both absent, final decisions `opaque-ownership-transfer`/unknown. |
| containerd/stargz-snapshotter, `624678b4e421947534cbf0618f9609853cccee0f` | Exit 3; reviewed worker TP at `store/manager.go:193:2` present. | Exit 3; that TP retained, with byte-identical nonempty diagnostic JSON (827 bytes). |

Every stderr file is empty. Exit 3 here denotes emitted diagnostics, not a load
failure. The exact per-site findings and receipts are in the
[follow-up TSV](registry-group-followup-2026-10-02.tsv). Stargz's original
review remains in `batch-51-findings.tsv`; no new verdict is inferred from the
presence alone. Candidate tests, generators and applications were not run.

Focused analyzer and traversal/commentary architecture controls pass. The final
canonical local gate passes all checks (`.build/goal-registry-group-final-verify.log`),
including ordinary tests (15 seconds), formatting, vet, lint, dead-code and local
dogfood. The initial gate found assertion-table formatting; the canonical
formatter corrected it. After checking and removing the temporary imported-fresh
false-negative probe, the final gate was repeated on the retained source.
No full precision regression or local race run was performed.
