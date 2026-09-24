package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestConstantHelperBranches(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
import "errors"
func nilError() error { return nil }
func tuple() (int, error) { return 42, nil }
func maybe(b bool) error { if b { return errors.New("failure") }; return nil }
func named() (err error) { defer func() { err = errors.New("failure") }(); return }
func yes() bool { return true }
func same(b bool) error { if b { println("one"); return nil }; println("two"); return nil }
func opaque() error
func recur() error { return recur() }
func direct() { if nilError() != nil { return }; println("reachable") }
func extracted() { _, err := tuple(); if err == nil { println("reachable") } }
func variable(b bool) { if maybe(b) == nil { println("unknown") } }
func deferred() { if named() == nil { println("unknown") } }
func indirect(fn func() error) { if fn() == nil { println("unknown") } }
func boolean() { if yes() { println("reachable") } }
func sameReturns(b bool) { if same(b) != nil { println("unreachable") } }
func unavailable() { if opaque() == nil { println("unknown") } }
func recursive() { if recur() == nil { println("unknown") } }
`)
	for _, test := range []struct {
		name string
		arm  int
	}{
		{"direct", 1},
		{"extracted", 0},
		{"variable", -1},
		{"deferred", -1},
		{"indirect", -1},
		{"boolean", 0},
		{"sameReturns", 1},
		{"unavailable", -1},
		{"recursive", -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, block := range pkg.Func(test.name).Blocks {
				if len(block.Instrs) == 0 {
					continue
				}
				if _, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If); !ok {
					continue
				}
				got := ssaflow.FeasibleSuccessors(block, nil)
				if test.arm < 0 {
					if len(got) != 2 {
						t.Fatalf("opaque result pruned successors: %v", got)
					}
				} else if len(got) != 1 || got[0] != block.Succs[test.arm] {
					t.Fatalf("successors = %v, want arm %d", got, test.arm)
				}
				return
			}
			t.Fatal("missing branch")
		})
	}
}
