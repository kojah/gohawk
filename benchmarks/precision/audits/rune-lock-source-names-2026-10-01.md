# Rune local lock message rendering

Implementation `c422971` reuses the existing acquisition source-name renderer
for local allocation identities in primary ordering-cycle messages. Declaration
classes retain their names; identities and cycle detection are unchanged.
The existing gate fixture fails before this change when it requires `gate`,
and passes afterward. Other declaration and longer-cycle fixtures still pass.
Final `make verify` passes, including ordinary tests, lint, vet and dogfood.

A successful scoped replay at rune pin
`3e2165f8983280542c985947378dfa740a397d03`, `./internal/ide/idepkg`, uses
`CGO_ENABLED=1`, `GOFLAGS=-mod=readonly`, `GOWORK=off`, and
`go vet -vettool=<immutable binary> -enable-all -json`. Terminal exit is 0;
stderr is empty. The binary `.build/gohawk-local-lock-names-final` has SHA-256
`cc850d926409bd91ccf2ef317319c6ef8968e4d64f34747a6cfe983314cf3ed9`.
Receipt: `.build/goal-rune-ide-source-names.json`.

Compared with `.build/goal-rune-ide-cgo-current.json` from the earlier
[callback follow-up](rune-callback-waiter-followup-2026-10-01.md), all diagnostic
keys and positions match. The one message at `manager.go:422:9` changes from
an `InstallPackageVersion:local:new:t59` identity to:

```text
contradictory lock order: struct{sync.Mutex; m map[string]*sync.Mutex}.Mutex and `mu`
```

This is a rendering correction, not an FP removal. Beads `gohawk-dho.7` owns
it; `gohawk-cnx` remains open for the publication-order precision question.
No candidate tests, applications or generators were executed. No full precision
replay ran. Frozen verdicts and the unresolved 20-site queue remain unchanged.
