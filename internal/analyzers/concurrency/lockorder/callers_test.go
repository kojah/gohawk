package lockorder

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestLockCallerInventoryScopeAndUses(t *testing.T) {
	pkg := lockCallerPackage(t)
	functions := []*ssa.Function{nil, pkg.Func("caller"), pkg.Func("many")}
	inventory := collectLockCallers(pkg.Func("init"), functions, ssaflow.NewSearchBudget(callerSetBudget))
	var dump strings.Builder
	for _, name := range []string{"init", "caller"} {
		if _, err := pkg.Func(name).WriteTo(&dump); err != nil {
			t.Fatal(err)
		}
	}
	t.Log(dump.String())
	for _, test := range []struct {
		name    string
		calls   int
		escaped bool
	}{
		{"duringInit", 1, false},
		{"direct", 1, false},
		{"handed", 0, true},
		{"async", 0, true},
		{"deferred", 0, true},
		{"crowded", 32, true},
	} {
		entry := inventory[pkg.Func(test.name)]
		if len(entry.calls) != test.calls || entry.escaped != test.escaped {
			t.Errorf("%s conditional callers: %+v", test.name, entry)
		}
	}
	if _, included := inventory[pkg.Func("Exported")]; included {
		t.Fatal("exported function became a private caller contract")
	}
}

func TestLockCallerCutoffDiscardsPrefix(t *testing.T) {
	pkg := lockCallerPackage(t)
	functions := []*ssa.Function{pkg.Func("caller"), pkg.Func("many")}
	complete := collectLockCallers(pkg.Func("init"), functions, nil)
	finished := false
	for limit := range 1000 {
		pool := ssaflow.NewSearchBudget(callerSetBudget)
		child := pool.Within(limit)
		inventory := collectLockCallers(pkg.Func("init"), functions, child)
		if inventory != nil {
			if child.Exhausted() || len(inventory) != len(complete) {
				t.Fatalf("cutoff %d published incomplete conditional callers", limit)
			}
			finished = true
			break
		}
		if !child.Exhausted() || pool.Exhausted() {
			t.Fatalf("cutoff %d lost child allowance ownership", limit)
		}
		fresh := collectLockCallers(pkg.Func("init"), functions, pool.Within(callerSetBudget))
		if fresh == nil || len(fresh[pkg.Func("crowded")].calls) != 32 {
			t.Fatalf("cutoff %d contaminated fresh discovery", limit)
		}
	}
	if !finished {
		t.Fatal("package inventory never completed")
	}
}

func lockCallerPackage(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "lockcallers", `package lockcallers
func duringInit()int{return 0}
var value=duringInit()
func direct(){}
func handed(){}
func async(){}
func deferred(){}
func crowded(){}
func Exported(){}
func take(callback func()){}
func caller(dynamic func()){
 direct();take(handed);go async();defer deferred();Exported();dynamic();println(value)
}
func many(){`+strings.Repeat("crowded();", 33)+`}
`)
}
