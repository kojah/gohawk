package calls_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestProgramEntryInitializerReference(t *testing.T) {
	for _, test := range []struct {
		name, extra string
		want        bool
	}{
		{"entry", "", true},
		{"alias", "var entry=main", false},
		{"table", "var entries=[]func(){main}", false},
		{"ordinary", "var entry=helper;func helper(){}", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "main", `package main
   func acquire()int{return 1}
   func main(){println(acquire())}
   `+test.extra)
			var call *ssa.Call
			for _, candidate := range ssaflow.InstructionsOf[*ssa.Call](pkg.Func("main")) {
				if ssaflow.CallName(candidate.Common()) == "acquire" {
					call = candidate
				}
			}
			if call == nil {
				t.Fatal("missing entry acquisition")
			}
			if got := ssacall.RunsOnceInProgramEntry(call); got != test.want {
				t.Fatalf("entry guarantee = %v, want %v", got, test.want)
			}
		})
	}
}
