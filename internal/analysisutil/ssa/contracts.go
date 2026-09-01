package ssautil

import (
	"github.com/kojah/gohawk/internal/analysisutil"

	"golang.org/x/tools/go/ssa"
)

// LibraryContract identifies a third-party semantic boundary whose behavior
// cannot be recovered from the caller's SSA alone. Keep these exceptions in a
// single registry so analyzers do not grow divergent package/name heuristics.
type LibraryContract uint8

const (
	ContractTestingCleanup LibraryContract = iota + 1
	ContractTestifyAssertion
	ContractTestifyFatalError
	ContractGoMockReturn
	ContractAfterFunc
	ContractDeferredCleanup
	ContractRuntimeGoexit
	ContractTestingTermination
)

// HasLibraryContract reports whether common exactly matches a registered API.
func HasLibraryContract(common *ssa.CallCommon, contract LibraryContract) bool {
	if common == nil {
		return false
	}
	switch contract {
	case ContractTestingCleanup:
		return matchesAnySymbol(
			common,
			// testing.T and testing.B promote Cleanup from the shared common
			// implementation type, which is the declaration retained in SSA.
			analysisutil.PackageMethod("testing", "common", "Cleanup"),
			analysisutil.PackageMethod("testing", "TB", "Cleanup"),
		)
	case ContractTestifyAssertion:
		return testifyAssertion(common, "Error") || testifyAssertion(common, "Nil")
	case ContractTestifyFatalError:
		return matchesAnySymbol(
			common,
			analysisutil.PackageFunction("github.com/stretchr/testify/require", "Error"),
			analysisutil.PackageMethod("github.com/stretchr/testify/require", "Assertions", "Error"),
		)
	case ContractGoMockReturn:
		return matchesAnySymbol(
			common,
			analysisutil.PackageMethod("go.uber.org/mock/gomock", "Call", "Return"),
			analysisutil.PackageMethod("github.com/golang/mock/gomock", "Call", "Return"),
		)
	case ContractAfterFunc:
		return matchesAnySymbol(
			common,
			analysisutil.PackageFunction("time", "AfterFunc"),
			analysisutil.PackageFunction("context", "AfterFunc"),
		)
	case ContractDeferredCleanup:
		// Cleanup registrars are intentionally structural: local test frameworks
		// commonly expose the same interface contract as Ginkgo. This is not an
		// exact symbol identity and therefore remains explicit at this boundary.
		return CallName(common) == "DeferCleanup"
	case ContractRuntimeGoexit:
		return CallMatchesSymbol(common, analysisutil.PackageFunction("runtime", "Goexit"))
	case ContractTestingTermination:
		for _, receiver := range []string{"common", "TB"} {
			for _, name := range []string{"FailNow", "Fatal", "Fatalf", "Skip", "Skipf", "SkipNow"} {
				if CallMatchesSymbol(common, analysisutil.PackageMethod("testing", receiver, name)) {
					return true
				}
			}
		}
		return false
	default:
		return false
	}
}

func testifyAssertion(common *ssa.CallCommon, name string) bool {
	for _, packagePath := range []string{"github.com/stretchr/testify/assert", "github.com/stretchr/testify/require"} {
		if matchesAnySymbol(
			common,
			analysisutil.PackageFunction(packagePath, name),
			analysisutil.PackageMethod(packagePath, "Assertions", name),
		) {
			return true
		}
	}
	return false
}

func matchesAnySymbol(common *ssa.CallCommon, symbols ...analysisutil.Symbol) bool {
	for _, symbol := range symbols {
		if CallMatchesSymbol(common, symbol) {
			return true
		}
	}
	return false
}
