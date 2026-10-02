# Assessment of the 18 unresolved production FP sites

This assessment follows the [22-site successful scoped replay](pending-production-fp-replay-2026-10-01.md)
and its four verified corrections: caller-owned destination storage, HTTP Body
handoff, captured worker-field cleanup, and imported asynchronous exposure.
The original labels and replay ledger remain historical snapshots. This note
credits no further removals and performs no new scans. Source inspection uses
the pinned checkouts identified by that ledger. Beads `gohawk-dho.10` owns this
assessment; `gohawk-dho.4` retains implementation and unresolved assessments.

The later [registry-group follow-up](registry-group-followup-2026-10-02.md)
corrects both FDio sites using the existing opaque-group origin boundary,
without proving the callback lifecycle. That leaves 16 unresolved sites. The
18-site table below remains the historical assessment that motivated this
reassessment; its counts are not a latest-binary corpus replay.

The later [program-entry context follow-up](program-entry-context-followup-2026-10-02.md)
corrects the k8ssandra site using existing one-time entry evidence, leaving 15
unresolved sites. Its unknown outcome makes no cancellation guarantee.

## Remaining families

Counts refer to diagnostic locations, not repositories or independent defects.
The ten rows below account for all 18 unresolved locations.

| Family | Sites | Evidence and required next proof |
| --- | ---: | --- |
| coder/acp-go-sdk main process exit | 4 | The agent, claude-code, client and gemini examples start commands in `main`. The latter three use `CommandContext`; normal paths kill the process or cancel its context, without waiting. These actions and main exit are the audit's rationale, but do not supply exact `Wait` evidence. Decide the program-entry policy separately from reusable callee guarantees. Do not make `Kill` equivalent to `Wait`. |
| Skywalking lock/field association | 2 | Cursor fields `head` and `current` are updated while the event list is read under `eventLocker.RLock`; other paths update the cursor without this lock. Unlocked writes do not prove reader confinement. A correction needs guard/field association or exact participant confinement. |
| k8ssandra main cancellation | 1 | `cancel` is handed to `SetupWithManager` only in the control-plane branch. The other branch eventually returns from `main` after manager execution. Proving the handoff in one branch cannot cover the other; program-lifetime cancellation needs a separate justified boundary. |
| Debian producer/consumer ordering | 2 | Workers consume dynamically produced work and settle a group. A safe proof must relate producer completion, consumer progress, channel closure and the registered participants. No guessed loop count or generic `Wait` nearby can substitute for those relationships. |
| FDio registered callback owner | 2 | `Connected` obtains private data and launches a worker with its group; `Disconnected` retrieves private data, closes the quit channel and waits. Registration connects the callbacks and private data. The proof must preserve that exact owner across callbacks and establish the lifecycle contract. |
| Openase transport close/completion | 2 | The caller uses the application-defined `ssh.Session` interface. Its stream-copy workers capture returned readers; the start-failure return runs deferred session cleanup. Resolving the `realSession` forwarding methods is only one step: returned-reader/session identity and close/completion semantics are also required. |
| boxesandglue caller process exit | 1 | `createPatterns` leaves its output file open on HTTP/copy error returns; its command entry ultimately exits on those errors. The callee has an uncovered return. A caller-context proof could explain the audit judgement, but an unconditional cleanup summary would be false. |
| goiardi caller branch preconditions | 2 | The `importSave` callers check `config.UsingDB`; the SQL helpers open a transaction and return `NoDBConfigured` without rollback if neither mutable configuration flag is set. A proof needs the call-site condition and stability through intervening calls, rather than assuming the helper's branch is globally unreachable. |
| ferro transport constructor state | 1 | `NewIO` creates a transport with no command; its `Start` implementation returns early from spawning in that state. The unwaited error return needs a receiver-state-dependent result guarantee. A universal `Start` success fact or method-name exemption would overstate the contract. |
| rev-dep detached telemetry | 1 | `Dispatch` detaches a self-spawned child, clears output streams, writes/closes its input pipe, and deliberately omits `Wait`. Detachment intent is explicit, but does not itself establish `Process.Release` or a reusable ownership handoff. Keep this policy assessment separate from exact wait/release proofs. |

## Pinned source anchors

- coder: [agent](https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/example/agent/main.go#L401), [claude-code](https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/example/claude-code/main.go#L184), [client](https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/example/client/main.go#L191), [gemini](https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/example/gemini/main.go#L199).
- Skywalking: [cursor update](https://github.com/apache/skywalking-rover/blob/e83d5925500a7e63dd55c080a9b1542d6cedaefb/pkg/tools/buffer/buffer.go#L629-L654).
- k8ssandra: [creation and conditional handoff](https://github.com/k8ssandra/k8ssandra-operator/blob/2028d352ecb495de4b6e053d99d7a77b21eb5107/main.go#L176-L205).
- Debian: [producer](https://github.com/Debian/dcs/blob/567a9be49163cbf731f25bf79890f692e04d22d9/internal/sourcebackend/sourcebackend.go#L433), [worker](https://github.com/Debian/dcs/blob/567a9be49163cbf731f25bf79890f692e04d22d9/internal/sourcebackend/sourcebackend.go#L562).
- FDio: [bridge callbacks](https://github.com/FDio/govpp/blob/c71484d8c74da940abbd70407b53894fa4c56f01/extras/gomemif/examples/bridge/bridge.go#L33), [polling callbacks](https://github.com/FDio/govpp/blob/c71484d8c74da940abbd70407b53894fa4c56f01/extras/gomemif/examples/icmp_responder_poll/icmp_responder_poll.go#L61).
- Openase: [stream workers and cleanup](https://github.com/PacificStudio/openase/blob/e530faf137e764337d5beaaf68af3be159eb17aa/internal/infra/hook/remote_shell_executor.go#L100-L155), [interface and forwarding methods](https://github.com/PacificStudio/openase/blob/e530faf137e764337d5beaaf68af3be159eb17aa/internal/infra/ssh/pool.go#L365-L416).
- boxesandglue: [helper error returns](https://github.com/boxesandglue/boxesandglue/blob/79509f4b6b0e2e7a1d0562139ab4d9946d4be080/helper/pattern.go#L79-L113), [entry caller](https://github.com/boxesandglue/boxesandglue/blob/79509f4b6b0e2e7a1d0562139ab4d9946d4be080/helper/main.go#L10-L35).
- goiardi: [transaction helpers](https://github.com/ctdk/goiardi/blob/937cae400a92d8036b88ae2f65d93506271c292e/shovey/sql_funcs.go#L459-L516), [caller conditions](https://github.com/ctdk/goiardi/blob/937cae400a92d8036b88ae2f65d93506271c292e/shovey/shovey.go#L874-L889).
- ferro: [constructed transport and error return](https://github.com/ferro-labs/ai-gateway/blob/d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4/mcp/stdio.go#L158-L175).
- rev-dep: [detached dispatch](https://github.com/jayu/rev-dep/blob/8a2fdb0927e2fc9b2a5b178c94f55d1887659152/internal/telemetry/telemetry.go#L75-L103).

## Consequence for call-graph work

No site currently has a demonstrated correction from call-target resolution
alone. Openase's two interface-backed sites are the clearest experiment;
FDio's two callback sites could also benefit from callback edges, with owner
identity and lifecycle semantics still required. This is a source-based
assessment, not an implementation or benchmark of CHA, RTA or VTA. The other
14 sites primarily require the evidence described above.

The four recent corrections used existing modular evidence and shared value
mechanics. They do not establish that the whole architecture is consolidated
or that all easy false positives have been eliminated. The next architecture
review should inspect duplicated proof decisions and value/storage mechanics,
while keeping the unresolved families available for concrete bounded proofs.
Keep application-specific target sets and path assumptions separate from the
uninstantiated declaration guarantees published as per-package facts.
