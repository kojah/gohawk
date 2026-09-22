package ssaflow

import (
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

// LibraryContract identifies a third-party semantic boundary whose behavior
// cannot be recovered from the caller's SSA alone. Keep these exceptions in a
// single registry so analyzers do not grow divergent package/name heuristics.
type LibraryContract uint8

const (
	ContractTestingCleanup LibraryContract = iota + 1
	// ContractTestifyErrorClaim is a testify assertion that the test fails
	// unless its argument is a non-nil value: Error or NotNil in either package.
	ContractTestifyErrorClaim
	// ContractTestifyNilClaim is a testify assertion that the test fails
	// unless its argument is nil.
	ContractTestifyNilClaim
	ContractTestifyNoError
	ContractTestifyFatalError
	ContractGoMockReturn
	ContractAfterFunc
	ContractDeferredCleanup
	ContractRuntimeGoexit
	ContractTestingTermination
	ContractProcessExit
)

// HasLibraryContract reports whether common exactly matches a registered API.
func HasLibraryContract(common *ssa.CallCommon, contract LibraryContract) bool {
	if common == nil {
		return false
	}
	switch contract {
	case ContractTestingCleanup:
		// testing.T and testing.B promote Cleanup from the shared common
		// implementation type, which is the declaration retained in SSA.
		return matchesAnySymbol(
			common,
			syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "testing", Receiver: "common", Name: "Cleanup"}),
			syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "testing", Receiver: "TB", Name: "Cleanup"}),
		)
	case ContractTestifyErrorClaim:
		return testifyAssertion(common, "Error") || testifyAssertion(common, "NotNil")
	case ContractTestifyNilClaim:
		return testifyAssertion(common, "Nil")
	case ContractTestifyNoError:
		return CallMatchesSymbol(common, syntax.PackageFunction("github.com/stretchr/testify/assert", "NoError"))
	case ContractTestifyFatalError:
		// require.NotNil applied to an error value is the same fatal claim as
		// require.Error; callers must check that the argument is the error.
		return matchesAnySymbol(
			common,
			syntax.PackageFunction("github.com/stretchr/testify/require", "Error"),
			syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "github.com/stretchr/testify/require", Receiver: "Assertions", Name: "Error"}),
			syntax.PackageFunction("github.com/stretchr/testify/require", "NotNil"),
			syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "github.com/stretchr/testify/require", Receiver: "Assertions", Name: "NotNil"}),
		)
	case ContractGoMockReturn:
		return matchesAnySymbol(
			common,
			syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "go.uber.org/mock/gomock", Receiver: "Call", Name: "Return"}),
			syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "github.com/golang/mock/gomock", Receiver: "Call", Name: "Return"}),
		)
	case ContractAfterFunc:
		return matchesAnySymbol(
			common,
			syntax.PackageFunction("time", "AfterFunc"),
			syntax.PackageFunction("context", "AfterFunc"),
		)
	case ContractDeferredCleanup:
		// Cleanup registrars are intentionally structural: local test frameworks
		// commonly expose the same interface contract as Ginkgo. This is not an
		// exact symbol identity and therefore remains explicit at this boundary.
		return CallName(common) == "DeferCleanup"
	case ContractRuntimeGoexit:
		return CallMatchesSymbol(common, syntax.PackageFunction("runtime", "Goexit"))
	case ContractProcessExit:
		return processExitContract(common)
	case ContractTestingTermination:
		if strictIsFailure(common) {
			return true
		}
		for _, receiver := range []string{"common", "TB"} {
			for _, name := range []string{"FailNow", "Fatal", "Fatalf", "Skip", "Skipf", "SkipNow"} {
				if CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "testing", Receiver: receiver, Name: name})) {
					return true
				}
			}
		}
		return testifyUnconditionalTermination(common)
	default:
		return false
	}
}

// matryer/is keeps its failure callback private. New installs FailNow, whereas
// NewRelaxed installs the returning Fail callback. Only exact strict factory
// provenance establishes termination; a parameter or mixed constructor does not.
// https://github.com/ConduitIO/conduit/blob/9946a19b9fff997675f78bbc5ff437e760d39f4f/pkg/lifecycle/stream/destination_acker_test.go#L30-L92
func strictIsFailure(common *ssa.CallCommon) bool {
	const packagePath = "github.com/matryer/is"
	if !CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: packagePath, Receiver: "I", Name: "Fail"})) {
		return false
	}
	return NewReachingWalk(TransparentChangeType).Every(CallReceiver(common), func(_ ReachingWalk, value ssa.Value) bool {
		constructor, ok := value.(*ssa.Call)
		return ok && matchesAnySymbol(constructor.Common(), syntax.PackageFunction(packagePath, "New"),
			syntax.PackageMethod(syntax.MethodSymbol{PackagePath: packagePath, Receiver: "I", Name: "New"}))
	})
}

// Unlike Error/NoError assertions, these APIs fail unconditionally. Keep
// assert.Fail separate: it records failure but allows the goroutine to return.
// Watchdog timeout arms can end here without returning an unsettled obligation:
// https://github.com/anyproto/any-sync/blob/cb940c50bc987066b998d05cca16c9c9715cf4a2/app/ocache/ocache_test.go#L901-L913
func testifyUnconditionalTermination(common *ssa.CallCommon) bool {
	if testifyAssertion(common, "FailNow") || testifyAssertion(common, "FailNowf") {
		return true
	}
	for _, name := range []string{"Fail", "Failf"} {
		if matchesAnySymbol(common,
			syntax.PackageFunction("github.com/stretchr/testify/require", name),
			syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "github.com/stretchr/testify/require", Receiver: "Assertions", Name: name})) {
			return true
		}
	}
	return false
}

func processExitContract(common *ssa.CallCommon) bool {
	if CallMatchesSymbol(common, syntax.PackageFunction("os", "Exit")) {
		return true
	}
	for _, name := range []string{"Fatal", "Fatalf", "Fatalln"} {
		if matchesAnySymbol(common, syntax.PackageFunction("log", name),
			syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "log", Receiver: "Logger", Name: name})) {
			return true
		}
	}
	return false
}

func testifyAssertion(common *ssa.CallCommon, name string) bool {
	for _, packagePath := range []string{"github.com/stretchr/testify/assert", "github.com/stretchr/testify/require"} {
		if matchesAnySymbol(
			common,
			syntax.PackageFunction(packagePath, name),
			syntax.PackageMethod(syntax.MethodSymbol{PackagePath: packagePath, Receiver: "Assertions", Name: name}),
		) {
			return true
		}
	}
	return false
}

func matchesAnySymbol(common *ssa.CallCommon, symbols ...syntax.Symbol) bool {
	for _, symbol := range symbols {
		if CallMatchesSymbol(common, symbol) {
			return true
		}
	}
	return false
}
