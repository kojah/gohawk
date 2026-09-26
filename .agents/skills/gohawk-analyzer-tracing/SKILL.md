---
description: Use when adding, changing, or reviewing structured evidence tracing in a gohawk analyzer, replacing temporary print probes, or choosing trace phases, outcomes, reasons, and safe details.
metadata:
    source: project
name: gohawk-analyzer-tracing
---

# Tracing gohawk analyzer decisions

Use GoHawk's existing `internal/trace` package to make one focused rerun explain
why an analyzer accepted, rejected, or could not prove a candidate. Tracing is
developer diagnostics, not analyzer policy: the proof remains authoritative and
the trace describes that proof without changing it.

For reading traces and inspecting the SSA or lifecycle facts behind them, use
[gohawk-debugging](../gohawk-debugging/SKILL.md). The complete flag and event
schema is in the
[debugging reference](../../../docs/development/debugging-reference.md).

## Instrument the owned decision boundary

Keep trace calls beside the analyzer policy that owns the evidence. Do not add
a second Boolean decision path purely for tracing.

- Bind all steps to the potentially reportable source position with
  `trace.For`. Use `trace.ForPackage` only for work that genuinely has no
  candidate.
- Check `Probe.Enabled` before building maps, formatting instruction text, or
  collecting trace-only metadata.
- Emit `candidate` before a walk or proof phase that may dominate wall time, so
  a trace ending there localizes a stall.
- Emit `label` once per instruction a lifecycle classifier labels settled,
  transferred, or unknown, with the label's reason; leave instructions
  labelled none untraced.
- Emit `evidence` for a fact that materially supports or rejects the proof.
- Emit `considered` when a meaningful suppression or alternative was tested
  and did not hold.
- Emit exactly one final `decision` from the proof's outcome and reason.
- Emit `fix` when suggested-edit availability or rejection needs explanation.
- Let the shared engine report its own give-ups: build the budgets a
  candidate's queries share with `ssaflow.NewSearchBudget(n).Observed(probe.Observer())`.
  `ssaflow` never imports the tracer; it reports through the budget's
  observer, and a disabled probe attaches nothing, so this is free when off.

Prefer a structured proof with an outcome and reason code. Map that outcome to
`trace.OutcomeAccepted`, `trace.OutcomeRejected`, or `trace.OutcomeUnknown`;
do not recompute the result in the tracing helper. Diagnostics themselves must
still flow through `check.Report` or `check.Reportf`.

## Keep the trace useful and safe

- Use stable, concise kebab-case reason codes that answer “why?”. Treat them as
  a diagnostic interface rather than prose to rewrite casually.
- Keep details bounded: classifications, validated enums, booleans, counts,
  types, and the relevant SSA instruction are useful. Do not emit source
  contents, runtime values, command arguments, environment variables,
  credentials, or dependency output.
- Source positions, function identities, and SSA instruction text are allowed
  in this developer-local trace because they are necessary to map a proof back
  to analyzed code. Do not extend that allowance to runtime data.
- Never write analyzer traces to stdout. The shared tracer owns JSONL
  serialization to stderr or `-gohawk-trace-file`, preserving `-json` output.
- Keep disabled tracing behaviorally invisible and effectively allocation-free.
  Trace initialization or writes must not change diagnostics or exit status.
- Avoid per-instruction or per-loop noise unless each event records distinct
  evidence used by the final proof. Aggregate repeated observations.

Use structured tracing instead of committed `fmt.Print*` probes. A temporary
print is acceptable only for a genuinely disposable measurement; remove it
before committing and verify that it cannot contaminate `-json`.

## Verify the contract

Add focused coverage beside the instrumented analyzer when the new event
captures a precision boundary. Assert stable phase, reason, outcome, and
candidate association rather than the complete serialized line.

Run:

1. The changed analyzer package tests.
2. `go test ./internal/trace` when the shared tracer changes.
3. A focused traced invocation proving the expected event appears.
4. A `-json` invocation proving tracing does not corrupt diagnostic output.

Exercise accepted, rejected, and unknown outcomes when the changed proof can
produce them. If a path remains untraced, name that limitation in the handoff.
