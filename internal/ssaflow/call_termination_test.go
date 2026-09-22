package ssaflow

import (
	"testing"

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
					if !InstructionTerminatesControlFlow(instruction) {
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
