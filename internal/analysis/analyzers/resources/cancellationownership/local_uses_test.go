package cancellationownership

import (
	"bytes"
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestExactLocalUseForms(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "localuses", `package localuses
 type Cancel func()
 type Raw func()
 func direct(fn Cancel) Cancel { return fn }
 func converted(fn Cancel) Raw { return Raw(fn) }
 func loaded(cell *Cancel) Cancel { return *cell }
 func nested(cell **Cancel) Cancel { return **cell }
 func wrappedLoad(cell *Cancel) Raw { return Raw(*cell) }
 func merged(first, second Cancel, choose bool) Cancel {
     result := first
     if choose { result = second }
     return result
 }
 func unrelated(first, second Cancel) Cancel { return second }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"direct", true},
		{"converted", true},
		{"loaded", true},
		{"nested", true},
		{"wrappedLoad", true},
		{"merged", false},
		{"unrelated", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			var dump bytes.Buffer
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			returns := ssaflow.InstructionsOf[*ssa.Return](function)
			if len(returns) != 1 || len(returns[0].Results) != 1 {
				t.Fatal("expected one returned value")
			}
			value := returns[0].Results[0]
			if test.name == "merged" {
				if _, ok := value.(*ssa.Phi); !ok {
					t.Fatal("merged input did not lower to a phi")
				}
			}
			if got := exactLocalValueUse(value, function.Params[0]); got != test.want {
				t.Fatalf("exact local use = %v, want %v", got, test.want)
			}
		})
	}
}
