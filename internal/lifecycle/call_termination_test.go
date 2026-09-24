package lifecycle

import (
	"path"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"

	"golang.org/x/tools/go/ssa"
)

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
					if !ssaflow.InstructionTerminatesControlFlow(instruction) {
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
				if got := ssaflow.InstructionTerminatesControlFlow(call); got != want {
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
				if ssaflow.CallName(call.Common()) == "Fail" && ssaflow.InstructionTerminatesControlFlow(call) != (name == "strict") {
					t.Fatalf("wrong termination contract for %s", call)
				}
			}
		})
	}
}
