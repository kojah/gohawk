package lockorder

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestFreshBindingIdentityAndLoopBoundary(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "bindingidentity", `package bindingidentity
 import "sync"
 type owner struct{mu sync.Mutex}
 func take(value *owner){value.mu.Lock();value.mu.Unlock()}
 func plain(){take(new(owner))}
 func repeated(n int){for i:=0;i<n;i++{take(new(owner))}}
 `)
	lock := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("take"))[0]
	effect, known := directMutexEffectWithin(lock, nil)
	if !known {
		t.Fatal("missing helper lock")
	}
	for _, name := range []string{"plain", "repeated"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			var dump strings.Builder
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			call := ssaflow.InstructionsOf[*ssa.Call](function)[0]
			bound := bindLockAcquisition(effect.acquired, call)
			if bound.class != bound.instance || bound.widened {
				t.Fatalf("fresh identity/class disagree: %+v", bound)
			}
			if (bound.instance != "") != (name == "plain") {
				t.Fatalf("loop boundary: %+v", bound)
			}
		})
	}
}
