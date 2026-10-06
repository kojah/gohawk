package goroutineownership

import (
	"bytes"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCallbackTargetForms(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "callbackforms", `package callbackforms
 type Fn func()
 type Other func()
 var sink int
 func worker() {}
 func direct() Fn { return worker }
 func wrapped() Other { return Other(Fn(worker)) }
 func captured(value int) Other { return Other(Fn(func() { sink = value })) }
 func unknown(callback Fn) Fn { return callback }
 func asserted(value any) Fn { return value.(Fn) }
 func merged(first, second Fn, choose bool) Fn {
     result := first
     if choose { result = second }
     return result
 }
`)
	for _, name := range []string{"direct", "wrapped", "captured", "unknown", "asserted", "merged"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			var dump bytes.Buffer
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			value := ssaflow.InstructionsOf[*ssa.Return](function)[0].Results[0]
			got, closure := callbackTarget(value)
			switch name {
			case "direct", "wrapped":
				if got != pkg.Func("worker") || closure != nil {
					t.Fatal("static callback target changed")
				}
			case "captured":
				closures := ssaflow.InstructionsOf[*ssa.MakeClosure](function)
				if len(closures) != 1 || closure != closures[0] || got != closure.Fn {
					t.Fatal("callback capture binding was lost")
				}
			default:
				if got != nil || closure != nil {
					t.Fatal("opaque callback supplied an exact target")
				}
			}
		})
	}
}
