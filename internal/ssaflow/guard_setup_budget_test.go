package ssaflow

import (
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestDominatingGuardBudgetAndPolicy(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "setup", `package setup
 func marker(n int) {}
 func stable(flag bool) { if !flag { return }; marker(1) }
 func loaded(flag *bool) { if *flag { marker(2) } }
 func mutated(flag *bool) { if *flag { *flag=false; marker(3) } }
`)
	for _, scenario := range []struct {
		name   string
		count  int
		stable bool
	}{
		{"stable", 1, true}, {"loaded", 1, false}, {"mutated", 0, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			target := InstructionsOf[*ssa.Call](pkg.Func(scenario.name))[0]
			cutoff := NewSearchBudget(1)
			if guards := GuardsDominatingWithin(target, cutoff); guards != nil || !cutoff.Exhausted() {
				t.Fatal("interrupted setup cannot publish a partial guard seed")
			}
			fresh := NewSearchBudget(QueryBudget)
			guards := GuardsDominatingWithin(target, fresh)
			if len(guards) != scenario.count || fresh.Exhausted() || !slices.Equal(guards, GuardsDominating(target)) {
				t.Fatal("fresh setup changed stable/loaded/invalidation policy")
			}
			if len(guards) != 0 && guards[0].Stable != scenario.stable {
				t.Fatal("loaded evidence must not become a stable guard")
			}
			pool := NewSearchBudget(1)
			shared := pool.Within(QueryBudget)
			if guards := GuardsDominatingWithin(target, shared); guards != nil || !shared.PoolExhausted() {
				t.Fatal("condition/address queries must retain the setup pool")
			}
		})
	}
}

func TestGuardSetupCutoffStopsObligation(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "flow", `package flow
 func marker(n int) {}
 func stable(flag bool) { if !flag { return }; marker(1) }
`)
	target := InstructionsOf[*ssa.Call](pkg.Func("stable"))[0]
	classified := 0
	flow := ObligationFlow{Start: target, Budget: NewSearchBudget(2), Instruction: func(ssa.Instruction) ObligationAction {
		classified++
		return ObligationNone
	}}
	outcome, witness := EvaluateObligationWitness(flow)
	if outcome != ObligationUncertain || witness != nil || classified != 0 || !flow.Budget.Exhausted() {
		t.Fatal("guard setup cutoff must stop before any uncovered-return judgement")
	}
	flow.Budget = NewSearchBudget(QueryBudget)
	if outcome, witness := EvaluateObligationWitness(flow); outcome != ObligationViolated || witness == nil || flow.Budget.Exhausted() {
		t.Fatal("fresh guard setup must retain the real uncovered-return witness")
	}
}

func TestComputedGuardCycleBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "cycle", `package cycle
 func marker(n int) {}
 func once(n int) { if n>0 { marker(1) } }
 func loop(n int) { for i:=0; i<n; i++ { if i>0 { marker(2) } } }
`)
	condition := InstructionsOf[*ssa.If](pkg.Func("once"))[0].Cond
	cutoff := NewSearchBudget(1)
	if _, _, stable, ok := guardConditionWithin(condition, cutoff); stable || ok || !cutoff.Exhausted() {
		t.Fatal("unfinished cycle search must not turn a computed Boolean stable")
	}
	fresh := NewSearchBudget(QueryBudget)
	if _, _, stable, ok := guardConditionWithin(condition, fresh); !stable || !ok || fresh.Exhausted() {
		t.Fatal("fresh acyclic computed guard lost its stable identity")
	}
	for _, branch := range InstructionsOf[*ssa.If](pkg.Func("loop")) {
		if _, _, stable, ok := guardConditionWithin(branch.Cond, NewSearchBudget(QueryBudget)); stable && ok {
			t.Fatal("recomputed loop guards must remain uncorrelated")
		}
	}
}

func TestGuardAddressAndNegationBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "decode", `package decode
 type inner struct { flag bool }
 type outer struct { nested inner }
 func projected(p *outer) bool { return p.nested.flag }
 func negated(flag bool) bool { return !flag }
`)
	loaded := InstructionsOf[*ssa.Return](pkg.Func("projected"))[0].Results[0].(*ssa.UnOp)
	cutoff := NewSearchBudget(1)
	if _, ok := guardAddressIdentityWithin(loaded.X, cutoff); ok || !cutoff.Exhausted() {
		t.Fatal("partial nested address decoding cannot supply a guard identity")
	}
	fresh := NewSearchBudget(QueryBudget)
	identity, ok := guardAddressIdentityWithin(loaded.X, fresh)
	want, defaultOK := GuardAddressIdentity(loaded.X)
	if !ok || !defaultOK || identity != want || fresh.Exhausted() {
		t.Fatal("fresh nested field identity must preserve default encoding")
	}
	value := InstructionsOf[*ssa.Return](pkg.Func("negated"))[0].Results[0]
	cutoff = NewSearchBudget(1)
	if source, odd := booleanNegationSourceWithin(value, cutoff); source != nil || odd || !cutoff.Exhausted() {
		t.Fatal("partial negation decoding must not publish a source or parity")
	}
	fresh = NewSearchBudget(QueryBudget)
	source, odd := booleanNegationSourceWithin(value, fresh)
	wantSource, wantOdd := BooleanNegationSource(value)
	if source != wantSource || odd != wantOdd || fresh.Exhausted() {
		t.Fatal("fresh negation decoding must preserve exact source and parity")
	}
}
