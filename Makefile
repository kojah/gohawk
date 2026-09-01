GO ?= go
PNPM ?= corepack pnpm
LYCHEE ?= lychee
GOLANGCI_LINT_VERSION ?= v2.13.2
GOLANGCI_LINT ?= $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOVULNCHECK_VERSION ?= v1.7.0
GOVULNCHECK ?= $(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

BUILD_DIRECTORY ?= $(CURDIR)/.build
GOHAWK_BINARY ?= $(BUILD_DIRECTORY)/gohawk
BENCHMARK_ARGS ?=
VERIFY_JOBS ?= 4

VERIFY_STATIC_TARGETS := mod-verify fmt-check generated-check vet lint dogfood
VERIFY_TARGETS := $(VERIFY_STATIC_TARGETS) test test-race
# GNU Make before 4.0, including the version shipped with macOS, does not
# support grouped parallel output. Parallel scheduling itself remains required.
VERIFY_OUTPUT_SYNC := $(if $(filter output-sync,$(.FEATURES)),--output-sync=target)
VERIFY_MAKE_ARGS := --no-print-directory $(VERIFY_OUTPUT_SYNC) --jobs=$(VERIFY_JOBS)

.DEFAULT_GOAL := help

.PHONY: help build fmt fmt-check generate generated-check mod-verify lint vuln test \
	test-exhaustive test-race vet coverage plugin-test dogfood skills-check verify-static verify ci benchmark site-install \
	precision-regression site-check site-build site-links site-links-external site-review

help:
	@printf '%s\n' \
		'Common targets:' \
		'  make fmt             Format tracked Go source files' \
		'  make generate        Regenerate analyzer documentation' \
		'  make lint            Run the standard golangci-lint suite' \
		'  make vuln            Check reachable dependencies for known vulnerabilities' \
		'  make test            Run the Go test suite' \
		'  make test-exhaustive Run the CI-only exhaustive CLI subprocess matrix' \
		'  make verify          Run the complete local verification suite in parallel' \
		'  make dogfood         Build and run gohawk on itself' \
		'  make skills-check    Check installed skills against their upstream repositories' \
		'  make plugin-test     Test the golangci-lint module plugin end to end' \
		'  make benchmark       Run pinned dogfooding benchmarks' \
		'  make precision-regression  Replay reviewed precision cohorts' \
		'  make site-check      Check the documentation website' \
		'  make site-build      Build the documentation website' \
		'  make site-links      Check internal links in the built website' \
		'  make site-review     Start the documentation review server'

build:
	@mkdir -p "$(BUILD_DIRECTORY)"
	$(GO) build -o "$(GOHAWK_BINARY)" .

fmt:
	$(GOLANGCI_LINT) fmt

fmt-check:
	$(GOLANGCI_LINT) fmt --diff

generate:
	$(GO) generate ./...

generated-check:
	$(GO) run ./tools/gendocs -check

mod-verify:
	$(GO) mod verify

test:
	$(GO) test ./...

test-exhaustive:
	$(GO) test -tags=exhaustive ./internal/cli -run '^TestCLIIntegrationExhaustive$$' -count=1

test-race:
	# Analyzer fixture packages are independent, so Go can exercise them in
	# parallel while the race detector covers every analyzer implementation.
	$(GO) test -race ./analyzers ./internal/analyzers/...

vet:
	$(GO) vet ./...

lint:
	$(GOLANGCI_LINT) run ./...

vuln:
	$(GOVULNCHECK) ./...

coverage:
	$(GO) test ./... -covermode=count \
		-coverpkg=./internal/syntax/...,./internal/catalog,./internal/check,./internal/flagvalue,./internal/trace,./analyzers,./internal/passes/...,./internal/analyzers/...,./internal/docexamples \
		-coverprofile=coverage.out
	$(GO) tool cover -func=coverage.out -o=coverage-summary.out

plugin-test:
	$(GO) test -tags=integration ./plugin/golangci \
		-run '^TestCustomGolangCILint$$' -count=1 -v

dogfood: build
	"$(GOHAWK_BINARY)" -enable-all ./...

skills-check:
	./scripts/check-skills-current.sh

verify-static:
	+$(MAKE) $(VERIFY_MAKE_ARGS) $(VERIFY_STATIC_TARGETS)

verify:
	+$(MAKE) $(VERIFY_MAKE_ARGS) $(VERIFY_TARGETS)

# The aggregate local CI target adds coverage. Hosted CI and release workflows
# run the custom golangci-lint plugin test as a separate gate.
ci:
	+$(MAKE) $(VERIFY_MAKE_ARGS) $(VERIFY_TARGETS) coverage

benchmark:
	./scripts/benchmark-dogfood.sh $(BENCHMARK_ARGS)

precision-regression:
	./scripts/precision-regression.py benchmarks/precision/round-2
	./scripts/precision-regression.py benchmarks/precision/round-3
	./scripts/precision-regression.py benchmarks/precision/round-4
	./scripts/precision-regression.py benchmarks/precision/round-5
	./scripts/precision-regression.py benchmarks/precision/round-6
	./scripts/precision-regression.py benchmarks/precision/round-7
	./scripts/precision-regression.py benchmarks/precision/round-8
	./scripts/precision-regression.py benchmarks/precision/round-9

site-install:
	$(PNPM) --dir site install --frozen-lockfile

site-check:
	$(PNPM) --dir site check

site-build:
	$(PNPM) --dir site build

site-links: site-build
	$(LYCHEE) --offline --include-fragments --index-files index.html \
		--exclude '#_top$$' --root-dir "$(CURDIR)/site/dist" \
		'site/dist/**/*.html'

site-links-external: site-build
	$(LYCHEE) --include-fragments --index-files index.html \
		--exclude '#_top$$' --exclude 'https://gohawk\.dev/404/$$' \
		--exclude-all-private --max-concurrency 12 --timeout 20 \
		--root-dir "$(CURDIR)/site/dist" 'site/dist/**/*.html'

site-review:
	$(PNPM) --dir site dev:review
