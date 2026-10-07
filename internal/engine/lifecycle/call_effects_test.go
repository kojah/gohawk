package lifecycle

import (
	"bytes"
	"fmt"
	"path"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"github.com/kojah/gohawk/internal/engine/syntax"
	"golang.org/x/tools/go/ssa"
)

func TestCallEffects(t *testing.T) {
	for _, test := range []struct {
		name  string
		body  string
		want  ssacall.CallEffect
		known bool
	}{
		{"unused", ``, 0, true},
		{"read", `_ = p.n`, ssacall.EffectRead, true},
		{"write", `p.n=1`, ssacall.EffectMutate, true},
		{"readThenRetain", `_ = p.n; saved=p`, ssacall.EffectRead | ssacall.EffectRetain, false},
		{"returnAddress", `return p`, ssacall.EffectRetain, true},
		{"nestedReader", `read(p)`, ssacall.EffectRead, true},
		{"nestedWriter", `write(p)`, ssacall.EffectMutate, true},
		{"nestedRetainer", `retain(p)`, ssacall.EffectRetain, false},
		{"asyncReader", `go read(p)`, ssacall.EffectRead | ssacall.EffectAsync, true},
		{"deferredReader", `defer read(p)`, ssacall.EffectRead, true},
		{"deferredWriter", `defer write(p)`, ssacall.EffectMutate, true},
		{"spilledClosureReader", `func(){ _=p.n }()`, ssacall.EffectRetain, false},
		{"spilledClosureWriter", `func(){ p.n=1 }()`, ssacall.EffectRetain, false},
		{"spilledRetainedClosure", `callback=func(){ _=p.n }`, ssacall.EffectRetain, false},
		{"spilledAsyncClosure", `go func(){ _=p.n }()`, ssacall.EffectRetain, false},
		{"opaque", `opaque(p)`, 0, false},
		{"dynamic", `dynamic(p)`, 0, false},
		{"recursive", `recurse(p)`, 0, false},
		{"fieldAddressRetained", `savedInt=&p.n`, ssacall.EffectRetain, false},
		{"scalarReturn", `_ = scalar(p)`, ssacall.EffectRead, true},
		{"branchEffects", `if pick { read(p) } else { write(p) }`, ssacall.EffectRead | ssacall.EffectMutate, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "effectprobe", `package effectprobe
type owner struct { n int }
var saved *owner
var savedInt *int
var callback func()
var dynamic func(*owner)
func opaque(*owner)
func read(p *owner) { _=p.n }
func write(p *owner) { p.n=1 }
func retain(p *owner) { saved=p }
func recurse(p *owner) { recurse(p) }
func scalar(p *owner) int { return p.n }
func probe(p *owner,pick bool) *owner { `+test.body+`;return nil }
`)
			fn := pkg.Func("probe")
			var dump bytes.Buffer
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			proof := ssacall.NewCallEffects(proofs.NewSearchBudget(1000)).Value(fn.Params[0])
			if proof.Proven() != test.known || proof.Effects != test.want {
				t.Fatalf("effects = %+v, want known=%t effects=%v", proof, test.known, test.want)
			}
			if proof.PreservesStorage() != (test.known && test.want & ^ssacall.EffectRead == 0) {
				t.Fatalf("unexpected preservation: %+v", proof)
			}
			exhausted := ssacall.NewCallEffects(proofs.NewSearchBudget(0)).Value(fn.Params[0])
			if exhausted.Proven() || exhausted.Reason != proofs.EvidenceBudgetExhausted {
				t.Fatalf("budget exhaustion: %+v", exhausted)
			}
		})
	}
}

func TestCallEffectArgumentAndCaptureMapping(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want ssacall.CallEffect
	}{
		{"read", `read(p)`, ssacall.EffectRead},
		{"bothArguments", `pair(p,p)`, ssacall.EffectRead | ssacall.EffectMutate},
		{"started", `go read(p)`, ssacall.EffectRead | ssacall.EffectAsync},
		{"deferred", `defer read(p)`, ssacall.EffectRead},
		{"captureCellRead", `func(){ _=p.n }()`, ssacall.EffectRead},
		{"captureCellWrite", `func(){ p=nil }()`, ssacall.EffectMutate},
		{"captureCellAsync", `go func(){ _=p.n }()`, ssacall.EffectRead | ssacall.EffectAsync},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "effectprobe", `package effectprobe
type owner struct { n int }
func read(p *owner) { _=p.n }
func pair(p,q *owner) { _=p.n; q.n=1 }
func probe(p *owner) { `+test.body+` }
`)
			fn := pkg.Func("probe")
			for _, block := range fn.Blocks {
				for _, instruction := range block.Instrs {
					common := ssaflow.InstructionCall(instruction)
					if common == nil {
						continue
					}
					var value ssa.Value = fn.Params[0]
					if closure, ok := common.Value.(*ssa.MakeClosure); ok {
						value = closure.Bindings[0]
					}
					proof := ssacall.NewCallEffects(nil).Call(instruction, value)
					if !proof.Proven() || proof.Effects != test.want {
						t.Fatalf("call effects = %+v, want %v", proof, test.want)
					}
					return
				}
			}
			t.Fatal("missing call")
		})
	}
}

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
		cleanup = cleanup || ssacall.HasLibraryContract(call.Common(), ssacall.ContractTestingCleanup)
		fatal = fatal || ssacall.HasLibraryContract(call.Common(), ssacall.ContractTestingTermination)
		goexit = goexit || ssacall.HasLibraryContract(call.Common(), ssacall.ContractRuntimeGoexit)
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
		if ssacall.CallMatchesSymbol(call.Common(), symbol) {
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

func TestInstructionTermination(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
import ("os"; "log"; "runtime")
func direct() { os.Exit(0) }
func fatal() { log.Fatal("stop") }
func logger(l *log.Logger) { l.Fatalf("stop") }
func deferred() { defer os.Exit(0); println("still running") }
func conditional(b bool) { if b { defer os.Exit(0) }; println("still running") }
func asynchronous() { go runtime.Goexit(); println("still running") }
func deferredGoexit() { defer runtime.Goexit(); println("still running") }
func indirect(exit func(int)) { exit(0) }
func Exit(int) {}
func misleading() { Exit(0) }
`)
	for _, test := range []struct {
		name             string
		calls, runDefers int
	}{
		{"direct", 1, 0},
		{"fatal", 1, 0},
		{"logger", 1, 0},
		{"deferred", 0, 1},
		{"conditional", 0, 0},
		{"asynchronous", 0, 0},
		{"deferredGoexit", 0, 1},
		{"indirect", 0, 0},
		{"misleading", 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls, runDefers := 0, 0
			for _, block := range pkg.Func(test.name).Blocks {
				for _, instruction := range block.Instrs {
					if !ssapath.InstructionTerminatesControlFlow(instruction) {
						continue
					}
					switch instruction.(type) {
					case *ssa.Call:
						calls++
					case *ssa.RunDefers:
						runDefers++
					default:
						t.Fatalf("registration or launch terminates caller: %s", instruction)
					}
				}
			}
			if calls != test.calls || runDefers != test.runDefers {
				t.Fatalf("termination sites = (%d calls, %d defers), want (%d, %d)", calls, runDefers, test.calls, test.runDefers)
			}
		})
	}
}

func TestUnconditionalTestifyTermination(t *testing.T) {
	const source = `
func Fail() {}
func FailNow() {}
func Error() {}
func sample() { Fail(); FailNow(); Error() }
`
	for _, test := range []struct {
		path string
		fail bool
	}{
		{"github.com/stretchr/testify/require", true},
		{"github.com/stretchr/testify/assert", false},
		{"example.com/require", false},
	} {
		t.Run(test.path, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, test.path, "package "+path.Base(test.path)+source)
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](pkg.Func("sample")) {
				want := test.fail && ssaflow.CallName(call.Common()) == "Fail" ||
					test.path != "example.com/require" && ssaflow.CallName(call.Common()) == "FailNow"
				if got := ssapath.InstructionTerminatesControlFlow(call); got != want {
					t.Errorf("%s: terminates=%t, want %t", call, got, want)
				}
			}
		})
	}
}

func TestStrictIsTermination(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "github.com/matryer/is", `package is
type I struct{}
func New() *I { return &I{} }
func NewRelaxed() *I { return &I{} }
func (*I) Fail() {}
func strict() { New().Fail() }
func relaxed() { NewRelaxed().Fail() }
func unknown(i *I) { i.Fail() }
func mixed(b bool) { i := New(); if b { i = NewRelaxed() }; i.Fail() }
`)
	for _, name := range []string{"strict", "relaxed", "unknown", "mixed"} {
		t.Run(name, func(t *testing.T) {
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](pkg.Func(name)) {
				if ssaflow.CallName(call.Common()) == "Fail" && ssapath.InstructionTerminatesControlFlow(call) != (name == "strict") {
					t.Fatalf("wrong termination contract for %s", call)
				}
			}
		})
	}
}

// A child budget stops at its own limit or when the pool runs out, whichever
// comes first, and reports which one it was.
func TestBudgetWithinPool(t *testing.T) {
	pool := proofs.NewSearchBudget(5)
	first := pool.Within(3)
	for range 3 {
		if !first.Spend() {
			t.Fatal("first child stopped before its own limit")
		}
	}
	if first.Spend() || first.PoolExhausted() {
		t.Fatal("first child should stop at its own limit, not the pool's")
	}
	second := pool.Within(10)
	for range 2 {
		if !second.Spend() {
			t.Fatal("pool still had two instructions")
		}
	}
	if second.Spend() || !second.PoolExhausted() || !pool.Exhausted() {
		t.Fatal("second child should stop because the pool ran out")
	}
	if !proofs.NewSearchBudget(1).Within(2).Spend() {
		t.Fatal("a plain budget spends")
	}
	var none *proofs.SearchBudget
	if !none.Within(1).Spend() {
		t.Fatal("a nil pool yields a plain budget")
	}
}
