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
VERIFY_TIMINGS ?= 1

VERIFY_BASE_TARGETS := mod-verify fmt-check vet deadcode
# Local generation skips expensive analyzer example validation. CI checks both
# ordinary generated content and live analyzer examples against committed pages.
VERIFY_STATIC_TARGETS := $(strip $(VERIFY_BASE_TARGETS) lint dogfood $(if $(CI),generated-check))
# CI runs lint and dogfood in dedicated jobs; its fast job must not repeat them.
VERIFY_CI_FAST_TARGETS := $(VERIFY_BASE_TARGETS) generated-check
# The race detector stays out of the routine local gate: gohawk is almost
# entirely synchronous, and CI runs test-race as its own job. make ci keeps it.
VERIFY_TARGETS := $(VERIFY_STATIC_TARGETS) test
# GNU Make before 4.0, including the version shipped with macOS, does not
# support grouped parallel output. Parallel scheduling itself remains required.
VERIFY_OUTPUT_SYNC := $(if $(filter output-sync,$(.FEATURES)),--output-sync=target)
# Keep going past the first failing target so one run reports every failure;
# the exit status still reflects any failure.
VERIFY_MAKE_ARGS := --no-print-directory $(VERIFY_OUTPUT_SYNC) --jobs=$(VERIFY_JOBS) --keep-going
verify_targets = $(if $(filter 1,$(VERIFY_TIMINGS)),$(addprefix verify-timed-,$(1)),$(1))

.DEFAULT_GOAL := help

.PHONY: help build fmt fmt-check generate generate-examples generated-check mod-verify lint deadcode vuln test \
	test-exhaustive test-race vet coverage plugin-test dogfood skills-check verify-static verify ci benchmark site-install \
	precision-regression site-check site-build site-audit site-audit-production site-links site-links-external site-review generated-sync

help:
	@printf '%s\n' \
		'Common targets:' \
		'  make fmt             Format tracked Go source files' \
		'  make generate        Regenerate analyzer documentation without rerunning examples' \
		'  make generate-examples  Validate analyzer examples and regenerate their pages' \
		'  make lint            Run the golangci-lint suite and the dead-code gate' \
		'  make deadcode        Fail on internal functions unreachable from any entry point' \
		'  make vuln            Check reachable dependencies for known vulnerabilities' \
		'  make test            Run the Go test suite' \
		'  make test-exhaustive Run the CI-only exhaustive CLI subprocess matrix' \
		'  make verify          Run the complete local verification suite in parallel' \
		'                       Timings are shown by default; set VERIFY_TIMINGS=0 to hide them' \
		'  make dogfood         Build and run gohawk on itself' \
		'  make skills-check    Check installed skills against their upstream repositories' \
		'  make plugin-test     Test the golangci-lint module plugin end to end' \
		'  make benchmark       Run pinned dogfooding benchmarks' \
		'  make precision-regression  Replay reviewed precision cohorts' \
		'                            (scope with ANALYZER=, ROUND=, REPOSITORY=; STAMP=1 records provenance;' \
		'                             CONTINUE=1 replays every cohort)' \
		'  make site-check      Check the documentation website' \
		'  make site-build      Build the documentation website' \
		'  make site-audit      Audit every sitemap page with Lighthouse' \
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
	GOHAWK_DOC_TIMINGS=$(VERIFY_TIMINGS) $(GO) generate ./...

generate-examples:
	GOHAWK_DOC_TIMINGS=$(VERIFY_TIMINGS) $(GO) run ./tools/gendocs -examples

generated-check:
	GOHAWK_DOC_TIMINGS=$(VERIFY_TIMINGS) $(GO) run ./tools/gendocs -check -examples

# Timed wrappers preserve each target's exit status and outer output grouping.
verify-timed-%:
	+@started=$$(date +%s); $(MAKE) --no-print-directory $*; status=$$?; \
		elapsed=$$(($$(date +%s) - started)); \
		printf 'verify timing: %s %ss exit=%s\n' '$*' "$$elapsed" "$$status" >&2; exit "$$status"

mod-verify:
	$(GO) mod verify

test:
	$(GO) test ./...

test-exhaustive:
	$(GO) test -tags=exhaustive ./internal/cli -run '^TestCLIIntegrationExhaustive$$' -count=1

test-race:
	# Sibling analyzers share tracing, ordered effects, and immutable fact
	# encoding caches. Exercise those concurrency contracts in CI.
	$(GO) test -race ./internal/trace ./internal/passes/concurrencyfacts ./internal/factcodec

vet:
	$(GO) vet ./...

# deadcode is a sibling gate rather than a prerequisite, so a dead-code
# finding does not hide the lint findings of the same run.
lint:
	$(GOLANGCI_LINT) run ./...

# golangci-lint's unused check skips exported identifiers, so internal helpers
# that lose their last caller survive it. See scripts/check-deadcode.sh.
deadcode:
	DEADCODE="$(DEADCODE)" ./scripts/check-deadcode.sh

vuln:
	$(GOVULNCHECK) ./...

coverage:
	$(GO) test ./... -covermode=count \
		-coverpkg=./internal/syntax/...,./internal/ssaflow/...,./internal/lifecycle,./internal/heapmodel,./internal/syncmodel,./internal/catalog,./internal/check,./internal/flagvalue,./internal/trace,./internal/cli,./analyzers,./internal/passes/...,./internal/analyzers/...,./internal/docexamples,./plugin/golangci \
		-coverprofile=coverage.out
	$(GO) tool cover -func=coverage.out -o=coverage-summary.out

plugin-test:
	$(GO) test -tags=integration ./plugin/golangci \
		-run '^TestCustomGolangCILint$$' -count=1 -v

dogfood: build
	"$(GOHAWK_BINARY)" -enable-all ./...

skills-check:
	./scripts/check-skills-current.sh

# Regenerate derived documentation while retaining existing examples before
# the local gates fan out. Mechanical drift is repaired in place, ahead of
# parallel checks, so nothing reads a page while it is being rewritten.
# Hosted CI runs generated-check with live examples; a stale committed page
# must fail there because CI cannot commit the fix.
ifndef CI
ifeq ($(VERIFY_TIMINGS),1)
generated-sync: verify-timed-generate
else
generated-sync: generate
endif
else
generated-sync:
endif

verify-static: generated-sync
	+$(MAKE) $(VERIFY_MAKE_ARGS) $(call verify_targets,$(if $(CI),$(VERIFY_CI_FAST_TARGETS),$(VERIFY_STATIC_TARGETS)))

verify: generated-sync
	+$(MAKE) $(VERIFY_MAKE_ARGS) $(call verify_targets,$(VERIFY_TARGETS))

# The aggregate local CI target adds coverage. Hosted CI and release workflows
# run the custom golangci-lint plugin test as a separate gate.
ci: generated-sync
	+$(MAKE) $(VERIFY_MAKE_ARGS) $(call verify_targets,$(VERIFY_TARGETS) test-race coverage)

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

site-audit: site-build
	$(PNPM) --dir site lighthouse

site-audit-production:
	$(PNPM) --dir site lighthouse:production

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
