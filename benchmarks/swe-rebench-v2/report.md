# SWE-rebench V2 Go analyzer opportunity scan

This report scans every Go row in the SWE-rebench V2 executable dataset, then narrows prevalence claims to bug-labelled, quality-A tasks. Repair signals are deterministic properties of gold patch additions and removals; they overlap and are not a claim that every matching task has the same root cause.

Source: [nebius/SWE-rebench-V2](https://huggingface.co/datasets/nebius/SWE-rebench-V2), using the executable dataset Parquet snapshot with SHA-256 `0e0bf9355f892ad74ae98d4e1c404f39fd6654a8e351ee3e6ab162e4a64cd3ad`. See the adjacent README for the exact regeneration command.

## Population

- Go tasks: **6,144** across **967** repositories.
- Bug-labelled tasks: **2,985** across **734** repositories.
- Quality-A bug tasks used for repair-signal counts: **2,332** across **655** repositories.

Feature, documentation, infrastructure, ambiguous, and test-misaligned rows remain represented in the population totals but do not inflate the repair counts below.

## Dataset categories

| Category | Tasks |
|---|---:|
| `core_feat` | 2,965 |
| `minor_bug` | 1,469 |
| `major_bug` | 665 |
| `regression_bug` | 422 |
| `documentation_enh` | 205 |
| `edge_case_bug` | 201 |
| `dev_ops_enh` | 198 |
| `critical_bug` | 148 |
| `performance_bug` | 87 |
| `security_bug` | 51 |
| `integration_feat` | 42 |
| `ui_ux_feat` | 22 |
| `core_bug` | 4 |

## Repair signals and analyzer overlap

| Repair signal | Tasks | Repositories | Existing coverage | Disposition |
|---|---:|---:|---|---|
| nil guard introduced | 488 | 270 | Go nilness, Staticcheck SA5011, nilerr, and nilnesserr cover provable variants. | Do not add a broad check; most remaining cases require an API or domain precondition. |
| error guard introduced | 311 | 203 | errcheck, nilerr, nilnesserr, go vet, and Staticcheck cover common static mistakes. | Do not duplicate; inspect only narrower recurring contracts. |
| error chain operation introduced | 34 | 32 | errorlint checks direct comparisons and assertions involving wrapped errors. | Covered externally. |
| error wrapping introduced | 63 | 50 | errorlint and wrapcheck cover the relevant error-chain policies. | Covered externally. |
| resource close introduced | 32 | 28 | gohawk resourcelifetime, bodyclose, and sqlclosecheck cover well-known contracts. | Extend an exact resource contract only when a repeated API family is found. |
| cancellation release introduced | 8 | 7 | go vet lostcancel and gohawk cancellationownership cover derived contexts. | Covered. |
| lock operation introduced | 38 | 31 | The race detector, go vet copylocks, and gohawk lockorder cover important subsets. | The signal is too broad; retain only independently provable lock protocols. |
| defensive copy introduced | 23 | 22 | No mainstream analyzer proves general borrowed-versus-owned slice or buffer contracts. | Research target; deeper alias and ownership modeling is required before diagnostics. |
| deterministic sort introduced | 28 | 26 | gohawk determinism covers map iteration that reaches ordered output. | Covered for the high-confidence output-order contract. |
| timer or ticker stop introduced | 3 | 3 | gohawk resourcelifetime models time.NewTimer and time.NewTicker; Staticcheck covers time.Tick leaks. | Covered. |
| terminal iterator error check introduced | 18 | 17 | rowserrcheck covers database rows; errcheck and API-specific tools cover other iterators. | Prefer an exact API contract over a name-based check. |
| map allocation introduced | 65 | 51 | Staticcheck SA5000 catches assignments to maps proven nil. | Do not infer that every zero map needs allocation. |
| panic recovery introduced | 6 | 6 | No general recovery requirement exists; the boundary contract is application-specific. | Not suitable without a configured callback contract. |
| context-guarded channel send | 3 | 3 | No mainstream golangci-lint analyzer caught the replayed Bubble Tea defect; gohawk also missed it. | Prototype a conservative producer-lifecycle extension that proves the receiver can exit on the same cancellation signal. |
| context-aware retry delay | 1 | 1 | github/gh-aw provides timesleepnocontext; mainstream golangci-lint and gohawk missed the replayed OpenSearch defect. | Do not duplicate the broad rule; consider only stronger lifecycle evidence shared with channel sends. |
| live synchronization primitive reset | 1 | 1 | go vet copylocks rejects copied locks, but it and golangci-lint --default all missed the replayed fresh-lock replacement. | Prototype an opt-in hazard for overwriting a receiver that contains an already-live synchronization primitive. |
| transaction rollback defer introduced | 1 | 1 | gohawk resourcelifetime covers database/sql transactions; specialized transaction analyzers cover additional frameworks. | Prefer configurable exact contracts over another built-in general check. |

## Shortlist

1. **Context-guarded channel sends.** Extend producer lifecycle only when analysis can connect a sender and receiver to the same cancellation signal and prove that the receiver may exit first. A mere channel send in a context-taking function is not sufficient evidence.
2. **Live synchronization primitive reset.** Investigate whole-object assignments that overwrite a receiver containing a mutex, RWMutex, Once, WaitGroup, Cond, or noCopy-bearing atomic value. Start opt-in because reset methods may have a documented quiescence precondition.
3. **Alias and buffer ownership research.** Defensive-copy fixes recur across independent repositories, but a diagnostic needs evidence that borrowed storage escapes or is mutated after transfer. Keep this as modeling work rather than a syntax check.

The context-aware sleep pattern is real but already has a focused external analyzer. Transaction rollback is also covered for database/sql by gohawk and by specialist tools for additional frameworks. Neither should become a duplicate built-in check.

## Representative overlap replay

The three highest-confidence concrete gaps were replayed at their dataset base commits with gohawk at `937b55c4edcd` (`-enable-all`) and golangci-lint v2.13.2 (`--no-config --default all --tests=false`). Unrelated and style diagnostics were ignored; neither tool reported the repaired defect site.

| Task | Base commit | Defect site | Result |
|---|---|---|---|
| `charmbracelet__bubbletea-1372` | `6a1ebaa0ea00` | `tea.go` | No diagnostic for the cancellation deadlock. |
| `opensearch-project__opensearch-go-540` | `2464386c5b71` | `opensearchtransport/opensearchtransport.go` | No diagnostic for the cancellation-blind retry delay. |
| `casbin__casbin-1229` | `2557f8dd4b37` | `persist/cache/cache_sync.go` | No diagnostic for replacing a live RWMutex. |

## Representative tasks

### nil guard introduced

A fix adds a nil comparison that was absent from removed lines.

- `a-h__templ-887` (`a-h/templ`): performance: templ parse takes a long time and uses high CPU when unclosed void elements are used
- `adnanh__webhook-449` (`adnanh/webhook`): Return JSON format when referenced value is not a simple type
- `aftership__clickhouse-sql-parser-158` (`AfterShip/clickhouse-sql-parser`): Parser failed to parse MATERIALIZED view with REFRESH keyword
- `alecthomas__gometalinter-319` (`alecthomas/gometalinter`): errcheck is called incorrectly and silently fails
- `alecthomas__gometalinter-333` (`alecthomas/gometalinter`): Custom linters have nil PartitionStrategy

### error guard introduced

A fix adds an `err != nil` branch that was absent from removed lines.

- `a-h__templ-887` (`a-h/templ`): performance: templ parse takes a long time and uses high CPU when unclosed void elements are used
- `adnanh__webhook-449` (`adnanh/webhook`): Return JSON format when referenced value is not a simple type
- `aftership__clickhouse-sql-parser-158` (`AfterShip/clickhouse-sql-parser`): Parser failed to parse MATERIALIZED view with REFRESH keyword
- `alecthomas__gometalinter-319` (`alecthomas/gometalinter`): errcheck is called incorrectly and silently fails
- `allegro__marathon-consul-145` (`allegro/marathon-consul`): Health check invalid port number

### error chain operation introduced

A fix starts using errors.Is or errors.As.

- `ably__ably-go-629` (`ably/ably-go`): ably-go doesn't retry requests to fallback hosts on a timeout
- `apigee__registry-1230` (`apigee/registry`): Registry tool: --config option fails to accept file name arguments
- `apigee__registry-1232` (`apigee/registry`): Configurations are read inconsistently in registry tool command implementations
- `chef__automate-4110` (`chef/automate`): Automate deploy fails when the linux hab user comes from external auth instead of local accounts
- `deepmap__oapi-codegen-572` (`deepmap/oapi-codegen`): If `MultiError` is enabled via `chi_middleware.OapiRequestValidatorWithOptions()`, the sever always return 500 as status code about invalid request.

### error wrapping introduced

A fix introduces a `%w` error wrap.

- `a-h__templ-1026` (`a-h/templ`): cmd: rebuild and refresh when a go file is changed in `--watch` mode
- `aiven__terraform-provider-aiven-2089` (`aiven/terraform-provider-aiven`): Importing aiven_pg_user forces recreation after fork and restore
- `argoproj__argo-4594` (`argoproj/argo`): WorkflowEventBinding passes raw go structs instead of json marshalled payload params
- `bufbuild__buf-3665` (`bufbuild/buf`): Zero length encoding is considered invalid.
- `celestiaorg__nmt-179` (`celestiaorg/nmt`): VerifyLeafHashes can panic in the completeness check if proof.nodes are not namespaced

### resource close introduced

A fix adds a Close call for an acquired value.

- `apache__camel-k-779` (`apache/camel-k`): Integration naming issues with numbers in them
- `apache__trafficcontrol-7744` (`apache/trafficcontrol`): /server_server_capabilities in TO API uses non-RFC3339 date/time strings
- `aws__aws-sdk-go-3183` (`aws/aws-sdk-go`): memory usage of s3 upload manager for Go 1.13.5 and 1.13.6
- `benhoyt__goawk-201` (`benhoyt/goawk`): Wrong order of output with pipe and print
- `blevesearch__bleve-1416` (`blevesearch/bleve`): term field reader stats out of sync when advancing backwards

### cancellation release introduced

A fix adds a call to a cancel function.

- `go-kratos__kratos-1945` (`go-kratos/kratos`): ResponseEncode should not write any data when it encode nil
- `go-kratos__kratos-1950` (`go-kratos/kratos`): the bind test has some errors
- `kubernetes-sigs__azuredisk-csi-driver-657` (`kubernetes-sigs/azuredisk-csi-driver`): Mounted volume size issue when cloning a larger size pvc
- `nats-io__nats.go-838` (`nats-io/nats.go`): Fetch ignores context timeout.
- `peak__s5cmd-597` (`peak/s5cmd`): [BUG] Sync Continues, Despite Failure to List S3 Bucket

### lock operation introduced

A fix adds a mutex lock or unlock operation.

- `apache__pulsar-client-go-957` (`apache/pulsar-client-go`): Consume Performance drops when set EnableBatchIndexAcknowledgment = true
- `argoproj__argo-cd-5661` (`argoproj/argo-cd`): Argo CD too frequently fetches Helm repo index that results in a lot of traffic
- `aws__aws-sdk-go-3183` (`aws/aws-sdk-go`): memory usage of s3 upload manager for Go 1.13.5 and 1.13.6
- `benbjohnson__litestream-93` (`benbjohnson/litestream`): cannot verify wal state: ...-wal: no such file or directory
- `casbin__casbin-1491` (`casbin/casbin`): [Bug] Data race in RoleManagerImpl.HasLink() breaks Enforcer when calling Enforce()

### defensive copy introduced

A fix introduces an explicit copy or Clone call.

- `ably__ably-go-629` (`ably/ably-go`): ably-go doesn't retry requests to fallback hosts on a timeout
- `argoproj-labs__applicationset-282` (`argoproj-labs/applicationset`): Issue #170 is not solved when using `server`, only when using `name` for destination
- `dymensionxyz__dymension-1668` (`dymensionxyz/dymension`): eibc order tracking_packet_key field malformed upon export
- `fxamacker__cbor-257` (`fxamacker/cbor`): bug: encoding uninitialized cbor.(Raw)Tag returns malformed CBOR data
- `github__git-lfs-1616` (`github/git-lfs`): running "git lfs track" in a large repository is very slow

### deterministic sort introduced

A fix introduces sorting that was absent from removed lines.

- `ably__ably-go-629` (`ably/ably-go`): ably-go doesn't retry requests to fallback hosts on a timeout
- `aquasecurity__tfsec-1009` (`aquasecurity/tfsec`): Running tfsec multiple times gives different results on AWS099
- `aws-cloudformation__rain-541` (`aws-cloudformation/rain`): build command produces incorrect output for resource type AWS::ECR::ReplicationConfiguration but only in non-bare mode
- `crossplane-contrib__provider-aws-2206` (`crossplane-contrib/provider-aws`): AWS EC2 Route Tables with Tags, never gets Ready
- `docker__compose-8658` (`docker/compose`): docker compose exec environment variable precedence in v2.0.0-rc.1

### timer or ticker stop introduced

A fix adds a Stop call.

- `apache__pulsar-client-go-957` (`apache/pulsar-client-go`): Consume Performance drops when set EnableBatchIndexAcknowledgment = true
- `opensearch-project__opensearch-go-540` (`opensearch-project/opensearch-go`): [BUG] Request context cancellation is ignored when `retryBackoff` is configured
- `pion__mediadevices-75` (`pion/mediadevices`): GetUserMedia doesn't close driver if request is partially successed

### terminal iterator error check introduced

A fix adds an Err call after iteration.

- `ariga__atlas-3508` (`ariga/atlas`): DDL parser error is obsure when "table" is misspelled
- `aws__aws-sdk-go-3183` (`aws/aws-sdk-go`): memory usage of s3 upload manager for Go 1.13.5 and 1.13.6
- `cayleygraph__cayley-848` (`cayleygraph/cayley`): NextPath is broken on Postgres
- `gocql__gocql-1368` (`gocql/gocql`): cancellation of a single query affects multiple queries in the same session
- `godbus__dbus-232` (`godbus/dbus`): Errors with a new connection if previous was closed

### map allocation introduced

A fix explicitly allocates a map.

- `alecthomas__gometalinter-289` (`alecthomas/gometalinter`): Should invoke linters once with multiple paths if the linter supports it
- `apache__pulsar-client-go-957` (`apache/pulsar-client-go`): Consume Performance drops when set EnableBatchIndexAcknowledgment = true
- `apache__trafficcontrol-7744` (`apache/trafficcontrol`): /server_server_capabilities in TO API uses non-RFC3339 date/time strings
- `aquasecurity__tfsec-871` (`aquasecurity/tfsec`): AWS018 failures
- `argoproj__argo-2976` (`argoproj/argo`): Retry Strategy doesn't work in GKE preemptible node (pod deleted)

### panic recovery introduced

A fix adds recover at a callback or process boundary.

- `creachadair__jrpc2-42` (`creachadair/jrpc2`): `PushCall` blocks indefinitely if remote side panics
- `go-kratos__kratos-3134` (`go-kratos/kratos`): logging.Server prints a wrong caller path
- `gobuffalo__pop-775` (`gobuffalo/pop`): pop.Connection.Transaction can leak connections
- `helm__helm-8141` (`helm/helm`): strvals parsing fails on negative index
- `kubernetes__klog-279` (`kubernetes/klog`): segfault for nil fmt.Stringer

### context-guarded channel send

A bare channel send is replaced by a select that can observe context cancellation.

- `charmbracelet__bubbletea-1372` (`charmbracelet/bubbletea`): Deadlock on context cancellation (inside eventLoop)
- `drand__drand-851` (`drand/drand`): Flaky Test: core/TestDrandPublicStreamProxy
- `gocql__gocql-1368` (`gocql/gocql`): cancellation of a single query affects multiple queries in the same session

### context-aware retry delay

A time.Sleep retry delay is replaced by a timer selected with context cancellation.

- `opensearch-project__opensearch-go-540` (`opensearch-project/opensearch-go`): [BUG] Request context cancellation is ignored when `retryBackoff` is configured

### live synchronization primitive reset

A whole receiver assignment containing a fresh mutex is removed in favor of preserving the existing lock.

- `casbin__casbin-1229` (`casbin/casbin`): [Bug] SyncedCachedEnforcer: RUnlock of unlocked RWMutex

### transaction rollback defer introduced

A fix introduces deferred rollback for a transaction-like resource.

- `gobuffalo__pop-775` (`gobuffalo/pop`): pop.Connection.Transaction can leak connections

## Validation protocol for a proposed check

1. Run the proposed analyzer on the dataset base commit and require a diagnostic at the defect site.
2. Apply the gold patch and require that the diagnostic disappears.
3. Run gohawk and golangci-lint with all checks enabled on both revisions to document overlap rather than infer it from names.
4. Minimize one diagnostic and multiple accepted forms into local fixtures.
5. Dogfood unrelated repositories before enabling the check; ambiguous ownership or lifecycle evidence suppresses the diagnostic.

## Limitations

- Gold patches can mix the actual repair with refactoring, generated changes, and test maintenance.
- Patch signals overlap; counts must not be summed.
- Quality-A is the dataset's LLM-assisted task-quality label, not a human proof that every changed line fixes the issue.
- Absence of a patch signal does not imply absence of that bug class.
- Replaying historical repositories can fail because of retired toolchains, dependencies, CGO libraries, or network services.
