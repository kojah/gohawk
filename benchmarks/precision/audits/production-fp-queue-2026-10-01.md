# Production false-positive queue refresh

This refresh covers the **55 production locations** originally labelled false
positive in frozen batches 62 and 63. The [per-site queue](production-fp-queue-2026-10-01.tsv)
keeps every original key, pin, reason, and verdict beside subsequent evidence
and a next action. It changes neither frozen ledger nor precision totals.
The 68 historical test-file FP labels are outside the current production-only
profile; excluding them is not a correction. Other historical batches and
the separate rune issues remain outside this bounded inventory.

| Current evidence | Sites |
| --- | ---: |
| Successful current package scan: absent | 14 |
| Successful current complete repository scan: absent | 1 |
| Earlier focused correction receipt: absent | 16 |
| Earlier successful package receipt, incomplete root: absent | 1 |
| Original FP review corrected to a genuine leak | 1 |
| No current replay receipt yet | 22 |
| Total original production FP locations | 55 |

The first two rows describe current observations. Earlier receipts are retained
with their scopes, rather than presented as a new replay of the entire queue.
These counts do not credit 32 newly fixed reports: several are already absent,
others have earlier correction records, and one original judgment was wrong.
The 22 unreplayed sites are unresolved, not assumed present or fixed.

## New observations

The executable implementing production source through `e13c006`, SHA-256
`2d4f92f8f00cd43cb884aba688fd578bc583f816924a367139de28be2a50195a`,
completed four selected package scans with `-enable-all -json`,
`CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, and `GOWORK=off`:

| Pinned repository | Package scope | Original FPs now absent |
| --- | --- | ---: |
| Authula/authula | `./plugins/oauth2/services` | 2 |
| koki-develop/gat | `./docs` | 4 |
| ubuntu/adsys | `./internal/ad/admxgen`, `./internal/ad/admxgen/dconf`, `./internal/ad/common` | 5 |
| tigrisdata/tigrisfs | `./core` | 2 |

Authula's body guards and gat/adsys's deferred cleanup reports need no further
patch at those sites. Tigris's two merged-response reports are also absent;
its two original production concurrent-capture true positives in the same
package remain. Other Tigris findings are not newly labelled by this refresh.
The source pins and local JSON receipts are recorded per row. No introducing
commit is inferred from a current absence.

Urunc's fourteenth current package absence is the scoped correction in
[round 69](../round-69/README.md). Its CGO-disabled root scan remains incomplete.
Tencent's complete current absence comes from
[round 70](../round-70/README.md), alongside a retained production wait control.
Promu's corrected true-positive judgment is preserved in
[round 65](../round-65/README.md), not counted as an analyzer fix.

## Remaining work

The unreplayed locations cluster into these source-review families:

| Family | Sites | Next investigation |
| --- | ---: | --- |
| Process lifetime at `main` exit | 4 | Recheck the exact policy and feasible exits; do not add a `main` exemption. |
| Lock/field association | 2 | Verify which mutex guards which storage before reporting read-lock writes. |
| Producer/consumer ordering | 2 | Inspect exact producer completion and join values, without guessing loop counts. |
| Registered callback owner | 2 | Resolve stored worker groups and concrete registration handoffs. |
| Transport close completes workers | 2 | Check exact pipe/session identity and shutdown contract. |
| Caller branch precondition | 2 | Separate local feasible paths from caller-only assumptions. |
| Other distinct handoffs and shutdown contracts | 8 | Replay the affected package and inspect SSA/trace before proposing a model. |

The last row includes channel handoff, profiling writer retention, main cancel
handoff, caller process exit, transport-start guarantees, indirect receiver
storage, detached launch, and server shutdown. These are original review
descriptions, not proof that every family merits a larger engine.

Beads epic `gohawk-dho` tracks the overall consolidation objective. Queue item
`gohawk-dho.2` owns this inventory; `gohawk-dho.3` owns the immediate process
guard correction. Existing rune items `gohawk-cnx` and `gohawk-tz7` remain
separate investigations. Prioritize bounded identity and handoff questions;
keep corpus-wide validation out of each development iteration. Candidate
tests, generators, and applications were not executed.
