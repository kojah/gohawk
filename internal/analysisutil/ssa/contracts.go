package ssautil

import "golang.org/x/tools/go/ssa"

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
)

// HasLibraryContract reports whether common exactly matches a registered API.
func HasLibraryContract(common *ssa.CallCommon, contract LibraryContract) bool {
	if common == nil {
		return false
	}
	packagePath, name := CallPackage(common), CallName(common)
	switch contract {
	case ContractTestingCleanup:
		return packagePath == "testing" && name == "Cleanup"
	case ContractTestifyAssertion:
		return (packagePath == "github.com/stretchr/testify/assert" || packagePath == "github.com/stretchr/testify/require") && (name == "Error" || name == "Nil")
	case ContractTestifyFatalError:
		return packagePath == "github.com/stretchr/testify/require" && name == "Error"
	case ContractGoMockReturn:
		return (packagePath == "go.uber.org/mock/gomock" || packagePath == "github.com/golang/mock/gomock") && name == "Return"
	case ContractAfterFunc:
		return (packagePath == "time" || packagePath == "context") && name == "AfterFunc"
	case ContractDeferredCleanup:
		// Ginkgo exposes this package function across versioned module paths;
		// the exact API name is stable even when the import path moves.
		return name == "DeferCleanup"
	default:
		return false
	}
}
