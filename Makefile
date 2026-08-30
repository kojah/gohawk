GO ?= go
GOFMT ?= gofmt
PNPM ?= corepack pnpm

BUILD_DIRECTORY ?= $(CURDIR)/.build
GOHAWK_BINARY ?= $(BUILD_DIRECTORY)/gohawk
BENCHMARK_ARGS ?=

VERIFY_STATIC_TARGETS := mod-verify fmt-check generated-check vet dogfood
VERIFY_TARGETS := $(VERIFY_STATIC_TARGETS) plugin-test test-race

.DEFAULT_GOAL := help

.PHONY: help build fmt fmt-check generate generated-check mod-verify test \
	test-race vet coverage plugin-test dogfood verify-static verify ci benchmark site-install \
	site-check site-build site-review

help:
	@printf '%s\n' \
		'Common targets:' \
		'  make fmt             Format tracked Go source files' \
		'  make generate        Regenerate analyzer documentation' \
		'  make test            Run the Go test suite' \
		'  make verify          Run the complete local verification suite' \
		'  make dogfood         Build and run gohawk on itself' \
		'  make plugin-test     Test the golangci-lint module plugin end to end' \
		'  make benchmark       Run pinned dogfooding benchmarks' \
		'  make site-check      Check the documentation website' \
		'  make site-build      Build the documentation website' \
		'  make site-review     Start the documentation review server'

build:
	@mkdir -p "$(BUILD_DIRECTORY)"
	$(GO) build -o "$(GOHAWK_BINARY)" .

fmt:
	$(GOFMT) -w $$(git ls-files '*.go')

fmt-check:
	@test -z "$$($(GOFMT) -l .)"

generate:
	$(GO) generate ./...

generated-check:
	$(GO) run ./internal/cmd/gendocs -check

mod-verify:
	$(GO) mod verify

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

coverage:
	$(GO) test ./... -covermode=count \
		-coverpkg=./analysisutil/...,./analyzers,./internal/analyzerbase,./internal/analyzers/...,./internal/docexamples \
		-coverprofile=coverage.out
	$(GO) tool cover -func=coverage.out -o=coverage-summary.out

plugin-test:
	$(GO) test -tags=integration ./plugin/golangci \
		-run '^TestCustomGolangCILint$$' -count=1 -v

dogfood: build
	"$(GOHAWK_BINARY)" ./...

verify-static: $(VERIFY_STATIC_TARGETS)

# The race suite executes the complete Go test suite, so verify does not repeat
# the same tests in a second non-race invocation. Use make test for the faster
# ordinary development loop.
verify: $(VERIFY_TARGETS)

# The aggregate CI target adds coverage to the release verification targets.
# GitHub Actions invokes these targets in separate parallel jobs.
ci: $(VERIFY_TARGETS) coverage

benchmark:
	./scripts/benchmark-dogfood.sh $(BENCHMARK_ARGS)

site-install:
	$(PNPM) --dir site install --frozen-lockfile

site-check:
	$(PNPM) --dir site check

site-build:
	$(PNPM) --dir site build

site-review:
	$(PNPM) --dir site dev:review
