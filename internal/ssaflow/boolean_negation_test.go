package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const negationSource = `package negations
func direct(flag bool) bool { return flag }
func odd(flag bool) bool { a := !flag; return a }
func even(flag bool) bool { a := !flag; b := !a; return b }
func triple(flag bool) bool { a := !flag; b := !a; c := !b; return c }
func loaded(flag *bool) bool { a := *flag; return !a }
func compared(flag bool) bool { a := flag == true; return !a }
type named bool
func converted(flag bool) named { a := named(flag); return !a }
func merged(flag, other, choose bool) bool { a := flag; if choose { a = other }; return !a }
`

func TestBooleanNegationSource(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "negations", negationSource)
	for _, test := range []struct {
		name string
		odd  bool
	}{
		{"direct", false},
		{"odd", true},
		{"even", false},
		{"triple", true},
		{"loaded", true},
		{"compared", true},
		{"converted", true},
		{"merged", true},
	} {
		function := pkg.Func(test.name)
		returns := ssaflow.InstructionsOf[*ssa.Return](function)
		if len(returns) != 1 {
			t.Fatalf("%s: want one normal return", test.name)
		}
		source, odd := ssaflow.BooleanNegationSource(returns[0].Results[0])
		if odd != test.odd {
			t.Errorf("%s: parity %t, want %t", test.name, odd, test.odd)
		}
		switch test.name {
		case "direct", "odd", "even", "triple":
			if source != function.Params[0] {
				t.Errorf("%s: source %v, want exact parameter", test.name, source)
			}
		case "loaded":
			if _, ok := source.(*ssa.UnOp); !ok {
				t.Errorf("load was peeled: %v", source)
			}
		case "compared":
			if _, ok := source.(*ssa.BinOp); !ok {
				t.Errorf("comparison was peeled: %v", source)
			}
		case "converted":
			if _, ok := source.(*ssa.ChangeType); !ok {
				t.Errorf("conversion was peeled: %v", source)
			}
		case "merged":
			if _, ok := source.(*ssa.Phi); !ok {
				t.Errorf("merge was peeled: %v", source)
			}
		}
	}
	if source, odd := ssaflow.BooleanNegationSource(nil); source != nil || odd {
		t.Errorf("nil source: %v, %t", source, odd)
	}
}

// Bindings still decide only their exact source; the shared NOT traversal does
// not turn a conversion, mutable load, comparison or merge into that binding.
func TestNegatedFixedValuesKeepOpaqueSources(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "negations", negationSource)
	for _, name := range []string{"direct", "odd", "even", "triple", "loaded", "compared", "converted", "merged"} {
		function := pkg.Func(name)
		condition := ssaflow.InstructionsOf[*ssa.Return](function)[0].Results[0]
		fixed := ssaflow.FixedValues{function.Params[0]: ssaflow.OutcomeTrue}
		holds, decided := fixed.Holds(condition)
		wantDecided := name == "direct" || name == "odd" || name == "even" || name == "triple"
		wantHolds := name == "direct" || name == "even"
		if decided != wantDecided || decided && holds != wantHolds {
			t.Errorf("%s: got holds=%t decided=%t, want %t/%t", name, holds, decided, wantHolds, wantDecided)
		}
	}
}
