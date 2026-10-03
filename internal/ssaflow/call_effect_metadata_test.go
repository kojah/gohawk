package ssaflow

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCallEffectMetadataAllowance(t *testing.T) {
	var parameters, arguments, captures, reads strings.Builder
	for index := range 40 {
		fmt.Fprintf(&parameters, ", unused%d int", index)
		arguments.WriteString(",0")
		fmt.Fprintf(&captures, ", other%d *box", index)
		fmt.Fprintf(&reads, "_=other%d.flag;", index)
	}
	source := `package metadata
 type box struct{flag bool}
 func helper(value *box` + parameters.String() + `){}
 func called(value *box){helper(value` + arguments.String() + `)}
 func mutate(first *box` + parameters.String() + `,last *box){last.flag=true}
 func mutation(value *box){mutate(value` + arguments.String() + `,value)}
 func captured(value *box` + captures.String() + `){func(){_=value.flag;` + reads.String() + `}()}
 `
	pkg := ssaflowtest.BuildPackage(t, "metadata", source)
	call := InstructionsOf[*ssa.Call](pkg.Func("called"))[0]
	closure := InstructionsOf[*ssa.MakeClosure](pkg.Func("captured"))[0]
	for _, kind := range []string{"value", "field", "closure"} {
		t.Run(kind, func(t *testing.T) {
			query := NewCallEffects(NewSearchBudget(10))
			proof := metadataEffectProof(query, kind, call, closure)
			if proof.Proven() || proof.Reason != EvidenceBudgetExhausted || proof.PreservesStorage() || proof.PreservesField() {
				t.Fatalf("partial metadata proved purity: %+v", proof)
			}
			query.budget = NewSearchBudget(1000)
			fresh := metadataEffectProof(query, kind, call, closure)
			if !fresh.Proven() || !fresh.PreservesStorage() || !fresh.PreservesField() {
				t.Fatalf("fresh allowance did not recover purity: %+v", fresh)
			}
			if kind == "closure" && (fresh.Effects != EffectRead || proof.Effects != EffectRead) {
				t.Fatalf("capture read changed: %+v", fresh)
			}
		})
	}
	mutation := InstructionsOf[*ssa.Call](pkg.Func("mutation"))[0]
	checkLateMetadataMutation(t, mutation)
}

func metadataEffectProof(query *CallEffects, kind string, call *ssa.Call, closure *ssa.MakeClosure) CallEffectProof {
	switch kind {
	case "value":
		return query.Call(call, call.Common().Args[0])
	case "field":
		return query.FieldCall(call, EmbeddedFieldPath{Root: call.Common().Args[0], Depth: 1})
	case "closure":
		return query.proof(query.closure(closure, closure.Bindings[0]))
	}
	return CallEffectProof{}
}

func checkLateMetadataMutation(t *testing.T, call *ssa.Call) {
	t.Helper()
	for _, kind := range []string{"value", "field"} {
		query := NewCallEffects(NewSearchBudget(10))
		cut := metadataEffectProof(query, kind, call, nil)
		if cut.Proven() || cut.Reason != EvidenceBudgetExhausted {
			t.Fatalf("%s metadata prefix became complete: %+v", kind, cut)
		}
		query.budget = NewSearchBudget(1000)
		fresh := metadataEffectProof(query, kind, call, nil)
		if !fresh.Proven() || fresh.Effects&EffectMutate == 0 || fresh.PreservesStorage() || fresh.PreservesField() {
			t.Fatalf("%s late matching mutation lost after cutoff: %+v", kind, fresh)
		}
		query.budget = NewSearchBudget(10).Within(1000)
		pool := metadataEffectProof(query, kind, call, nil)
		if pool.Proven() || pool.Reason != EvidenceBudgetExhausted || !query.budget.PoolExhausted() {
			t.Fatalf("%s pool cutoff became complete: %+v", kind, pool)
		}
	}
}
