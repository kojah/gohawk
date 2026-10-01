# Review correction: upload rejected before transport

This cohort preserves the production report at `cmd/release.go:191:14` in
promu at `304b60c9fb862b9fa5d897e93901740da729c13b`. The original batch 63
ledger called it a false positive because the HTTP transport closes its
request body. The [review correction](../audits/batch-63-review-corrections.tsv)
records the new true-positive judgment without rewriting the frozen verdict.

The pinned dependency is go-github v25.1.3, commit
`361256aa6e01256279b0805586b6c491ff2045d9`.
[UploadReleaseAsset](https://github.com/google/go-github/blob/361256aa6e01256279b0805586b6c491ff2045d9/github/repos_releases.go#L339-L380)
passes the file into a request, then delegates to `Client.Do`.
That [Do implementation](https://github.com/google/go-github/blob/361256aa6e01256279b0805586b6c491ff2045d9/github/github.go#L487-L501)
checks cached rate limits before calling the HTTP client. A depleted quota
with a future reset returns immediately; neither that branch nor its caller
closes the request body.

This is feasible in the [release callback](https://github.com/prometheus/promu/blob/304b60c9fb862b9fa5d897e93901740da729c13b/cmd/release.go#L145-L208):
the preceding asset-list request can successfully consume the last core quota
unit. Asset listing and upload share the core category. The upload then opens
a file and fails the cached check before transport takes it. The callback
returns an error without closing the file and can open another on retry;
the default retry count is two. This is source-backed path evidence, not a
runtime reproduction or a claim about newer dependency versions.

No analyzer change or suppression is warranted. The current trace still
reports an unowned return, and the site now serves as a genuine-bug control.
This correction is not credited as removal of a false-positive diagnostic.

Run `make precision-regression ROUND=round-65 REQUIRE_SCANNABLE=1`.
Candidate tests, generators and applications are not executed. The cohort
contains one reviewed label; it is not a fresh repository audit or a full
cumulative precision-regression run.

Validation with the binary containing `1075818` retained the one true-positive
label, with no unscannable exclusions. `go test ./internal/architecture -count=1`
also passed. This update changes audit records only; the analyzer code and its
previous passing `make verify` validation are unchanged.
