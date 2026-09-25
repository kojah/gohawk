# Architecture in detail

This note extends the public [codebase layout](../architecture.md) with the
shared engine's layering, the rules the architecture tests enforce, and the
tooling details a contributor needs while working on it.

## Generation and verification timings

Documentation generation prints phase timings by default, including fixture
scanning, package loading, analyzer runs, page rendering, and file syncing.
The analyzer runs are also listed slowest first, with the fixture-root count
for each; these times include each check's prerequisite passes.
`make verify` also prints elapsed time and exit status for each gate. Set
`VERIFY_TIMINGS=0` to hide these measurements in Makefile workflows, or pass
`-timings=false` directly to `tools/gendocs`.

## Shared engine

- `internal/ssaflow` owns the reusable SSA mechanics: proof outcomes and
  budgets, value provenance (`ReachingWalk`), calls, and control-flow queries
  (`WalkStates` and `EvaluateObligation`). It provides how to walk, not an
  analyzer's reporting policy.
- `internal/lifecycle` builds completion and ownership-transfer
  proofs from `ssaflow` and `heapmodel`. Analyzers import the layer that owns
  the query they need; neither package forwards the other's API.
- `internal/heapmodel` owns demand-driven storage queries, the per-function
  points-to graph and its cache, heap-summary projection and registration,
  and application at call sites. `heapmodel.Storage` combines reaching-write
  and graph evidence without making unknown contents or truncated summaries
  into negative proofs. Consumers access its queries directly, not through
  forwarding wrappers in `lifecycle`. `QueryEscape` reads that same graph to
  distinguish confinement within an explicit scope, possible publication, and
  unknown evidence. Escape destinations never establish cleanup ownership;
  interpreting a transfer remains lifecycle policy.
- `internal/resourcemodel` proves exact owner-to-resource relationships over
  the existing heap/storage model and tracks a comparable per-path resource
  obligation. External API contracts can establish state transitions through
  those relationships; the consuming analyzer still decides whether to report.
  Lifecycle summaries can carry conditional transitions through helpers and
  across package boundaries.
- Every interprocedural question spends a `ssaflow.SearchBudget`, named
  `QueryBudget` or `SummaryBudget` unless a proof has a reason of its own, and
  a lifecycle analyzer draws each question's budget from one pool per
  candidate with `Within`, so the proof as a whole is bounded and every
  give-up reaches the candidate's trace. Exhaustion means what the question's
  own polarity says it means; a pool decides nothing about that.
- `ssaflow.PathGuards` remembers which arm of a branch a path took so a later
  branch on the same condition can be related to it. A stable guard, over
  parameters and constants or a Boolean computed once, cannot change within
  the invocation; a loaded guard reads a cell a hidden store could change.
  The engine reports which kind a contradiction is and each walk chooses:
  the obligation walk and lock order prune the other arm of a stable guard,
  resource lifetime and every walk treat a loaded contradiction as unknown.
- `heapmodel.Storage` is the shared, bounded query for local contents and stable
  owner projections. It resolves loads at their own execution points, including
  fields, constant array elements, and aggregate-copy snapshots. Completion,
  lifecycle facts, and analyzer-local identity checks use the same query rather
  than selecting stores independently. Historical containment remains a
  different question: it can justify uncertainty, never exact cleanup.
- `internal/passes/lifecyclefacts` exports a per-function summary of what a
  callee does to its parameters, so an analyzer can see through a call into
  another package. Consumers use `LifecycleEvidence`, which consults local
  evidence first and imported facts second. See
  [Inferred facts](fact-model.md).
- `internal/summaries` brokers typed function-summary components selected at
  analyzer setup. Result guarantees, lifecycle effects, and concurrency effects
  retain independent inference and fact passes. The broker selects ordinary
  prerequisites and exposes declaration summaries separately from bound
  call-site evidence; it is not another scheduler or a universal proof model.
- `internal/check` and `internal/trace` provide reporting and evidence
  tracing. Every diagnostic flows through `check.Report`, which is what lets
  the tracer record whether a candidate was reported, suppressed, or removed.

## Invariants the tests enforce

`internal/architecture` contains repository-wide conformance tests. Each one
guards a rule that the rest of this page relies on, so the architecture and
the code cannot drift apart silently.

| test | rule it enforces |
|---|---|
| `TestSourceInventoryExcludesNonProductionTrees` | source inventories exclude fixtures, generated files, tests, and dot/underscore-prefixed trees such as cached audit checkouts |
| `TestInternalPackagesRespectDependencyDirection` | analyzers may use shared tools; shared tools never depend on analyzers or the catalog |
| `TestAnalyzerPackageLayout` | one package per analyzer under `internal/analyzers/<group>/<name>` |
| `TestAnalyzersUseSharedReporting` | diagnostics only through `check.Report` or `check.Reportf`, never `analysis.Pass.Report` directly |
| `TestAnalyzerCodeUsesStructuredTracing` | production analyzer and analysis-pass code uses `internal/trace` rather than `fmt.Print`, `fmt.Printf`, or `fmt.Println` probes |
| `TestNoPublicCheckIsAConvention` | no listed check is kind `policy`: a diagnostic a reader must first agree with is withdrawn from the catalog rather than published |
| `TestTraceEventsAreAttributedToACandidate` | every trace event names the candidate whose proof it serves, so one finding's evidence can be selected out of a package's trace |
| `TestTransparentFormsAreNamedAtTheCallSite` | each proof names the SSA wrappers it may look through, so a form added later cannot widen a proof nobody reviewed for it |
| `TestCallGraphGuardsGoThroughTheSharedMemo` | a path-scoped call-graph guard goes through `ssaflow.CallGraphMemo`, so a walk covers the call graph rather than every call path through it |
| `TestInterproceduralSearchesNameABudget` | a completion request names a `ssaflow.SearchBudget`, so an interprocedural walk gives up rather than hanging on mutually recursive callees, and its caller decides what an abandoned search permits |
| `TestSearchBudgetsAreNamed` | a `SearchBudget` is constructed from `ssaflow.QueryBudget`, `ssaflow.SummaryBudget`, or a named constant beside the proof, never a bare number, so the size of a bound is a recorded decision rather than a copied neighbour |
| `TestSummaryInfrastructureBoundaries` | analyzers, SSA engines, and fact passes use the shared summary API; only the two implementation files own raw memo/guard operations and fields. Analyzer query sites must not pass literal nil budgets |
| `TestSummaryBoundaryMatcher` | summary API checks resolve type identity, including import aliases, generic types, promoted methods, and method expressions; unrelated lookalike names remain allowed |
| `TestReasonEnumBoundaryMatcher` | reason checks reject string aliases and raw reason fields while allowing textual observer/output boundaries |
| `TestNoRawReasonClassifications` | all production Go reason domains use numeric enums; raw reason fields, parameters, declarations, assignments, and literal classifications are rejected outside the two textual output-boundary files |
| `TestRawReasonClassificationMatcher` | migration accounting recognizes raw reason fields, parameters, assignments, and composite literals without treating ordinary display text as classification |
| `TestAnalyzersUseSymbolIdentity` | well-known functions matched through `syntax.Symbol`, not reconstructed from package paths and names |
| `TestProductionCodeReturnsTerminationDecisions` | no `panic`, `log.Fatal`, or `os.Exit` in analyzer or library code |
| `TestForbiddenTerminationIdentity` | the termination rule's matcher recognizes exactly the builtin `panic`, the `log.Fatal` variants, and `os.Exit`, and nothing else |
| `TestAnalyzerCommentaryCoverage` | non-obvious spans of analyzer code carry model-level rationale |
| `TestObjectFactsStayInTheirDefiningPackage` | object facts imported and exported only in the package that defines the fact type |
| `TestAnalyzersUseSummaryBroker` | analyzer access to lifecycle, concurrency, and result knowledge goes through a pass-level summary selection, not raw prerequisites or domain constructors |
| `TestSummaryBrokerMatchesDeclarationIdentity` | broker boundaries recognize aliases, dot imports, constructors, and type assertions without banning unrelated lookalike packages |
| `TestAnalyzersUseSharedTraversal` | value-provenance recursion — phi fan-out and visited sets — lives only in `ssaflow` |
| `TestLifecycleFamiliesLayerDownward` | `lifecycle` files are named by family — store, completion, evidence — and a file references declarations only from its own family or a lower one |
| `TestSemanticModelDependencyBoundaries` | Lifecycle proofs depend on heap evidence; synchronization queries consume prerequisite effects without depending on analyzers or the summary broker |
| `TestAnalyzerProseRulesSeparateVentingFromContracts` | the analyzer page vocabulary rule flags proof-boundary language and accepts contracts, inline code, and the design note link |
| `TestChecksSectionRuleAcceptsOnlyTheTable` | the rule that an analyzer page's Checks section holds only its generated table flags stray prose and accepts the table |
| `TestPublicDocumentationStaysConcise` | public pages stay within their line budget and carry no commit-pinned dogfood links; detail belongs in `docs/development/` |
| `TestDocumentationReferencesResolve` | the development docs and project skills cite only code that exists, and their helper, `Fact` field, and test inventories are complete |
| `TestSharedHelperReferencesStayCurrent` | package-specific shared API references match current signatures, comments, source links, and every prerequisite pass package |

Conventions that are not yet enforced by a test are described in the
repository's `AGENTS.md`.

Summary conformance is deliberately split between architecture and behavior.
The architecture check recognizes direct calls by type identity; it is not a
whole-program proof that a budget variable is non-nil or that every operation
charges it. Shared summary contract tests cover nested recursion and budget
cuts, independent cache entries, policy isolation, and call-site binding.
Analyzer tests must still establish the meaning of an incomplete answer:
positive witnesses may survive a cut, but missing effects cannot prove absence.
Binding adapters must produce their own result without mutating cached slices
or maps; the generic engine cannot deep-copy arbitrary analyzer evidence.

### Reason-code migration

Internal classifications use domain-owned numeric enums, with explicit unset
and invalid values. Convert to stable textual codes at tracing, serialization,
or display boundaries; keep free-form explanations separate. Do not place
analyzer-specific vocabulary in a single global reason catalog.

Synchronization queries preserve a typed graph failure or upstream concurrency
summary cause. Consumers must retain that cause rather than convert it to text
to move it between proof layers; only trace rendering chooses its external code.

The guard covers production Go code repository-wide, including analyzer wrappers
and the golangci plugin, with no migration-debt baseline. Only
`internal/trace/trace.go` and `internal/ssaflow/proof_observer.go` retain textual
reason transport. These are output boundaries, not inference APIs. Syntax checks
cannot infer every string's purpose: review unnamed helper return values and
dynamically synthesized codes too. Keep golden trace tests and enum-to-code
tests so representation refactors preserve external output.

## Why analyzers are organized this way

This follows Staticcheck's grouped one-package-per-pass layout while retaining
gosec's practice of splitting a substantial analyzer into focused files within
its package. Analyzer groups are catalog metadata mirrored by container
directories, not Go package boundaries.

Shared source-level helpers live under `internal/syntax`, while SSA traversal
mechanics live under `internal/ssaflow`, storage queries under `internal/heapmodel`,
and completion and transfer proofs under `internal/lifecycle`; they are implementation
details rather than an external integration API. Cross-cutting
diagnostic, catalog, flag, and trace infrastructure lives in its own focused
internal package instead of being folded into analysis utilities.
