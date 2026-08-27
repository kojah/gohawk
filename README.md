# gohawk

[![CI](https://github.com/kojah/gohawk/actions/workflows/ci.yml/badge.svg)](https://github.com/kojah/gohawk/actions/workflows/ci.yml)

gohawk is a focused set of static analyzers for Go. It ships sixteen
framework-neutral checks covering API design, concurrency, resource ownership,
determinism, serialization, tests, and error handling.

## Contents

- [Quick Start](#quick-start)
- [Analyzers](#analyzers)
  - [API and data contracts](#api-and-data-contracts)
  - [Ownership and lifecycle](#ownership-and-lifecycle)
  - [Reliability and safety](#reliability-and-safety)
  - [Testing](#testing)
- [Examples](#examples)
  - [API and data contract examples](#api-and-data-contract-examples)
  - [Ownership and lifecycle examples](#ownership-and-lifecycle-examples)
  - [Reliability and safety examples](#reliability-and-safety-examples)
  - [Testing examples](#testing-examples)
- [Analyzer configuration](#analyzer-configuration)
- [Suppressions](#suppressions)
- [License](#license)

## Quick Start

```sh
# Install.
go install github.com/kojah/gohawk/cmd/gohawk@latest

# Run every check.
gohawk ./...

# Use it with go vet.
go vet -vettool="$(command -v gohawk)" ./...

# Preview safe fixes, then apply them.
gohawk -fix -diff ./...
gohawk -fix ./...

# Run selected checks, or exclude one from the defaults.
gohawk -wirepolicy -globalstate ./...
gohawk -globalstate=false ./...
```

## Analyzers

### API and data contracts

| Analyzer | What it catches |
| --- | --- |
| [`apishape`](#apishape) | Exported APIs with error-prone parameters or receiver shapes. |
| [`contextpolicy`](#contextpolicy) | Misplaced, stored, or nil contexts, plus test contexts with the wrong owner. |
| [`closedomain`](#closedomain) | Plain strings standing in for a closed set of values. |
| [`wirepolicy`](#wirepolicy) | Missing serialization tags and positional wire literals. |

### Ownership and lifecycle

| Analyzer | What it catches |
| --- | --- |
| [`cancellationownership`](#cancellationownership) | Context and signal cancellation functions that are never called. |
| [`channelpolicy`](#channelpolicy) | Unexplained channel capacity and broken closing ownership. |
| [`goroutineownership`](#goroutineownership) | Goroutines without a recognizable join handle or lifecycle owner. |
| [`processownership`](#processownership) | Started commands that are neither waited on nor transferred with wait ownership. |
| [`resourcelifetime`](#resourcelifetime) | Files, responses, SQL handles, timers, or compressors left open on some path. |

### Reliability and safety

| Analyzer | What it catches |
| --- | --- |
| [`determinism`](#determinism) | Unsorted map iteration that reaches ordered output. |
| [`errorownership`](#errorownership) | Errors handled twice or classified by matching their text. |
| [`globalstate`](#globalstate) | Mutable package-level state. |
| [`lockorder`](#lockorder) | Mutexes acquired in contradictory orders. |
| [`taintpolicy`](#taintpolicy) | Untrusted environment or argument data reaching sensitive sinks. |

### Testing

| Analyzer | What it catches |
| --- | --- |
| [`blockingtest`](#blockingtest) | Blocking test channels without cancellation ownership. |
| [`testpolicy`](#testpolicy) | Missing lifecycle ownership in test helpers. |

These checks are opinionated by design. When a finding is intentional, you can
[suppress it](#suppressions) with a short explanation.

## Examples

Here is what each check looks like in practice. gohawk reports every finding
and offers a fix when it can make the change safely. The examples show one
common finding and one way to fix it; some checks cover additional cases.

### API and data contract examples

#### `apishape`

Group error-prone parameters.

| Knob | Default | Effect |
| --- | --- | --- |
| `max-parameters` | `4` | Largest allowed parameter count on an exported function; `0` turns this off. |
| `max-adjacent-same-type` | `2` | Largest allowed run of adjacent parameters with one type; `0` turns this off. |

Flagged:

```go
func CreateUser(name, email, city, country, role string) error { return nil }
```

OK:

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
| `prefer-test-context` | `true` | Prefer `t.Context()` or `b.Context()` over `context.Background()` when the module's Go version supports it. |

Flagged:

```go
func LoadUser(id string, ctx context.Context) error { return nil }
```

OK:

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

OK:

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

OK:

```go
type EventRow struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

var event = EventRow{ID: "42", Kind: "created"}
```

### Ownership and lifecycle examples

#### `cancellationownership`

Call derived cancel functions.

Flagged:

```go
func work(parent context.Context) {
	ctx, _ := context.WithCancel(parent)
	doWork(ctx)
}
```

OK:

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
| `max-unexplained-capacity` | `1` | Largest constant capacity allowed without an explanatory comment; a negative value turns this off. |

Flagged:

```go
func consume(events chan Event) {
	defer close(events)
	for event := range events {
		handle(event)
	}
}
```

OK:

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
| `mode` | `context` | Choose `context` for context-controlled workers, `lifecycle` to require an owner, or `join` to require a completion signal or wait. |

By default, a context is enough to own a worker. Use
`-goroutineownership.mode=lifecycle` to require a lifecycle owner, or
`-goroutineownership.mode=join` to require an explicit join.

Flagged:

```go
func refresh() {
	go updateCache()
}
```

OK:

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

OK:

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
| `contracts` | `os,http,sql,time,compress` | Comma-separated resource families to check. |
| `require-reader-close` | `true` | Require gzip and zlib readers to be closed. |

The built-in contracts cover files, transactions, SQL rows and statements,
HTTP response bodies, timers and tickers, and gzip/zlib readers and writers.

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

OK:

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

### Reliability and safety examples

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

OK:

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

OK:

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
| `allow-names` | empty | Comma-separated package variables that may be mutable globals. |
| `allow-types` | empty | Comma-separated named types that may be mutable globals. Use full import paths. |

Type allowlists use the full import path:

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

OK:

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

OK:

```go
func forward() { first.Lock(); defer first.Unlock(); second.Lock(); defer second.Unlock() }
func reverse() { first.Lock(); defer first.Unlock(); second.Lock(); defer second.Unlock() }
```

#### `taintpolicy`

Validate untrusted input before a sink.

| Knob | Default | Effect |
| --- | --- | --- |
| `sinks` | `filesystem,process,terminal,log` | Comma-separated sink families to check. |
| `sanitizers` | empty | Extra sanitizer functions, comma-separated and fully qualified. |

Flagged:

```go
func runConfiguredTool() error {
	return exec.Command(os.Getenv("TOOL")).Run()
}
```

OK:

```go
func runConfiguredTool() error {
	tool, err := validateTool(os.Getenv("TOOL"))
	if err != nil {
		return err
	}
	return exec.Command(tool).Run()
}
```

### Testing examples

#### `blockingtest`

Make test waits cancellable.

Flagged:

```go
func waitForEvent(t *testing.T, events <-chan Event) Event {
	return <-events
}
```

OK:

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

OK:

```go
func requireUser(t *testing.T, user *User) {
	t.Helper()
	if user == nil {
		t.Fatal("expected a user")
	}
}
```

## Analyzer configuration

Analyzer options use standard `go/analysis` flags and work with both gohawk and
`go vet -vettool=...`. Prefix each option with its analyzer name:

```sh
gohawk -goroutineownership -goroutineownership.mode=join ./...
gohawk -contextpolicy -contextpolicy.prefer-test-context=false ./...
```

## Suppressions

Put `//gohawk:ignore <analyzer> [reason]` on the flagged line or the line above:

```go
//gohawk:ignore goroutineownership worker belongs to the process lifecycle
go serveMetrics()
```

Ignores are always scoped to one analyzer. The reason is optional.

## License

Licensed under either the Apache License, Version 2.0 or the MIT License, at
your option.
