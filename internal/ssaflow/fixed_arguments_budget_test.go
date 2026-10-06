package ssaflow_test

import (
	"reflect"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const fixedBindingFixture = `package fixedbinding
 func effect(){}
 func flags(yes bool,p *int){if yes{effect()};if p==nil{effect()}}
 func untested(p *int){println(p)}
 func literal(){flags(true,nil)}
 func forwarded(yes bool){flags(yes,nil)}
 func typedNil(){var p *int;boxed(p)}
 func boxed(x interface{}){if x==nil{effect()}}
 func irrelevant(){untested(nil)}
 func captured(yes bool){defer func(){if yes{effect()}}()}
 func mutable(yes bool){defer func(){if yes{effect()}}();yes=!yes}
 func nested(yes bool){defer func(){defer func(){if yes{effect()}}()}()}
 func nestedWrite(yes bool){defer func(){func(){yes=false}()}()}
 func capturedNil(){var p *int;defer func(){if p==nil{effect()}}()}
 func storedNil(){p:=(*int)(nil);defer func(){if p==nil{effect()}}()}
`

func TestFixedArgumentBindingAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "fixedbinding", fixedBindingFixture)
	for _, test := range []struct {
		name       string
		count      int
		knownParam bool
		knownCell  bool
	}{
		{"literal", 2, false, false},
		{"forwarded", 1, false, false},
		{"forwarded", 2, true, false},
		{"typedNil", 1, false, false},
		{"irrelevant", 0, false, false},
		{"captured", 1, true, false},
		{"mutable", 0, true, false},
		{"mutable", 1, false, true},
		{"nested", 1, true, false},
		{"nestedWrite", 0, true, false},
		{"capturedNil", 0, false, false},
		{"storedNil", 1, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := fixedBindingCall(t, fn)
			callee, closure := ssaflow.DirectCallee(ssaflow.InstructionCall(call))
			known := ssaflow.FixedValues{}
			if test.knownParam {
				known[fn.Params[0]] = ssaflow.OutcomeFalse
			}
			if test.knownCell {
				known[closure.Bindings[0]] = ssaflow.OutcomeTrue
			}
			baseline := ssaflow.ProveFixedArgumentsWithin(ssaflow.InstructionCall(call), closure, callee, known, nil)

			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			if _, err := callee.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			if !baseline.Proven() || len(baseline.Values) != test.count {
				t.Fatalf("default=%+v want%d bindings", baseline, test.count)
			}
			checkFixedBindingCutoffs(t, func(budget *proofs.SearchBudget) ssaflow.FixedArgumentsProof {
				return ssaflow.ProveFixedArgumentsWithin(ssaflow.InstructionCall(call), closure, callee, known, budget)
			}, baseline)
		})
	}
}

func fixedBindingCall(t *testing.T, fn *ssa.Function) ssa.Instruction {
	t.Helper()
	for instruction := range ssaflow.InstructionsWithin(fn, nil) {
		switch instruction.(type) {
		case *ssa.Call, *ssa.Defer:
			return instruction
		}
	}
	t.Fatal("no binding call")
	return nil
}

func checkFixedBindingCutoffs(t *testing.T, prove func(*proofs.SearchBudget) ssaflow.FixedArgumentsProof, want ssaflow.FixedArgumentsProof) {
	t.Helper()
	completed := false
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		got := prove(budget)
		if budget.Exhausted() || limit == 0 {
			if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted || got.Values != nil {
				t.Fatalf("cut%d published%+v", limit, got)
			}
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("complete=%+v want%+v", got, want)
		}
		completed = true
		break
	}
	if !completed {
		t.Fatal("binding never completed")
	}
	pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	child := pool.Within(0)
	if got := prove(child); got.State != proofs.EvidenceUnknown || got.Values != nil || pool.Exhausted() {
		t.Fatalf("child=%+v", got)
	}
	if got := prove(pool.Within(proofs.SummaryBudget)); !reflect.DeepEqual(got, want) {
		t.Fatalf("fresh=%+v want%+v", got, want)
	}
}

func TestFixedArgumentPartialCensusDiscarded(t *testing.T) {
	source := `package partialbinding
 func effect(){}
 func slow(yes bool,p *int){` + strings.Repeat("println(p);", proofs.QueryBudget+1) + `if p==nil && yes{effect()}}
 func caller(){slow(true,nil)}
 `
	fn := ssaflowtest.BuildPackage(t, "partialbinding", source).Func("caller")
	call := fixedBindingCall(t, fn)
	callee, closure := ssaflow.DirectCallee(ssaflow.InstructionCall(call))
	pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	child := pool.Within(proofs.QueryBudget)
	proof := ssaflow.ProveFixedArgumentsWithin(ssaflow.InstructionCall(call), closure, callee, nil, child)
	if proof.State != proofs.EvidenceUnknown || proof.Values != nil || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("partial=%+v", proof)
	}
	fresh := ssaflow.ProveFixedArgumentsWithin(ssaflow.InstructionCall(call), closure, callee, nil, pool.Within(2*proofs.SummaryBudget))
	expectedOutcomes := fresh.Values[callee.Params[0]] == ssaflow.OutcomeTrue && fresh.Values[callee.Params[1]] == ssaflow.OutcomeNil
	if !fresh.Proven() || len(fresh.Values) != 2 || !expectedOutcomes {
		t.Fatalf("fresh=%+v", fresh)
	}
}

func TestFixedArgumentMetadataCensus(t *testing.T) {
	budget := proofs.NewSearchBudget(0)
	proof := ssaflow.ProveFixedArgumentsWithin(nil, nil, nil, nil, budget)
	if !proof.Proven() || proof.Values != nil || budget.Exhausted() || proof.Provenance != 0 {
		t.Fatalf("metadata=%+v", proof)
	}
}

func TestFixedNestedCaptureBindings(t *testing.T) {
	fn := ssaflowtest.BuildPackage(t, "fixedbinding", fixedBindingFixture).Func("nested")
	outer := fixedBindingCall(t, fn)
	body, closure := ssaflow.DirectCallee(ssaflow.InstructionCall(outer))
	known := ssaflow.FixedValues{fn.Params[0]: ssaflow.OutcomeFalse}
	first := ssaflow.ProveFixedArgumentsWithin(ssaflow.InstructionCall(outer), closure, body, known, nil)
	inner := fixedBindingCall(t, body)
	nested, nestedClosure := ssaflow.DirectCallee(ssaflow.InstructionCall(inner))
	want := ssaflow.ProveFixedArgumentsWithin(ssaflow.InstructionCall(inner), nestedClosure, nested, first.Values, nil)
	if !want.Proven() || len(want.Values) != 1 || want.Values[nested.FreeVars[0]] != ssaflow.OutcomeFalse {
		t.Fatalf("forwarded capture=%+v", want)
	}
	checkFixedBindingCutoffs(t, func(budget *proofs.SearchBudget) ssaflow.FixedArgumentsProof {
		return ssaflow.ProveFixedArgumentsWithin(ssaflow.InstructionCall(inner), nestedClosure, nested, first.Values, budget)
	}, want)
}
