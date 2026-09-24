package resultfacts

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

const storageResultsFixture = `package storedresults
type box struct { flag bool; err error; pointer *int }
type failure struct{}
func (*failure) Error() string { return "failure" }
var global box
func opaque(*box)
func literal() bool { return true }
func stable() bool { b := &box{flag: true}; return b.flag }
func typedNil() error { b := &box{err: (*failure)(nil)}; return b.err }
func nilError() error { b := &box{err: nil}; return b.err }
func freshPointer() *int { b := &box{pointer: new(int)}; return b.pointer }
func snapshot() bool { b := &box{flag: true}; saved := b.flag; b.flag = false; return saved }
func overwritten() bool { b := &box{flag: true}; b.flag = false; return b.flag }
func mixed(pick bool) bool { b := &box{flag: true}; if pick { b.flag = false }; return b.flag }
func escaped() bool { b := &box{flag: true}; opaque(b); return b.flag }
func deferred() (flag bool) { flag = true; defer func(){ flag = false }(); return }
func globalLoad() bool { return global.flag }
func forwarded() bool { return stable() }
func fieldAlias() bool { b := &box{flag: true}; alias := &b.flag; *alias = false; return b.flag }
func conditional(pick bool) bool { if pick { return stable() }; return false }
func mixedInterface(pick bool) error { b := &box{err: (*failure)(nil)}; if pick { b.err = nil }; return b.err }
func async() bool { b := &box{flag: true}; go func(){ b.flag = false }(); return b.flag }
func preservedSnapshot() error { b := &box{err: (*failure)(nil)}; saved := b.err; b.err = nil; return saved }
`

func TestStoredResultGuarantees(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "storedresults", storageResultsFixture)
	for name, want := range map[string]Guarantee{
		"stable": AlwaysTrue, "typedNil": AlwaysNonNil, "nilError": AlwaysNil,
		"freshPointer": AlwaysNonNil, "snapshot": AlwaysTrue, "overwritten": AlwaysFalse,
		"mixed": Unknown, "escaped": Unknown, "deferred": Unknown, "globalLoad": Unknown,
		"forwarded": AlwaysTrue, "fieldAlias": AlwaysFalse, "conditional": Unknown,
		"mixedInterface": Unknown, "async": Unknown, "preservedSnapshot": AlwaysNonNil,
	} {
		t.Run(name, func(t *testing.T) {
			got := NewEngine().Function(pkg.Func(name), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
			if got.Result(0) != want {
				var ir strings.Builder
				if _, err := pkg.Func(name).WriteTo(&ir); err != nil {
					t.Fatal(err)
				}
				t.Log(ir.String())
				t.Errorf("%s: got %v, want %v", name, got.Result(0), want)
			}
		})
	}
}

func TestStoredResultBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "storedresults", storageResultsFixture)
	engine := NewEngine()
	if got := engine.Function(pkg.Func("stable"), ssaflow.NewSearchBudget(3)); got.Result(0) != Unknown {
		t.Fatal("exhausted query established a result guarantee")
	}
	if got := engine.Function(pkg.Func("stable"), ssaflow.NewSearchBudget(ssaflow.SummaryBudget)); got.Result(0) != AlwaysTrue {
		t.Fatal("exhausted query poisoned fresh inference")
	}
}

func BenchmarkStoredResultSummary(b *testing.B) {
	pkg := ssaflowtest.BuildPackage(b, "storedresults", storageResultsFixture)
	for _, name := range []string{"literal", "stable", "escaped", "mixed"} {
		b.Run(name, func(b *testing.B) {
			function := pkg.Func(name)
			b.ReportAllocs()
			for b.Loop() {
				NewEngine().Function(function, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
			}
		})
	}
}
