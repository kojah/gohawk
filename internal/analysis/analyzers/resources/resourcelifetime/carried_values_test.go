package resourcelifetime

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCarriedValueAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "carried", `package carried
type resource struct { n int }
type holder struct { value *resource }
func direct(p, other *resource) any { return p }
func loaded(p, other *resource) any { h := &holder{p}; return h.value }
func nested(p, other *resource) any { return &holder{p} }
func unrelated(p, other *resource) any { return &holder{other} }
func derived(p, other *resource) any { return &struct{ value *int }{&p.n} }
func closure(p, other *resource) any { return func() { println(p) } }
`)
	for _, test := range []struct {
		name string
		want bool
	}{{"direct", true}, {"loaded", true}, {"nested", true}, {"unrelated", false}, {"derived", true}, {"closure", true}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			value := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
			baseline := aggregateEscapeAnalysis(fn).proveCarriedValueWithin(value, nil).Proven()
			if baseline != test.want {
				t.Fatalf("default carry = %v, want %v; SSA:\n%s", baseline, test.want, carriedSSA(t, fn))
			}
			completed := false
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				proof := aggregateEscapeAnalysis(fn).proveCarriedValueWithin(value, budget)
				if resourceFlowExhausted(budget) || limit == 0 {
					if proof.State != proofs.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted {
						t.Fatalf("allowance %d retained interrupted carry: %+v", limit, proof)
					}
					continue
				}
				if proof.State == proofs.EvidenceUnknown || proof.Proven() != baseline {
					t.Fatalf("complete carry = %+v, want %v", proof, baseline)
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("carry never completed")
			}
		})
	}
}

func TestCarriedStorageChildCutoff(t *testing.T) {
	var source strings.Builder
	source.WriteString("package storagecap\ntype resource struct{}\ntype a *resource\ntype b *resource\nfunc long(p, other a) a {\n")
	previous := "other"
	for index := range 1010 {
		typeName := "b"
		if index%2 == 1 {
			typeName = "a"
		}
		name := fmt.Sprintf("x%d", index)
		fmt.Fprintf(&source, "%s := %s(%s)\n", name, typeName, previous)
		previous = name
	}
	fmt.Fprintf(&source, "return %s\n}", previous)
	fn := ssaflowtest.BuildPackage(t, "storagecap", source.String()).Func("long")
	if count := len(ssaflow.InstructionsOf[*ssa.ChangeType](fn)); count != 1010 {
		t.Fatalf("conversion count = %d", count)
	}
	budget := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	value := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
	proof := aggregateEscapeAnalysis(fn).proveCarriedDirectlyWithin(value, budget)
	if proof.State != proofs.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted || resourceFlowExhausted(budget) {
		t.Fatalf("storage child cutoff with available caller = %+v, caller exhausted %v", proof, resourceFlowExhausted(budget))
	}
}

func TestNestedCarryIncludesDerivedStoredValues(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "derivedcarry", `package derivedcarry
type resource struct { n int }
func derived(p *resource) any { return &struct{ value *int }{&p.n} }
`)
	fn := pkg.Func("derived")
	value := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
	if contained := lifecycle.ProveMayContainValueWithin(value, fn.Params[0], nil); contained.Proven() {
		t.Fatalf("control already has object containment: %+v", contained)
	}
	proof := aggregateEscapeAnalysis(fn).proveNestedCarryWithin(value, proofs.NewSearchBudget(proofs.SummaryBudget))
	if !proof.Proven() || proof.Reason != resourceReasonAggregateMayCarry {
		t.Fatalf("derived stored value = %+v; SSA:\n%s", proof, carriedSSA(t, fn))
	}
}

func TestCarriedPayloadClassifierCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "payload", `package payload
type resource struct{}
type holder struct { value *resource }
func send(p, other *resource, ch chan any) { ch <- &holder{p} }
func unrelated(p, other *resource, ch chan any) { ch <- &holder{other} }
func put(p, other *resource, m map[int]any) { m[0] = p }
func choose(p, other *resource, ch chan any) { select { case ch <- p: default: } }
func dynamic(p, other *resource, f func(any)) { f(p) }
`)
	for _, test := range []struct {
		name   string
		reason resourceLifetimeReason
		want   bool
	}{
		{"send", resourceReasonSentToChannel, true},
		{"unrelated", resourceReasonSentToChannel, false},
		{"put", resourceReasonStoredInMap, true},
		{"choose", resourceReasonSentToChannel, true},
		{"dynamic", resourceReasonDynamicCallee, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			var payload ssa.Instruction
			for instruction := range ssaflow.InstructionsWithin(fn, nil) {
				switch instruction.(type) {
				case *ssa.Send, *ssa.MapUpdate, *ssa.Select, *ssa.Call:
					payload = instruction
				}
			}
			if payload == nil {
				t.Fatalf("no payload in SSA:\n%s", carriedSSA(t, fn))
			}
			query := aggregateEscapeAnalysis(fn)
			query.pool = proofs.NewSearchBudget(0)
			if reason, opaque := query.opaqueConsumption(payload); !opaque || reason != resourceReasonBudgetExhausted {
				t.Fatalf("interrupted payload = %v/%v", reason, opaque)
			}
			query.pool = proofs.NewSearchBudget(proofs.SummaryBudget)
			if reason, opaque := query.opaqueConsumption(payload); opaque != test.want || reason != test.reason {
				t.Fatalf("fresh payload = %v/%v, want %v/%v", reason, opaque, test.reason, test.want)
			}
		})
	}
}

func carriedSSA(t *testing.T, fn *ssa.Function) string {
	t.Helper()
	var output strings.Builder
	if _, err := fn.WriteTo(&output); err != nil {
		t.Fatalf("write SSA: %v", err)
	}
	return output.String()
}
