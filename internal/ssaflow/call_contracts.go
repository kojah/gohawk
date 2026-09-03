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
	case ContractTestingTermination:
		for _, receiver := range []string{"common", "TB"} {
			for _, name := range []string{"FailNow", "Fatal", "Fatalf", "Skip", "Skipf", "SkipNow"} {
				if CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "testing", Receiver: receiver, Name: name})) {
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
