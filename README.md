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

Analyzer-specific options preserve the default policies and also work through
`go vet -vettool=...`. Each configurable analyzer documents its knobs beside
its example below.

Boolean options accept explicit values when disabling a policy:

```sh
gohawk -contextpolicy \
  -contextpolicy.prefer-test-context=false \
  ./...
```

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
| [`apishape`](#apishape) | Flags exported APIs with error-prone parameter or receiver shapes. |
| [`contextpolicy`](#contextpolicy) | Checks context placement, storage, nil use, and test ownership. |
| [`closedomain`](#closedomain) | Finds builtin strings used as closed semantic domains. |
| [`wirepolicy`](#wirepolicy) | Checks serialized structs and their composite literals. |

### Ownership and lifecycle

| Analyzer | Policy |
| --- | --- |
| [`cancellationownership`](#cancellationownership) | Checks that context and signal-derived cancellation functions are called. |
| [`channelpolicy`](#channelpolicy) | Checks channel capacity and closing ownership. |
| [`goroutineownership`](#goroutineownership) | Requires explicit goroutines to have a join handle or lifecycle owner. |
| [`processownership`](#processownership) | Requires started commands to be waited on or transferred with their wait ownership. |
| [`resourcelifetime`](#resourcelifetime) | Checks files, HTTP responses, SQL handles, timers, and compressors are released on every path. |

### Reliability and safety

| Analyzer | Policy |
| --- | --- |
| [`determinism`](#determinism) | Detects unsorted map iteration reaching ordered output. |
| [`errorownership`](#errorownership) | Detects double-handled errors and error-text classification. |
| [`globalstate`](#globalstate) | Flags mutable package-level state. |
| [`lockorder`](#lockorder) | Detects contradictory mutex acquisition order. |
| [`taintpolicy`](#taintpolicy) | Checks untrusted environment and argument data reaching sensitive sinks. |

### Testing

| Analyzer | Policy |
| --- | --- |
| [`blockingtest`](#blockingtest) | Checks cancellation ownership for blocking test channels. |
| [`testpolicy`](#testpolicy) | Checks lifecycle ownership in test helpers. |

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

### API and data contracts

#### `apishape`

Group error-prone parameters.

| Knob | Default | Effect |
| --- | --- | --- |
| `max-parameters` | `4` | Maximum parameters on an exported function; `0` disables the check. |
| `max-adjacent-same-type` | `2` | Maximum adjacent parameters sharing one type; `0` disables the check. |
| `check-adjacent-optional-scalars` | `true` | Reports adjacent pointer-to-scalar parameters. |
| `check-mixed-receivers` | `true` | Reports types mixing pointer and value receivers. |

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

#### `contextpolicy`

Put context first.

| Knob | Default | Effect |
| --- | --- | --- |
| `require-first` | `true` | Requires `context.Context` to be the first parameter. |
| `forbid-storage` | `true` | Reports contexts stored in struct fields. |
| `prefer-test-context` | `true` | Prefers `t.Context()` or `b.Context()` over `context.Background()` in tests when supported by the module's Go version. |
| `forbid-nil` | `true` | Reports definitely nil context arguments. |

Flagged:

```go
func LoadUser(id string, ctx context.Context) error { return nil }
```

Preferred:

```go
func LoadUser(ctx context.Context, id string) error { return nil }
```

#### `closedomain`

Represent closed sets with named types.

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

#### `wirepolicy`

Tag fields and key literals.

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

### Ownership and lifecycle

#### `cancellationownership`

Call derived cancel functions.

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

#### `channelpolicy`

Let the creator close the channel.

| Knob | Default | Effect |
| --- | --- | --- |
| `max-unexplained-capacity` | `1` | Largest constant capacity allowed without a rationale; a negative value disables the check. |
| `check-borrowed-close` | `true` | Reports closing channels received from callers. |
| `check-send-after-close` | `true` | Reports sends proven to follow a close. |

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

#### `goroutineownership`

Join spawned goroutines.

| Knob | Default | Effect |
| --- | --- | --- |
| `accept-context-lifecycle` | `true` | Accepts a passed or captured context as lifecycle ownership. |
| `require-join` | `false` | Requires a completion signal or wait instead of context or lifecycle ownership. |

Context-controlled workers are accepted by default. Set
`-goroutineownership.accept-context-lifecycle=false` to disable only that
allowance, or use `-goroutineownership.require-join` for the strict policy.

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

#### `processownership`

Wait for started processes.

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

#### `resourcelifetime`

Release owned resources on every path.

| Knob | Default | Effect |
| --- | --- | --- |
| `contracts` | `os,http,sql,time,compress` | Comma-separated resource contract families to check. |
| `require-reader-close` | `true` | Requires gzip and zlib readers to be closed. |

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

### Reliability and safety

#### `determinism`

Sort map-derived output.

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

#### `errorownership`

Handle an error at one layer.

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

#### `globalstate`

Give mutable state an owner.

| Knob | Default | Effect |
| --- | --- | --- |
| `allow-names` | empty | Comma-separated package variable names allowed as mutable globals. |
| `allow-types` | empty | Comma-separated fully-qualified named types allowed as mutable globals. |

Fully-qualified type allowlists include the complete import path:

```sh
gohawk -globalstate \
  -globalstate.allow-names=metrics,registry \
  -globalstate.allow-types=example.com/project.Registry \
  ./...
```

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

#### `lockorder`

Acquire locks consistently.

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

#### `taintpolicy`

Validate untrusted input before a sink.

| Knob | Default | Effect |
| --- | --- | --- |
| `sinks` | `filesystem,process,terminal,log` | Comma-separated sink families to check. |
| `sanitizers` | empty | Additional comma-separated fully-qualified sanitizer functions. |

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

### Testing

#### `blockingtest`

Make test waits cancellable.

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

#### `testpolicy`

Mark test helpers.

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

## License

Licensed under either the Apache License, Version 2.0 or the MIT License, at
your option.
