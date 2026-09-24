package heapmodel_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestStableFieldContentAcrossHelpers(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "fields", `package fields
type owner struct { data chan int; unrelated int; self *owner }
var saved *owner
func read(o *owner) { <-o.data }
func async(o *owner) { go read(o) }
func mutate(o *owner) { o.data = nil }
func retain(o *owner) { saved = o }
func rewrite(o *owner) { *o = owner{} }
func sibling(o *owner) { o.unrelated++ }
func recursive(o *owner) { recursive(o) }
func observe(o *owner) {}
func stable() { var o owner; o.data = make(chan int); async(&o); sibling(&o); observe(&o) }
func changed() { var o owner; o.data = make(chan int); observe(&o); mutate(&o) }
func replaced() { var o owner; o.data = make(chan int); observe(&o); rewrite(&o) }
func escaped() { var o owner; o.data = make(chan int); retain(&o); observe(&o) }
func unknown(f func(*owner)) { var o owner; o.data = make(chan int); f(&o); observe(&o) }
func cycle() { var o owner; o.data = make(chan int); recursive(&o); observe(&o) }
func selfAlias() { var o owner; o.data = make(chan int); o.self = &o; observe(&o); mutate(o.self) }
func directWrite() { var o owner; o.data = make(chan int); o.data = nil }
`)
	for _, name := range []string{"stable", "changed", "replaced", "escaped", "unknown", "cycle", "selfAlias"} {
		function := pkg.Func(name)
		var address ssa.Value
		for _, field := range ssaflow.InstructionsOf[*ssa.FieldAddr](function) {
			if field.Field == 0 {
				address = field
				break
			}
		}
		var observation ssa.Instruction
		for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
			if call.Common().StaticCallee() == pkg.Func("observe") {
				observation = call
			}
		}
		storage := heapmodel.NewStorage(ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		got := storage.StableFieldContent(address, observation)
		if got.Proven() != (name == "stable") {
			t.Errorf("%s: %+v", name, got)
		}
	}
	stores := ssaflow.InstructionsOf[*ssa.Store](pkg.Func("directWrite"))
	write := stores[len(stores)-1]
	storage := heapmodel.NewStorage(ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	if got := storage.StableFieldContent(write.Addr, write); got.Proven() {
		t.Errorf("observation's own write was ignored: %+v", got)
	}
}
