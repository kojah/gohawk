package lockorder

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestExclusiveParameterRequiresCompleteCallerSet(t *testing.T) {
	for _, test := range []struct {
		name, extra string
		want        bool
	}{
		{"freshDirect", "", true},
		{"sharedDirect", "func other(){target(shared)}", false},
		{"sharedGo", "func other(){go target(shared)}", false},
		{"sharedDefer", "func other(){defer target(shared)}", false},
		{"callback", "var callback=target", false},
		{"initialization", "var initialized=initialize();func initialize()int{target(shared);return 0}", false},
		{"callerLimit", "func many(){" + strings.Repeat("target(&box{});", 31) + "}", true},
		{"tooManyCallers", "func many(){" + strings.Repeat("target(&box{});", 32) + "}", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "exclusivecallers", `package exclusivecallers
type box struct{value int}
var shared=&box{}
var published *box
func target(value *box){value.value++;published=value}
func direct(){target(&box{})}
`+test.extra)
			inventory := collectLockCallers(pkg.Func("init"), ssaflow.DeclaredFunctions(pkg), nil)
			callers := newExclusiveCallers(nil, inventory)
			var dump strings.Builder
			for _, fn := range []*ssa.Function{pkg.Func("init"), pkg.Func("direct"), pkg.Func("other")} {
				if fn != nil {
					if _, err := fn.WriteTo(&dump); err != nil {
						t.Fatal(err)
					}
				}
			}
			t.Log(dump.String())
			if got := callers.parameterExclusive(pkg.Func("target"), 0, nil).state == ssaflow.EvidenceProven; got != test.want {
				t.Fatalf("parameter exclusivity = %v, want %v", got, test.want)
			}
			cutoff := collectLockCallers(pkg.Func("init"), ssaflow.DeclaredFunctions(pkg), ssaflow.NewSearchBudget(0))
			if newExclusiveCallers(nil, cutoff).parameterExclusive(pkg.Func("target"), 0, nil).state == ssaflow.EvidenceProven {
				t.Fatal("incomplete caller inventory proved parameter exclusivity")
			}
		})
	}
}

func TestExclusiveMethodCallerSetRemainsUnknown(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "exclusivemethod", `package exclusivemethod
type box struct{value int}
type receiver struct{}
type worker interface{run(*box)}
var dynamic worker=receiver{}
func (receiver)run(value *box){value.value++}
func direct(){receiver{}.run(&box{})}
func other(value *box){dynamic.run(value)}
`)
	functions := ssaflow.DeclaredFunctions(pkg)
	inventory := collectLockCallers(pkg.Func("init"), functions, nil)
	for _, function := range functions {
		if function.Name() == "run" {
			if newExclusiveCallers(nil, inventory).parameterExclusive(function, 1, nil).state == ssaflow.EvidenceProven {
				t.Fatal("fresh method call hid an unmodeled interface caller")
			}
			return
		}
	}
	t.Fatal("missing method SSA")
}
