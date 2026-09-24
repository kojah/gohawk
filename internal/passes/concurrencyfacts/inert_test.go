package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

// Builtins, helper results, assertions, and panicking branches are passive
// only when every value involved is inert. Each admitted form has a
// neighbouring form that could carry a resource and stays unknown.
func TestInertCallsAssertionsAndPanics(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "inertcalls", `package inertcalls
type problem struct{ msg string }
func (p *problem) Error() string { return p.msg }
func fail(msg string) error { return &problem{msg} }
func channel(ch chan int) chan int { return ch }
func Helpers(ch chan int, msg string) error {
	err := fail(msg)
	close(ch)
	return err
}
func ChannelHelper(ch chan int) { close(channel(ch)) }
func Builtins(ch chan int, items []int) int {
	items = append(items, len(items), cap(items))
	close(ch)
	return max(len(items), 1)
}
func ChannelAppend(ch chan int, all []chan int) { all = append(all, ch); close(ch) }
func Assert(ch chan int, value any) string {
	close(ch)
	name, _ := value.(string)
	return name
}
func AssertChannel(value any) { close(value.(chan int)) }
func PanicBranch(ch chan int, bad bool) {
	if bad { panic("bad") }
	close(ch)
}
func AlwaysPanics(ch chan int) { close(ch); panic("always") }
func EveryBranchPanics(ch chan int, bad bool) {
	if bad { panic("bad") }
	panic("worse")
}
`)
	engine := NewEngine()
	for _, name := range []string{"Helpers", "Builtins", "Assert", "PanicBranch"} {
		got := engine.Root(pkg.Func(name), ssaflow.NewSearchBudget(2000))
		if got.Completeness() != CompleteWithEffects || len(got.Operations) != 1 || got.Operations[0].Kind != Close {
			t.Errorf("%s = %+v, want one close", name, got)
		}
	}
	if got := engine.Function(pkg.Func("fail"), ssaflow.NewSearchBudget(2000)); got.Completeness() != CompleteNoEffects {
		t.Errorf("helper returning an error = %+v, want complete", got)
	}
	for _, name := range []string{"ChannelHelper", "ChannelAppend", "AssertChannel", "AlwaysPanics", "EveryBranchPanics"} {
		if got := engine.Root(pkg.Func(name), ssaflow.NewSearchBudget(2000)); got.Complete() {
			t.Errorf("%s = %+v, want incomplete", name, got)
		}
	}
}
