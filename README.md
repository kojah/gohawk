# GoHawk

[![CI](https://github.com/kojah/gohawk/actions/workflows/ci.yml/badge.svg)](https://github.com/kojah/gohawk/actions/workflows/ci.yml)

GoHawk is a focused collection of static analyzers for Go. It currently
ships sixteen framework-neutral checks for API shape, concurrency and
resource ownership, determinism, serialization, tests, and error handling.

## Install

```sh
go install github.com/kojah/gohawk/cmd/gohawk@latest
```

Run GoHawk directly against package patterns:

```sh
gohawk ./...
```

It can also run as a `go vet` tool:

```sh
go vet -vettool="$(command -v gohawk)" ./...
```

## CLI usage

With no analyzer flags, GoHawk runs every analyzer. Passing one or more analyzer
flags runs only the selected checks:

```sh
gohawk -wirepolicy -globalstate ./...
```

Set an analyzer flag to `false` to exclude it while leaving the remaining
analyzers enabled:

```sh
gohawk -globalstate=false ./...
```

Print build and version information with `gohawk -V`.

### Analyzer configuration

Analyzer-specific options use the standard `go/analysis` flag mechanism. In a
multi-analyzer command, the option is prefixed with the analyzer name:

```sh
gohawk -goroutineownership \
  -goroutineownership.require-join \
  ./...
```

`goroutineownership` accepts context-controlled workers by default. Set
`-goroutineownership.accept-context-lifecycle=false` to disable only that
allowance, or use `-goroutineownership.require-join` to require a completion
signal or wait instead of context or lifecycle ownership. These options also
work through `go vet -vettool=...`.

In normal text mode, GoHawk exits with status 0 when the analysis is clean and
status 3 when it reports diagnostics. Successful JSON and suggested-fix runs
exit with status 0 even when their output contains diagnostics or a diff, so
automation using `-json` or `-fix -diff` must inspect the output. When GoHawk is
used through `go vet`, the `go` command controls the exit status and normally
uses status 1 for diagnostics.

## Suggested fixes

GoHawk provides suggested fixes when a diagnostic has one unambiguous,
behavior-preserving rewrite. Preview the available changes as a diff:

```sh
gohawk -fix -diff ./...
```

Or apply them in place:

```sh
gohawk -fix ./...
```

Suggested fixes are currently available for unkeyed `wirepolicy` literals,
missing unconditional `testpolicy` helper calls, and leaked cancel functions
reported by `cancellationownership`. Design-dependent findings remain
diagnostic-only.

## Analyzers

### API and data contracts

| Analyzer | Policy |
| --- | --- |
| `apishape` | Flags exported APIs with error-prone parameter or receiver shapes. |
| `contextpolicy` | Checks context placement, storage, nil use, and test ownership. |
| `closedomain` | Finds builtin strings used as closed semantic domains. |
| `wirepolicy` | Checks serialized structs and their composite literals. |

### Ownership and lifecycle

| Analyzer | Policy |
| --- | --- |
| `cancellationownership` | Checks that context and signal-derived cancellation functions are called. |
| `channelpolicy` | Checks channel capacity and closing ownership. |
| `goroutineownership` | Requires explicit goroutines to have a join handle or lifecycle owner. |
| `processownership` | Requires started commands to be waited on or transferred with their wait ownership. |
| `resourcelifetime` | Checks files, HTTP responses, SQL handles, timers, and compressors are released on every path. |

### Reliability and safety

| Analyzer | Policy |
| --- | --- |
| `determinism` | Detects unsorted map iteration reaching ordered output. |
| `errorownership` | Detects double-handled errors and error-text classification. |
| `globalstate` | Flags mutable package-level state. |
| `lockorder` | Detects contradictory mutex acquisition order. |
| `taintpolicy` | Checks untrusted environment and argument data reaching sensitive sinks. |

### Testing

| Analyzer | Policy |
| --- | --- |
| `blockingtest` | Checks cancellation ownership for blocking test channels. |
| `testpolicy` | Checks lifecycle ownership in test helpers. |

These are intentionally opinionated policy checks. A deliberate error-text
match can be suppressed immediately above the call with a comment containing
`gohawk:error-text-match` and a rationale. Intentional mutable package state can
be suppressed immediately above its declaration with
`//gohawk:globalstate <rationale>`.

## Suppressions

Any diagnostic can be suppressed on the same line or immediately above it with
an analyzer-specific directive and a required rationale:

```go
//gohawk:ignore goroutineownership worker belongs to the process lifecycle
go serveMetrics()
```

The form is `//gohawk:ignore <analyzer> <rationale>`. GoHawk deliberately does
not provide an unscoped ignore directive: the analyzer and reason must remain
visible in code review. The older `gohawk:error-text-match` and
`gohawk:globalstate` directives remain supported for their respective checks.

## Examples

GoHawk reports every violation and can rewrite the unambiguous cases described
under Suggested fixes. Each example below shows a representative pattern that
is flagged and one possible rewrite. Some analyzers enforce additional cases
summarized in the table above.

### `apishape` — group error-prone parameters

Flagged:

```go
func CreateUser(name, email, city, country, role string) error { return nil }
```

Preferred:

```go
type CreateUserInput struct {
	Name, Email, City, Country, Role string
}

func CreateUser(input CreateUserInput) error { return nil }
```

### `contextpolicy` — put context first

Flagged:

```go
func LoadUser(id string, ctx context.Context) error { return nil }
```

Preferred:

```go
func LoadUser(ctx context.Context, id string) error { return nil }
```

### `globalstate` — give mutable state an owner

Flagged:

```go
var users = map[string]User{}
```

Preferred:

```go
type Store struct {
	users map[string]User
}

func NewStore() *Store {
	return &Store{users: make(map[string]User)}
}
```

### `wirepolicy` — tag fields and key literals

Flagged:

```go
type EventRow struct {
	ID   string
	Kind string
}

var event = EventRow{"42", "created"}
```

Preferred:

```go
type EventRow struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

var event = EventRow{ID: "42", Kind: "created"}
```

### `testpolicy` — mark test helpers

Flagged:

```go
func requireUser(t *testing.T, user *User) {
	if user == nil {
		t.Fatal("expected a user")
	}
}
```

Preferred:

```go
func requireUser(t *testing.T, user *User) {
	t.Helper()
	if user == nil {
		t.Fatal("expected a user")
	}
}
```

### `blockingtest` — make test waits cancellable

Flagged:

```go
func waitForEvent(t *testing.T, events <-chan Event) Event {
	return <-events
}
```

Preferred:

```go
func waitForEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-t.Context().Done():
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}
```

### `goroutineownership` — join spawned goroutines

Flagged:

```go
func refresh() {
	go updateCache()
}
```

Preferred:

```go
func refresh() {
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		updateCache()
	}()
	group.Wait()
}
```

### `errorownership` — handle an error at one layer

Flagged:

```go
func load() error {
	if err := readConfig(); err != nil {
		log.Print(err)
		return err
	}
	return nil
}
```

Preferred:

```go
func load() error {
	if err := readConfig(); err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	return nil
}
```

### `channelpolicy` — let the creator close the channel

Flagged:

```go
func consume(events chan Event) {
	defer close(events)
	for event := range events {
		handle(event)
	}
}
```

Preferred:

```go
func consume(events <-chan Event) {
	for event := range events {
		handle(event)
	}
}
```

### `processownership` — wait for started processes

Flagged:

```go
func run(ctx context.Context) error {
	command := exec.CommandContext(ctx, "worker")
	return command.Start()
}
```

Preferred:

```go
func run(ctx context.Context) error {
	command := exec.CommandContext(ctx, "worker")
	if err := command.Start(); err != nil {
		return err
	}
	return command.Wait()
}
```

### `closedomain` — represent closed sets with named types

Flagged:

```go
type Job struct {
	State string
}

func finished(job Job) bool {
	return job.State == "done" || job.State == "failed"
}
```

Preferred:

```go
type JobState string

const (
	JobDone   JobState = "done"
	JobFailed JobState = "failed"
)

type Job struct {
	State JobState
}
```

### `taintpolicy` — validate untrusted input before a sink

Flagged:

```go
func runConfiguredTool() error {
	return exec.Command(os.Getenv("TOOL")).Run()
}
```

Preferred:

```go
func runConfiguredTool() error {
	tool, err := validateTool(os.Getenv("TOOL"))
	if err != nil {
		return err
	}
	return exec.Command(tool).Run()
}
```

### `lockorder` — acquire locks consistently

Flagged:

```go
func forward() { first.Lock(); defer first.Unlock(); second.Lock(); defer second.Unlock() }
func reverse() { second.Lock(); defer second.Unlock(); first.Lock(); defer first.Unlock() }
```

Preferred:

```go
func forward() { first.Lock(); defer first.Unlock(); second.Lock(); defer second.Unlock() }
func reverse() { first.Lock(); defer first.Unlock(); second.Lock(); defer second.Unlock() }
```

### `resourcelifetime` — release owned resources on every path

Built-in contracts cover files, transactions, SQL rows and statements, HTTP
response bodies, timers and tickers, and gzip/zlib readers and writers.

Flagged:

```go
func read(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return decode(file)
}
```

Preferred:

```go
func read(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return decode(file)
}
```

### `determinism` — sort map-derived output

Flagged:

```go
func names(users map[string]User) []string {
	var result []string
	for name := range users {
		result = append(result, name)
	}
	return result
}
```

Preferred:

```go
func names(users map[string]User) []string {
	var result []string
	for name := range users {
		result = append(result, name)
	}
	slices.Sort(result)
	return result
}
```

### `cancellationownership` — call derived cancel functions

Flagged:

```go
func work(parent context.Context) {
	ctx, _ := context.WithCancel(parent)
	doWork(ctx)
}
```

Preferred:

```go
func work(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	doWork(ctx)
}
```

## License

Licensed under either the Apache License, Version 2.0 or the MIT License, at
your option.
