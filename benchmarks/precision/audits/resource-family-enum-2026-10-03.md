# Resource contract family enum

Beads `gohawk-dho.23.41` closes a classification gap found during completion
review. At `116376a8`, resource contracts still declared `family string` and
used raw labels to select HTTP, SQL, compression and process-exit policies.
The existing architecture checks guarded kind declarations and phase/mode
parameters, but did not guard this family discriminator.

`resourceFamily` now owns numeric unknown, OS, SQL, HTTP, compression and
inferred-owner constants beside the acquisition contracts. All constructors,
policy comparisons and fixtures use those constants. Unknown and inferred
owners retain the prior default branches. A fixture's non-HTTP placeholder
becomes the unknown family rather than gaining OS policy. Package paths,
API names and diagnostic role labels remain textual identities.

The resource contract file already exceeds the review size threshold. The
enum stays there because it owns the same acquisition/policy vocabulary and
adds no second proof engine or external boundary. No new production function
is introduced. The callback test's closed proof-family selector becomes a
local numeric enum with labels used only for subtest names.

## Architecture evidence

One discriminator-name selector now serves the syntax and type gates for kind
and family fields, local declarations and parameters. Named Family domains
join the existing closed-domain suffix checks. Phase and mode wire fields
remain textual; their parameters remain guarded. The existing matcher fixture
runner is shared by the reason and named-domain tests instead of repeating it.

Raw family fields and parameters, named string families and indirect imported
aliases have focused controls. Removing family selection and the Family suffix
from the final guard makes six control assertions fail:
`.build/goal-family-guard-final-counterfactual.log`. The corresponding overlay
is `.build/goal-family-guard-final-overlay.json`.

These checks cover authored source under the existing inventory exclusions;
syntax covers inactive build files and type evidence covers current root-module
build/test variants. They do not infer closed semantics for arbitrarily named
text. Graph tools were unavailable, so the review used exact source. The current
production string-case review finds API/builtin names, HTTP header names and
CLI input parsing, rather than another resource-family decision representation.

## Behavior and validation

The five changed production files contain 52 function bodies. Their exact token
streams match the parent after substituting enum constants with their previous
labels. This does not prove arbitrary runtime correctness, but verifies that
the migration changes representation rather than the policy bodies. Receipt:
`.build/goal-resource-family-body-check/results.json`.

Four clean pinned scopes run every check with resource tracing, readonly module
resolution, `CGO_ENABLED=0` and `GOWORK=off`. All terminate with exit zero and
empty stderr. Candidate tests, generators and applications are not executed.

| Repository | Pin | Package | Complete diagnostics | Trace records |
| --- | --- | --- | ---: | ---: |
| urunc-dev/urunc | `ef1dc96a6bf0c188fc7714200d66d95557ae8af3` | `./internal/metrics` | 0 | 6,459 |
| ctdk/goiardi | `937cae400a92d8036b88ae2f65d93506271c292e` | `./shovey` | 9 | 38,178 |
| Contextualist/acp | `579b477d0281df41ab8753a7cbcb8f7807e52e2c` | `./pkg/pnet` | 0 | 7,249 |
| prometheus/promu | `304b60c9fb862b9fa5d897e93901740da729c13b` | `./cmd` | 1 | 13,550 |

All ten diagnostics and all 65,436 trace records match their parent multisets.
The repaired urunc result and Promu's reviewed leak are preserved. Goiardi's
two assessed FP targets remain reported; this migration receives no FP credit.
Complete payloads, clean-pin checks, terminal receipts and comparison data:
`.build/goal-resource-family-pinned/`.

Reviewed binary SHA-256:
`4319d29f5d5688f7c753dcd6614293d4e5de6a43f11bba9235c76e952ad2ac80`.
It implements `116376a8` plus this production family migration; later test-runner
and documentation edits do not change its production behavior.

Focused resource and architecture tests pass. The first canonical run exposed
line-length and repeated test-runner issues, which were corrected. The final
`make verify VERIFY_TIMINGS=1` passes all eight gates in
`.build/goal-resource-family-final-verify.log`. Final architecture validation
after documentation uses `.build/goal-resource-family-final-architecture.log`.
The full precision-regression corpus is not rerun for this migration.
