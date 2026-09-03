GO ?= go
PNPM ?= corepack pnpm
LYCHEE ?= lychee
GOLANGCI_LINT_VERSION ?= v2.13.2
GOLANGCI_LINT ?= $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOVULNCHECK_VERSION ?= v1.7.0
GOVULNCHECK ?= $(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
DEADCODE_VERSION ?= v0.49.0
DEADCODE ?= $(GO) run golang.org/x/tools/cmd/deadcode@$(DEADCODE_VERSION)

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

.PHONY: help build fmt fmt-check generate generated-check mod-verify lint deadcode vuln test \
	test-exhaustive test-race vet coverage plugin-test dogfood skills-check verify-static verify ci benchmark site-install \
	precision-regression site-check site-build site-links site-links-external site-review generated-sync

help:
	@printf '%s\n' \
		'Common targets:' \
		'  make fmt             Format tracked Go source files' \
		'  make generate        Regenerate analyzer documentation' \
		'  make lint            Run the golangci-lint suite and the dead-code gate' \
		'  make deadcode        Fail on internal functions unreachable from any entry point' \
		'  make vuln            Check reachable dependencies for known vulnerabilities' \
		'  make test            Run the Go test suite' \
		'  make test-exhaustive Run the CI-only exhaustive CLI subprocess matrix' \
		'  make verify          Run the complete local verification suite in parallel' \
		'  make dogfood         Build and run gohawk on itself' \
		'  make skills-check    Check installed skills against their upstream repositories' \
		'  make plugin-test     Test the golangci-lint module plugin end to end' \
		'  make benchmark       Run pinned dogfooding benchmarks' \
		'  make precision-regression  Replay reviewed precision cohorts' \
		'                            (scope with ANALYZER=, ROUND=, REPOSITORY=; STAMP=1 records provenance;' \
		'                             CONTINUE=1 replays every cohort)' \
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

lint: deadcode
	$(GOLANGCI_LINT) run ./...

# golangci-lint's unused check skips exported identifiers, so internal helpers
# that lose their last caller survive it. See scripts/check-deadcode.sh.
deadcode:
	DEADCODE="$(DEADCODE)" ./scripts/check-deadcode.sh

vuln:
	$(GOVULNCHECK) ./...

coverage:
	$(GO) test ./... -covermode=count \
		-coverpkg=./internal/syntax/...,./internal/ssaflow,./internal/catalog,./internal/check,./internal/flagvalue,./internal/trace,./internal/cli,./analyzers,./internal/passes/...,./internal/analyzers/...,./internal/docexamples,./plugin/golangci \
		-coverprofile=coverage.out
	$(GO) tool cover -func=coverage.out -o=coverage-summary.out

plugin-test:
	$(GO) test -tags=integration ./plugin/golangci \
		-run '^TestCustomGolangCILint$$' -count=1 -v

dogfood: build
	"$(GOHAWK_BINARY)" -enable-all ./...

skills-check:
	./scripts/check-skills-current.sh

# Regenerate derived documentation before the local gates fan out, so
# mechanical drift is repaired in place instead of reported and then fixed by
# hand. It runs as a prerequisite, ahead of the parallel checks, so nothing
# reads a page while it is being rewritten. Hosted CI keeps the strict
# generated-check: it cannot commit a fix, and a stale committed page must
# fail there.
ifndef CI
generated-sync: generate
else
generated-sync:
endif

verify-static: generated-sync
	+$(MAKE) $(VERIFY_MAKE_ARGS) $(VERIFY_STATIC_TARGETS)

verify: generated-sync
	+$(MAKE) $(VERIFY_MAKE_ARGS) $(VERIFY_TARGETS)

# The aggregate local CI target adds coverage. Hosted CI and release workflows
# run the custom golangci-lint plugin test as a separate gate.
ci: generated-sync
	+$(MAKE) $(VERIFY_MAKE_ARGS) $(VERIFY_TARGETS) coverage

benchmark:
	./scripts/benchmark-dogfood.sh $(BENCHMARK_ARGS)

# Replay reviewed precision cohorts. Scope a run while iterating on one
# analyzer: ANALYZER=<name> replays only that analyzer's labels and skips the
# repositories that carry none, ROUND=<round-13> replays one cohort, and
# REPOSITORY=<owner/name> replays one repository. CHECKOUT_ROOT=<directory>
# reuses clones between runs and GOHAWK=<binary> skips rebuilding, which is
# what makes a scoped replay quick enough to run beside the unit tests.
# STAMP=1 records the running revision on every label that still holds, so a
# later failure reports when the label was last confirmed instead of leaving
# a drifted label indistinguishable from a fresh regression.
# CONTINUE=1 replays every cohort instead of stopping at the first failure,
# which is what an audit wants: one stale label in an early cohort otherwise
# hides the state of every cohort after it. The exit status still reports
# whether anything failed.
# REQUIRE_SCANNABLE=1 fails when a repository could not be analysed at all,
# which is reported but tolerated by default because the corpus already
# carries repositories that need a build step before they compile.
PRECISION_ROUNDS := $(if $(ROUND),benchmarks/precision/$(ROUND),$(wildcard benchmarks/precision/round-*))
PRECISION_SCOPE := $(foreach analyzer,$(ANALYZER),--analyzer $(analyzer)) \
	$(foreach repository,$(REPOSITORY),--only $(repository)) \
	$(if $(CHECKOUT_ROOT),--checkout-root $(CHECKOUT_ROOT)) \
	$(if $(GOHAWK),--gohawk $(GOHAWK)) \
	$(if $(STAMP),--stamp) \
	$(if $(REQUIRE_SCANNABLE),--require-scannable)

precision-regression:
	@failed=0; for cohort in $$(printf '%s\n' $(PRECISION_ROUNDS) | sort -V); do \
		./scripts/precision-regression.py "$$cohort" $(PRECISION_SCOPE) || failed=1; \
		if [ "$$failed" = 1 ] && [ -z "$(CONTINUE)" ]; then exit 1; fi; \
	done; exit $$failed

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
