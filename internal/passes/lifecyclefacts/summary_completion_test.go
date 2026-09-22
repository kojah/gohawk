package lifecyclefacts

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestNonReturningMethodsDoNotInventCleanupContracts(t *testing.T) {
	contracts := contractsFor(t, `
package lifecyclefactstest
import "os"
type Pending struct { file *os.File }
func NewPending(path string) (*Pending, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	return &Pending{file:f}, nil
}
func (p *Pending) Configure() { panic("not implemented") }
type Real struct { file *os.File }
func NewReal(path string) (*Real, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	return &Real{file:f}, nil
}
func (r *Real) Stop() { r.file.Close() }
func (r *Real) Configure() { panic("not implemented") }
`)
	if contract := contracts["Pending"]; contract != nil {
		t.Errorf("panic-only method invented cleanup contract: %v", contract)
	}
	contract := contracts["Real"]
	if contract == nil || len(contract.Methods) != 1 || contract.Methods[0] != "Stop" {
		t.Errorf("real cleanup contract = %v, want only Stop", contract)
	}
}

func TestLifecycleMasksRequireCompletionWitness(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest
type closer interface { Close() error }
type holder struct { value closer }
func panicOnly(c closer) { panic("not implemented") }
func loopOnly(c closer) { for {} }
func panicUnlessNil(c closer) { if c == nil { return }; panic("not implemented") }
func panicOwner(c closer) *holder { panic("not implemented") }
func (h *holder) PanicStore(c closer) { panic("not implemented") }
func closeNormally(c closer) { c.Close() }
`)
	pass := &analysis.Pass{Pkg: pkg.Pkg, ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for _, name := range []string{"panicOnly", "loopOnly", "panicUnlessNil", "panicOwner"} {
		fact := summarize(pass, newRetentionCache(), pkg.Func(name))
		for _, mask := range lifecycleMasks {
			if got := *mask.field(&fact); got != 0 {
				t.Errorf("%s invented %s mask %#x", name, mask.name, got)
			}
		}
		if fact.SynchronouslyInvoked != 0 {
			t.Errorf("%s invented synchronous invocation", name)
		}
	}
	method := pkg.Prog.LookupMethod(types.NewPointer(pkg.Type("holder").Type()), pkg.Pkg, "PanicStore")
	if fact := summarize(pass, newRetentionCache(), method); fact.ReceiverStore != 0 {
		t.Errorf("panic-only receiver method invented receiver-store mask %#x", fact.ReceiverStore)
	}
	fact := summarize(pass, newRetentionCache(), pkg.Func("closeNormally"))
	if !fact.Closed.contains(0) {
		t.Error("real Close witness did not produce Closed fact")
	}
}
