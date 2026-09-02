---
title: How to contribute
description: How to add an analyzer to gohawk.
sidebar:
  order: 3
---

AI-assisted contributions are permitted when they follow the same quality
standards as every other contribution. See the [AI policy](../ai-policy/) for
the disclosure and review requirements.

## Want to add an analyzer?

Before getting started, consider a few questions:

1. Does the analyzer catch a correctness or reliability issue, or is it mainly
   enforcing an opinionated style preference?
2. Does a popular Go linter already provide the same check?
3. How much noise or how many false positives is it likely to create in real
   codebases?
4. If possible, can you link to real-world bugs the analyzer would have caught,
   either in your own code or in other Go projects? This evidence is optional,
   but it helps demonstrate the check's value and shape realistic test cases.

## Adding an analyzer

An analyzer should report a real problem that a developer can act on. gohawk
prefers missing an unusual bug over producing false alerts on valid code.
See [Architecture](../architecture/) for the package map and the relationship
between analyzer declarations, fixtures, and generated documentation.

Suppose the new analyzer is called `examplepolicy`.

### 1. Write the analyzer

Choose the matching catalog group, then create
`internal/analyzers/<group>/examplepolicy/analyzer.go`. The group directory is
only an organizational container; `examplepolicy` remains its own Go package.
Export an `Analyzer` constructor, then put the analysis in a separate run
function:

```go
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "examplepolicy",
		Doc:  "checks ...",
		Run:  runExamplePolicy,
	}
}

func runExamplePolicy(pass *analysis.Pass) (any, error) {
	// Inspect the package and report problems here.
	return nil, nil
}
```

Add any analysis passes you need to `Requires`. Diagnostics must point at the
smallest useful source range. Use `check.Reportf` when you have a position and
`check.Report` when you need to provide the range yourself.

Only add flags for choices that projects may reasonably disagree about, such
as a threshold or ownership policy. Define them on the analyzer's `Flags` set.

### 2. Register it

Add a `catalog.AnalyzerSpec` for the analyzer to the matching group in
`analyzers/catalog_specs.go`, including its checks, opt-in status, and
suggested-fix support. Then add its analyzer ID to the stable order in
`analyzers/analyzers.go`.

Define each stable check identity alongside the existing check constants in
`internal/check/check.go`.

Give every diagnostic rule a stable check identity in its `AnalyzerSpec`.
Report each diagnostic through `check.Report` or `check.Reportf` with that
check identity. Analyzers and checks run by
default unless their spec explicitly sets `OptIn: true`.

Then update `analyzers/analyzers_test.go`:

1. Add the name to `expectedAnalyzerNames`.
2. Add it to the expected group.

### 3. Add fixtures

Create
`internal/analyzers/<group>/examplepolicy/testdata/src/examplepolicy/examplepolicy.go`.
Add a package-local `analyzer_test.go` that runs it through the shared
`internal/analyzertest` harness. Put small examples that should be reported in
the fixture and mark each expected diagnostic with a `want` comment:

```go
func flagged() {
	problem() // want "the important part of the diagnostic"
}
```

Add examples that must stay quiet too. Cover realistic near misses and safe
forms, not just the easiest happy path. If dogfooding finds a false alert, turn
the smallest version of it into an accepted fixture.

If the analyzer has flags, suggested fixes, policy modes, or cross-package
behavior, put those packages beneath its fixture directory. Existing
`config/`, `fix/`, and mode directories show the convention.

### 4. Add the living documentation example

Add
`internal/analyzers/<group>/examplepolicy/testdata/src/examplepolicy/doc_examples.go`
with one or more flagged regions and exactly one OK region. Give multiple
flagged regions short titles that distinguish the behavior each snippet
demonstrates:

```go
//gohawk:example flagged Direct policy violation
func flaggedExample() {
	problem() // want "the important part of the diagnostic"
}
//gohawk:example end

//gohawk:example ok
func okExample() {
	safeAlternative()
}
//gohawk:example end
```

These snippets are compiled and analyzed as tests. The docs generator copies
them to the website and inserts each real diagnostic as a `// gohawk:` comment
immediately above the line it identifies. The diagnostic's precise source range
remains highlighted in the code, but the message is no longer hidden in a
tooltip.

### 5. Add the analyzer page

Create a Markdown page under the matching group in `docs/analyzers`. It only
needs frontmatter, a short explanation, and an Examples heading:

```md
---
title: examplepolicy
description: "A short description."
---

## Rule details

Explain what the rule catches and why it matters.

## Examples
```

Run `go generate ./...`. This fills in the examples, options table, analyzer
indexes, and website data. Edit the fixture when an example needs to change;
do not edit generated example blocks by hand.

### 6. Check the result

Format and test the repository:

```sh
make fmt
make generate
make verify
```

`make verify` runs its checks in parallel: it checks gofumpt formatting and
generated documentation, runs the standard and curated golangci-lint suite,
fails on internal functions that no entry point can reach, runs unit and race
tests, vets and builds the project, and dogfoods the resulting gohawk binary
with every analyzer and check enabled. The dead-code gate exists because
golangci-lint's `unused` check ignores exported identifiers, even under
`internal/`; delete unreachable code rather than adding it to
`.deadcode-baseline`, which only records findings that predate the gate. It
uses four concurrent jobs by default; set `VERIFY_JOBS` to tune that bound for
the machine. The custom golangci-lint plugin integration test remains a
dedicated CI and release gate and can be run locally with `make plugin-test`.
Run `make help` to see the
focused targets for individual checks, documentation development, and
dogfooding benchmarks.

Run the analyzer on a few real Go projects too. Investigate every new finding
and fix recurring false-positive patterns before enabling broader coverage.

Finally, refresh the coverage badge so the CI check agrees with the new tests:

```sh
make coverage
go run github.com/AlexBeauchemin/gobadge@v0.4.0 \
  -filename=coverage-summary.out \
  -target=README.md \
  -link=https://github.com/kojah/gohawk/actions/workflows/ci.yml
```
