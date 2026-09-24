package ssainfer

import (
	"fmt"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

func TestCallMatchesSymbolUsesReceiverIdentity(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

type command struct{}

func (*command) Wait() error { return nil }

func calls(t *testing.T, cmd *exec.Cmd, local *command) {
	_ = strings.Contains("value", "v")
	_ = cmd.Wait()
	_ = local.Wait()
	t.Cleanup(func() {})
	t.Fatal("stop")
	time.AfterFunc(0, func() {})
	runtime.Goexit()
	_ = len([]int{})
}
`)
	calls := functionCalls(pkg.Func("calls"))

	assertSingleCallMatch(t, calls, syntax.PackageFunction("strings", "Contains"))
	assertSingleCallMatch(t, calls, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os/exec", Receiver: "Cmd", Name: "Wait"}))
	assertSingleCallMatch(t, calls, syntax.PackageMethod(syntax.MethodSymbol{
		PackagePath: "example.com/ssaflowtest",
		Receiver:    "command",
		Name:        "Wait",
	}))
	assertSingleCallMatch(t, calls, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "testing", Receiver: "common", Name: "Cleanup"}))
	assertSingleCallMatch(t, calls, syntax.PackageFunction("time", "AfterFunc"))
	assertSingleCallMatch(t, calls, syntax.PackageFunction("runtime", "Goexit"))
	assertSingleCallMatch(t, calls, syntax.Builtin("len"))

	var cleanup, fatal, goexit bool
	for _, call := range calls {
		cleanup = cleanup || ssaflow.HasLibraryContract(call.Common(), ssaflow.ContractTestingCleanup)
		fatal = fatal || ssaflow.HasLibraryContract(call.Common(), ssaflow.ContractTestingTermination)
		goexit = goexit || ssaflow.HasLibraryContract(call.Common(), ssaflow.ContractRuntimeGoexit)
	}
	if !cleanup || !fatal || !goexit {
		t.Fatalf("library contracts = cleanup:%t fatal:%t goexit:%t, want all true", cleanup, fatal, goexit)
	}
}

func functionCalls(function *ssa.Function) []*ssa.Call {
	return ssaflow.InstructionsOf[*ssa.Call](function)
}

func assertSingleCallMatch(t *testing.T, calls []*ssa.Call, symbol syntax.Symbol) {
	t.Helper()
	var got int
	for _, call := range calls {
		if ssaflow.CallMatchesSymbol(call.Common(), symbol) {
			got++
		}
	}
	if got != 1 {
		var identities []string
		for _, call := range calls {
			var receiver any
			if value := ssaflow.CallReceiver(call.Common()); value != nil {
				receiver = value.Type()
			}
			identities = append(identities, fmt.Sprintf("%s receiver=%v", ssaflow.CallName(call.Common()), receiver))
		}
		t.Fatalf("CallMatchesSymbol() matched %d calls, want 1; calls: %v", got, identities)
	}
}
