package path

import (
	"go/types"
	"slices"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	ssaflow "github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestWalkStatesBudgetBeforeKeyAndRevisits(t *testing.T) {
	keys, steps := 0, 0
	zero := proofs.NewSearchBudget(0)
	cfg.WalkStatesWithin([]int{0}, func(n int) int { keys++; return n }, func(int) ([]int, bool) { steps++; return nil, true }, zero)
	if keys != 0 || steps != 0 || !zero.Exhausted() {
		t.Fatal("queued visits must spend before constructing keys")
	}
	fresh := proofs.NewSearchBudget(3)
	cfg.WalkStatesWithin([]int{0}, func(n int) int { return n % 2 }, func(n int) ([]int, bool) {
		steps++
		return []int{n + 1}, true
	}, fresh)
	if steps != 2 || fresh.Exhausted() || fresh.Spend() {
		t.Fatal("revisited keys must spend without expanding a third state")
	}
	for _, phase := range []string{"key", "step"} {
		budget := proofs.NewSearchBudget(3)
		keys, steps = 0, 0
		cfg.WalkStatesWithin([]int{0}, func(n int) int {
			keys++
			if phase == "key" {
				for budget.Spend() {
				}
			}
			return n
		}, func(n int) ([]int, bool) {
			steps++
			for budget.Spend() {
			}
			return []int{n + 1}, true
		}, budget)
		wantSteps := 1
		if phase == "key" {
			wantSteps = 0
		}
		if keys != 1 || steps != wantSteps || !budget.Exhausted() {
			t.Fatal("interrupted keys/successors must stop before admission")
		}
	}
}

func TestGuardStateBudgetKeepsDefaultPolicy(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "guards", `package guards
 type owner struct { flag bool }
 func subject(p *owner) { if p.flag { p.flag=false } }
 func stable(flag bool) { if flag { subject(nil) } }
`)
	function := pkg.Func("subject")
	branch := ssaflow.InstructionsOf[*ssa.If](function)[0]
	identity, _, _, ok := GuardCondition(branch.Cond)
	if !ok {
		t.Fatal("expected an actual loaded guard")
	}
	guards := PathGuards{{Identity: identity, Value: true, Stable: false}}
	store := ssaflow.InstructionsOf[*ssa.Store](function)[0]
	zero := proofs.NewSearchBudget(0)
	if kept := guards.AfterWithin(store, zero); kept != nil || !zero.Exhausted() {
		t.Fatal("unknown store identity cannot retain an active guard")
	}
	fresh := proofs.NewSearchBudget(proofs.QueryBudget)
	if kept := guards.AfterWithin(store, fresh); len(kept) != 0 || len(guards.After(store)) != 0 || fresh.Exhausted() {
		t.Fatal("fresh mutation must forget the same guard")
	}
	zero = proofs.NewSearchBudget(0)
	if key := guards.KeyWithin(zero); key != "" || !zero.Exhausted() {
		t.Fatal("partial state-key rendering must be unavailable")
	}
	if guards.KeyWithin(proofs.NewSearchBudget(proofs.QueryBudget)) != guards.KeyWithin(nil) {
		t.Fatal("fresh guard key must preserve default identity")
	}
	zero = proofs.NewSearchBudget(0)
	if edges := (SuccessorPolicy{}).EdgesWithin(branch.Block(), nil, guards, zero); edges != nil || !zero.Exhausted() {
		t.Fatal("edge census must spend before successor selection")
	}
	// Taking the other branch contradicts the loaded guard only uncertainly.
	_, loaded := guards.ExtendWithin(branch.Block(), branch.Block().Succs[1], nil, proofs.NewSearchBudget(proofs.QueryBudget))
	guards[0].Stable = true
	// Stability comes from the decoded condition, not an asserted held flag.
	_, sameLoaded := guards.ExtendWithin(branch.Block(), branch.Block().Succs[1], nil, nil)
	if loaded != GuardLoadedContradiction || sameLoaded != loaded {
		t.Fatal("bounded extension must preserve loaded contradiction semantics")
	}
	stableBranch := ssaflow.InstructionsOf[*ssa.If](pkg.Func("stable"))[0]
	stableIdentity, _, stable, ok := GuardCondition(stableBranch.Cond)
	if !ok || !stable {
		t.Fatal("expected an actual stable parameter guard")
	}
	stableGuards := PathGuards{{Identity: stableIdentity, Value: true, Stable: true}}
	_, contradiction := stableGuards.ExtendWithin(stableBranch.Block(), stableBranch.Block().Succs[1], nil, proofs.NewSearchBudget(proofs.QueryBudget))
	if contradiction != GuardStableContradiction {
		t.Fatal("bounded extension must preserve stable path pruning")
	}
}

func TestObligationRejectsExhaustedCallbacks(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "callbacks", `package callbacks
 func marker(n int) {}
 func subject(flag bool) { marker(0); marker(1); if flag { marker(2) } }
`)
	function := pkg.Func("subject")
	start := ssaflow.InstructionsOf[*ssa.Call](function)[0]
	for _, phase := range []string{"instruction", "return", "edge", "successors", "terminator"} {
		t.Run(phase, func(t *testing.T) {
			budget := proofs.NewSearchBudget(proofs.QueryBudget)
			called := false
			cut := func() {
				called = true
				for budget.Spend() {
				}
			}
			flow := ObligationFlow{Start: start, Budget: budget, Instruction: func(ssa.Instruction) ObligationAction { return ObligationNone }}
			switch phase {
			case "instruction":
				flow.Instruction = func(ssa.Instruction) ObligationAction { cut(); return ObligationExact }
			case "return":
				flow.Return = func(*ssa.Return) ObligationAction { cut(); return ObligationNone }
			case "edge":
				flow.Edge = func(*ssa.BasicBlock, *ssa.BasicBlock) ObligationAction { cut(); return ObligationExact }
			case "successors":
				flow.Successors = func(block, _ *ssa.BasicBlock) []*ssa.BasicBlock { cut(); return block.Succs }
			case "terminator":
				flow.Terminates = func(*ssa.Call) bool { cut(); return true }
			}
			outcome, witness := EvaluateObligationWitness(flow)
			if !called || !budget.Exhausted() || outcome != ObligationUncertain || witness != nil {
				t.Fatal("an interrupted callback cannot settle, violate or terminate a path")
			}
		})
	}
}

func TestBoundConditionAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "assumptions", `package assumptions
func direct(flag bool) { if flag { println(1) } }
func negated(flag bool) { if !flag { println(1) } }
func nilCheck(p *int) { if p != nil { println(1) } }
func reversed(p *int) { if nil != p { println(1) } }
func odd(flag bool) bool { return !flag }
`)
	for _, test := range []struct {
		name    string
		outcome ssacall.Outcome
		holds   bool
	}{
		{"direct", ssacall.OutcomeTrue, true},
		// SSA swaps edges for if !flag; its branch condition is still flag.
		{"negated", ssacall.OutcomeTrue, true},
		{"nilCheck", ssacall.OutcomeNonNil, true},
		{"reversed", ssacall.OutcomeNonNil, true},
	} {
		function := pkg.Func(test.name)
		branch := ssaflow.InstructionsOf[*ssa.If](function)[0]
		fixed := ssacall.FixedValues{function.Params[0]: test.outcome}
		fresh := proofs.NewSearchBudget(proofs.QueryBudget)
		holds, known := fixed.HoldsWithin(branch.Cond, fresh)
		if !known || holds != test.holds || fresh.Exhausted() {
			t.Fatalf("%s condition %s: holds=%t known=%t exhausted=%t", test.name, branch.Cond, holds, known, fresh.Exhausted())
		}
		for limit := range proofs.QueryBudget - fresh.Remaining() {
			cut := proofs.NewSearchBudget(limit)
			if _, known := fixed.HoldsWithin(branch.Cond, cut); known || !cut.Exhausted() {
				t.Fatal("an interrupted condition cannot decide a branch")
			}
		}
		cut := proofs.NewSearchBudget(0)
		if got := fixed.NarrowWithin(branch.Block().Succs, branch.Block(), cut); !slices.Equal(got, branch.Block().Succs) || !cut.Exhausted() {
			t.Fatal("interrupted narrowing must keep the primitive's edges unpruned")
		}
		got := fixed.NarrowWithin(branch.Block().Succs, branch.Block(), proofs.NewSearchBudget(proofs.QueryBudget))
		if !slices.Equal(got, fixed.Narrow(branch.Block().Succs, branch.Block())) {
			t.Fatal("fresh bound filtering must preserve default edges")
		}
	}
	odd := pkg.Func("odd")
	condition := ssaflow.InstructionsOf[*ssa.Return](odd)[0].Results[0]
	if _, ok := condition.(*ssa.UnOp); !ok {
		t.Fatal("expected an actual returned negation")
	}
	fixed := ssacall.FixedValues{odd.Params[0]: ssacall.OutcomeTrue}
	fresh := proofs.NewSearchBudget(proofs.QueryBudget)
	if holds, known := fixed.HoldsWithin(condition, fresh); holds || !known || fresh.Exhausted() {
		t.Fatal("a fresh odd negation must invert its exact binding")
	}
	for limit := range proofs.QueryBudget - fresh.Remaining() {
		cut := proofs.NewSearchBudget(limit)
		if _, known := fixed.HoldsWithin(condition, cut); known || !cut.Exhausted() {
			t.Fatal("interrupted negation cannot establish bound truth")
		}
	}
}

func TestNilFoldAllowanceKeepsBoxingOpaque(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "assumptions", `package assumptions
type pointer *int
func converted() pointer { return pointer((*int)(nil)) }
func boxed() any { return (*int)(nil) }
func mixed(flag bool) *int { var p *int; if flag { p = new(int) }; return p }
`)
	for _, name := range []string{"converted", "boxed", "mixed"} {
		value := ssaflow.InstructionsOf[*ssa.Return](pkg.Func(name))[0].Results[0]
		fresh := proofs.NewSearchBudget(proofs.QueryBudget)
		if got := ssaflow.DefinitelyNilWithin(value, fresh); got != (name == "converted") || fresh.Exhausted() {
			t.Fatal("nil-fold allowance must preserve boxing and phi policy")
		}
		for limit := range proofs.QueryBudget - fresh.Remaining() {
			cut := proofs.NewSearchBudget(limit)
			if ssaflow.DefinitelyNilWithin(value, cut) || !cut.Exhausted() {
				t.Fatal("an interrupted nil fold cannot supply definite nilness")
			}
		}
	}
}

func TestAssumedSuccessorAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "assumptions", `package assumptions
type owner struct { field *int }
type item struct{}
func direct(p *int) { if p != nil { println(1) } }
func field(p *owner) { if p.field != nil { println(1) } }
func foreign(p, q *owner) { if q.field != nil { println(1) } }
func asserted(p any) { if _, ok := p.(*item); ok { println(1) } }
func unmatched(p any) { if _, ok := p.(int); ok { println(1) } }
`)
	for _, name := range []string{"direct", "field", "foreign", "asserted", "unmatched"} {
		function := pkg.Func(name)
		branch := ssaflow.InstructionsOf[*ssa.If](function)[0]
		var concrete types.Type
		if name == "asserted" || name == "unmatched" {
			concrete = types.NewPointer(pkg.Pkg.Scope().Lookup("item").Type())
		}
		fresh := proofs.NewSearchBudget(proofs.QueryBudget)
		got := assumedSuccessorsWithin(branch.Block().Succs, branch.Block(), function.Params[0], concrete, fresh)
		want := branch.Block().Succs[:1]
		if name == "foreign" || name == "unmatched" {
			want = branch.Block().Succs
		}
		if !slices.Equal(got, want) || fresh.Exhausted() {
			t.Fatal("fresh assumption must require exact root/field/assertion identity")
		}
		for limit := range proofs.QueryBudget - fresh.Remaining() {
			cut := proofs.NewSearchBudget(limit)
			if got := assumedSuccessorsWithin(branch.Block().Succs, branch.Block(), function.Params[0], concrete, cut); got != nil || !cut.Exhausted() {
				t.Fatal("an interrupted assumption cannot supply pruned successors")
			}
		}
	}
}

func TestSuccessorAssumptionConsumersRejectCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "assumptions", `package assumptions
func bound(flag bool) { if flag { println(1) } }
func assumed(p *int) { if p != nil { println(1) } }
`)
	for _, name := range []string{"bound", "assumed"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			branch := ssaflow.InstructionsOf[*ssa.If](function)[0]
			policy := SuccessorPolicy{Feasible: func(block, _ *ssa.BasicBlock) []*ssa.BasicBlock { return block.Succs }}
			if name == "bound" {
				policy.Constants = ssacall.FixedValues{function.Params[0]: ssacall.OutcomeTrue}
			} else {
				policy.NonNil = function.Params[0]
			}
			cut := proofs.NewSearchBudget(0)
			if got := policy.SuccessorsWithin(branch.Block(), nil, cut); got != nil || !cut.Exhausted() {
				t.Fatal("successor policy cannot consume uncharged bound/assumption evidence")
			}
			freshBudget := proofs.NewSearchBudget(proofs.QueryBudget)
			if got := policy.SuccessorsWithin(branch.Block(), nil, freshBudget); !slices.Equal(got, policy.Successors(branch.Block(), nil)) {
				t.Fatal("fresh successor policy must preserve default filtering")
			}
		})
	}
}

func TestAssumptionFlowCutoffClearsWitness(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "assumptions", `package assumptions
func marker() {}
func cleanup() {}
func bound(flag bool) { marker(); if flag { cleanup() } }
func assumed(p *int) { marker(); if p != nil { cleanup() } }
`)
	for _, name := range []string{"bound", "assumed"} {
		function := pkg.Func(name)
		flow := ObligationFlow{
			Start: ssaflow.InstructionsOf[*ssa.Call](function)[0], Budget: proofs.NewSearchBudget(proofs.QueryBudget),
			Successors: func(block, _ *ssa.BasicBlock) []*ssa.BasicBlock { return block.Succs },
			Instruction: func(instruction ssa.Instruction) ObligationAction {
				if call, ok := instruction.(*ssa.Call); ok && call.Common().StaticCallee() == pkg.Func("cleanup") {
					return ObligationExact
				}
				return ObligationNone
			},
		}
		if name == "bound" {
			flow.Constants = ssacall.FixedValues{function.Params[0]: ssacall.OutcomeTrue}
		} else {
			flow.NonNil = function.Params[0]
		}
		if got, witness := EvaluateObligationWitness(flow); got != ObligationHonored || witness != nil || flow.Budget.Exhausted() {
			t.Fatal("fresh assumption must preserve exact return coverage")
		}
		used := proofs.QueryBudget - flow.Budget.Remaining()
		for limit := range used {
			flow.Budget = proofs.NewSearchBudget(limit)
			if got, witness := EvaluateObligationWitness(flow); got != ObligationUncertain || witness != nil || !flow.Budget.Exhausted() {
				t.Fatalf("%s limit %d retained incomplete coverage or a witness", name, limit)
			}
		}
	}
}

func TestBranchValueAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "literal", `package literal
func subject(a, b bool) bool { condition := a && b; if condition { return true }; return false }
`)
	phi := ssaflow.InstructionsOf[*ssa.Phi](pkg.Func("subject"))[0]
	for predecessor, operand := range ssaflow.PhiIncoming(phi) {
		fresh := proofs.NewSearchBudget(proofs.QueryBudget)
		if got := BranchValueWithin(phi, phi.Block(), predecessor, fresh); got != operand || fresh.Exhausted() {
			t.Fatal("fresh selection must preserve the actual incoming operand")
		}
		for limit := range proofs.QueryBudget - fresh.Remaining() {
			cut := proofs.NewSearchBudget(limit)
			if got := BranchValueWithin(phi, phi.Block(), predecessor, cut); got != nil || !cut.Exhausted() {
				t.Fatal("interrupted phi selection must supply no operand")
			}
		}
	}
	if got := BranchValueWithin(phi, pkg.Func("subject").Blocks[0], phi.Block().Preds[0], proofs.NewSearchBudget(proofs.QueryBudget)); got != phi {
		t.Fatal("a historical phi must not select a different block's incoming path")
	}
}

func TestLiteralBranchAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "literal", `package literal
func agree(flag bool) bool { if flag { return true }; return true }
func mixed(flag bool) bool { if flag { return true }; return false }
func deferred() (result bool) { defer func() { result = false }(); return true }
func forwarded() bool { return agree(true) }
func pair() (int, bool) { return 1, true }
func count() int { return 3 }
func agreeing(flag bool) { if agree(flag) { println(1) } }
func differing(flag bool) { if mixed(flag) { println(1) } }
func mutation() { if deferred() { println(1) } }
func forwarding() { if forwarded() { println(1) } }
func paired() { _, ok := pair(); if ok { println(1) } }
func compared() { if count() > 0 { println(1) } }
`)
	for _, test := range []struct {
		name  string
		known bool
	}{
		{"agreeing", true},
		{"differing", false},
		{"mutation", false},
		{"forwarding", false},
		{"paired", true},
		{"compared", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			branch := ssaflow.InstructionsOf[*ssa.If](pkg.Func(test.name))[0]
			fresh := proofs.NewSearchBudget(proofs.QueryBudget)
			holds, known := BranchBoolWithin(branch.Cond, branch.Block(), nil, fresh)
			if known != test.known || known && !holds || fresh.Exhausted() {
				t.Fatal("fresh evidence must preserve literal-only return policy")
			}
			for limit := range proofs.QueryBudget - fresh.Remaining() {
				cut := proofs.NewSearchBudget(limit)
				if _, known := BranchBoolWithin(branch.Cond, branch.Block(), nil, cut); known || !cut.Exhausted() {
					t.Fatal("interrupted helper/comparison evidence cannot decide a branch")
				}
			}
			freshBudget := proofs.NewSearchBudget(proofs.QueryBudget)
			if got := FeasibleSuccessorsWithin(branch.Block(), nil, freshBudget); !slices.Equal(got, FeasibleSuccessors(branch.Block(), nil)) {
				t.Fatal("fresh feasibility must retain default successors")
			}
			cut := proofs.NewSearchBudget(0)
			if got := FeasibleSuccessorsWithin(branch.Block(), nil, cut); !slices.Equal(got, branch.Block().Succs) || !cut.Exhausted() {
				t.Fatal("cutoff must retain all successors without pruning")
			}
		})
	}
}

func TestLiteralHelperCapRemainsIndependent(t *testing.T) {
	source := `package literal
func marker() {}
func admitted() bool { ` + strings.Repeat("marker();", 127) + `return true }
func oversized() bool { ` + strings.Repeat("marker();", 128) + `return true }
func first() { if admitted() { marker() } }
func second() { if oversized() { marker() } }
`
	pkg := ssaflowtest.BuildPackage(t, "literal", source)
	for _, test := range []struct {
		name  string
		known bool
	}{{"first", true}, {"second", false}} {
		branch := ssaflow.InstructionsOf[*ssa.If](pkg.Func(test.name))[0]
		budget := proofs.NewSearchBudget(proofs.QueryBudget)
		_, known := BranchBoolWithin(branch.Cond, branch.Block(), nil, budget)
		if known != test.known || budget.Exhausted() {
			t.Fatal("the independent helper cap must not exhaust the caller allowance")
		}
	}
}

func TestLiteralFeasibilityCutoffKeepsFlowUnknown(t *testing.T) {
	source := `package literal
func marker() {}
func cleanup() {}
func helper() bool { ` + strings.Repeat("marker();", 64) + `return true }
func subject() { if helper() { cleanup() } }
`
	pkg := ssaflowtest.BuildPackage(t, "literal", source)
	function := pkg.Func("subject")
	budget := proofs.NewSearchBudget(proofs.QueryBudget)
	flow := ObligationFlow{Budget: budget, Instruction: func(instruction ssa.Instruction) ObligationAction {
		if _, ok := instruction.(*ssa.If); ok {
			// The helper is within its own cap but exceeds the caller's
			// remaining allowance. No classifier answer is interrupted here.
			for budget.Remaining() > 32 {
				budget.Spend()
			}
		}
		if call, ok := instruction.(*ssa.Call); ok && call.Common().StaticCallee() == pkg.Func("cleanup") {
			return ObligationExact
		}
		return ObligationNone
	}}
	if got := EvaluateObligationFromEntry(function, flow); got != ObligationUncertain || !budget.Exhausted() {
		t.Fatal("interrupted helper feasibility cannot prove honored coverage")
	}
	flow.Budget = proofs.NewSearchBudget(proofs.QueryBudget)
	flow.Instruction = func(instruction ssa.Instruction) ObligationAction {
		if call, ok := instruction.(*ssa.Call); ok && call.Common().StaticCallee() == pkg.Func("cleanup") {
			return ObligationExact
		}
		return ObligationNone
	}
	if got := EvaluateObligationFromEntry(function, flow); got != ObligationHonored || flow.Budget.Exhausted() {
		t.Fatal("fresh literal feasibility must retain exact coverage")
	}
}

func TestReachableConstantBlocksAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "reachablebudget", `package reachablebudget
 func use(){}
 func branch(yes bool){if yes{use()}else{return};use()}
 func loop(yes bool){for yes{use()}}
 `)
	for _, name := range []string{"branch", "loop"} {
		for _, outcome := range []ssacall.Outcome{ssacall.OutcomeTrue, ssacall.OutcomeFalse} {
			fn := pkg.Func(name)
			constants := ssacall.FixedValues{fn.Params[0]: outcome}
			baseline := ReachableBlocksAssumingWithin(fn, constants, nil)
			for limit := 0; limit <= proofs.QueryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				blocks := ReachableBlocksAssumingWithin(fn, constants, budget)
				if budget.Exhausted() || limit == 0 {
					if blocks != nil {
						t.Fatalf("cut census retained%d blocks", len(blocks))
					}
					continue
				}
				if !slices.Equal(blocks, baseline) {
					t.Fatal("completed census changed discovery order")
				}
				break
			}
		}
	}
}

func TestNormalReturnProofAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returns", `package returns
import "os"
func normal() {}
func forever() { for {} }
func die() { os.Exit(0) }
func deferred() { defer os.Exit(0) }
func conditional(flag bool) { if flag { defer os.Exit(0) } }
`)
	for _, test := range []struct {
		name      string
		reachable bool
	}{{"normal", true}, {"forever", false}, {"die", false}, {"deferred", false}, {"conditional", true}} {
		function := pkg.Func(test.name)
		fresh := proofs.NewSearchBudget(proofs.QueryBudget)
		proof := ProveNormalReturnWithin(function.Blocks[0], nil, fresh)
		if !proof.Known() || proof.Proven() != test.reachable || fresh.Exhausted() ||
			(proof.Witness != nil) != test.reachable || NormalReturnReachableWith(function.Blocks[0], nil) != test.reachable {
			t.Fatalf("%s: fresh return proof = %+v", test.name, proof)
		}
		for limit := range proofs.QueryBudget - fresh.Remaining() {
			cut := proofs.NewSearchBudget(limit)
			proof := ProveNormalReturnWithin(function.Blocks[0], nil, cut)
			if proof.Known() || proof.Witness != nil || proof.Reason != proofs.EvidenceBudgetExhausted || !cut.Exhausted() {
				t.Fatal("interrupted reachability cannot prove a return or its absence")
			}
		}
		pool := proofs.NewSearchBudget(0)
		if proof := ProveNormalReturnWithin(function.Blocks[0], nil, pool.Within(proofs.QueryBudget)); proof.Known() || proof.Witness != nil ||
			!pool.Exhausted() {
			t.Fatal("pool cutoff cannot establish normal-return reachability")
		}
	}
	if proof := ProveNormalReturnWithin(nil, nil, proofs.NewSearchBudget(proofs.QueryBudget)); proof.Known() || proof.Reason != proofs.EvidenceUnavailable {
		t.Fatal("missing entry is unavailable, not proof of termination")
	}
}

func TestNormalReturnRejectsInterruptedTerminator(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returns", `package returns
func marker() {}
func subject() { marker() }
`)
	for _, answer := range []bool{false, true} {
		budget := proofs.NewSearchBudget(proofs.QueryBudget)
		called := false
		proof := ProveNormalReturnWithin(pkg.Func("subject").Blocks[0], func(*ssa.Call) bool {
			called = true
			for budget.Spend() {
			}
			return answer
		}, budget)
		if !called || !budget.Exhausted() || proof.Known() || proof.Witness != nil || proof.Reason != proofs.EvidenceBudgetExhausted {
			t.Fatal("an interrupted terminator cannot prove presence or absence of a return")
		}
	}
}

func TestInstructionOrderAndIndexBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "order", `package order
 func marker(value int) {}
 func subject(flag bool) { marker(1); marker(2); if flag { marker(3) }; marker(4) }
`)
	calls := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("subject"))
	for _, before := range calls {
		zero := proofs.NewSearchBudget(0)
		if cfg.InstructionIndexWithin(before, zero) != -1 || !zero.Exhausted() {
			t.Fatal("initial position cutoff must remain unavailable")
		}
		fresh := proofs.NewSearchBudget(proofs.QueryBudget)
		if cfg.InstructionIndexWithin(before, fresh) != cfg.InstructionIndex(before) || fresh.Exhausted() {
			t.Fatal("fresh index must preserve actual SSA position")
		}
		for _, after := range calls {
			zero := proofs.NewSearchBudget(0)
			if cfg.InstructionDominatesWithin(before, after, zero) || !zero.Exhausted() {
				t.Fatal("same/cross-block dominance must charge before evidence")
			}
			fresh := proofs.NewSearchBudget(proofs.QueryBudget)
			if cfg.InstructionDominatesWithin(before, after, fresh) != cfg.InstructionDominates(before, after) || fresh.Exhausted() {
				t.Fatal("fresh dominance changed direction or block order")
			}
		}
	}
	pool := proofs.NewSearchBudget(1)
	shared := pool.Within(proofs.QueryBudget)
	if cfg.InstructionDominatesWithin(calls[0], calls[1], shared) || !shared.PoolExhausted() {
		t.Fatal("both same-block positions must share the candidate pool")
	}
}

func TestObligationInitialLookupCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "setup", `package setup
 func marker(value int) {}
 func subject() { marker(1); marker(2); marker(3); marker(4) }
`)
	calls := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("subject"))
	for _, action := range []ObligationAction{ObligationNone, ObligationExact} {
		classified := 0
		flow := ObligationFlow{Start: calls[2], Budget: proofs.NewSearchBudget(2), Instruction: func(ssa.Instruction) ObligationAction {
			classified++
			return action
		}}
		outcome, witness := EvaluateObligationWitness(flow)
		if outcome != ObligationUncertain || witness != nil || classified != 0 || !flow.Budget.Exhausted() {
			t.Fatal("incomplete setup cannot classify, honor or violate an obligation")
		}
		flow.Budget = proofs.NewSearchBudget(proofs.QueryBudget)
		got, witness := EvaluateObligationWitness(flow)
		want := ObligationHonored
		if action == ObligationNone {
			want = ObligationViolated
		}
		if got != want || flow.Budget.Exhausted() || (witness != nil) != (want == ObligationViolated) {
			t.Fatal("fresh setup lost exact coverage or its uncovered-return witness")
		}
	}
}

func TestDeferredTerminationAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "termination", `package termination
import "os"
func marker() {}
func unconditional() { marker(); defer os.Exit(0); marker() }
func conditional(flag bool) { if flag { defer os.Exit(0) }; marker() }
func unrelated() { defer marker(); marker() }
`)
	for _, name := range []string{"unconditional", "conditional", "unrelated"} {
		t.Run(name, func(t *testing.T) {
			runs := ssaflow.InstructionsOf[*ssa.RunDefers](pkg.Func(name))
			if len(runs) == 0 {
				t.Fatal("expected actual SSA deferred execution")
			}
			for _, run := range runs {
				fresh := proofs.NewSearchBudget(proofs.QueryBudget)
				got := InstructionTerminatesWithin(run, nil, fresh)
				if got != (name == "unconditional") || got != InstructionTerminatesWith(run, nil) || fresh.Exhausted() {
					t.Fatal("fresh query must preserve conditional registration and catalog policy")
				}
				used := proofs.QueryBudget - fresh.Remaining()
				for limit := range used {
					cut := proofs.NewSearchBudget(limit)
					if InstructionTerminatesWithin(run, nil, cut) || !cut.Exhausted() {
						t.Fatalf("limit %d admitted termination or missed interrupted work", limit)
					}
				}
				pool := proofs.NewSearchBudget(0)
				if InstructionTerminatesWithin(run, nil, pool.Within(proofs.QueryBudget)) || !pool.Exhausted() {
					t.Fatal("candidate-pool cutoff cannot establish termination")
				}
			}
		})
	}
}

func TestTerminationCallbackAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "termination", `package termination
func marker() {}
func subject() { marker() }
`)
	call := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("subject"))[0]
	zero := proofs.NewSearchBudget(0)
	called := false
	if InstructionTerminatesWithin(call, func(*ssa.Call) bool { called = true; return true }, zero) || called || !zero.Exhausted() {
		t.Fatal("call dispatch must spend before consulting a callback")
	}
	budget := proofs.NewSearchBudget(proofs.QueryBudget)
	if InstructionTerminatesWithin(call, func(*ssa.Call) bool {
		for budget.Spend() {
		}
		return true
	}, budget) || !budget.Exhausted() {
		t.Fatal("an interrupted callback cannot establish termination")
	}
	if !InstructionTerminatesWithin(call, func(*ssa.Call) bool { return true }, proofs.NewSearchBudget(proofs.QueryBudget)) {
		t.Fatal("a fresh callback still establishes termination")
	}
}

func TestDeferredTerminationCutoffKeepsFlowUnknown(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "termination", `package termination
import "os"
func marker() {}
func subject() { marker(); defer os.Exit(0); marker() }
`)
	function := pkg.Func("subject")
	budget := proofs.NewSearchBudget(proofs.QueryBudget)
	called := false
	flow := ObligationFlow{Budget: budget, Instruction: func(instruction ssa.Instruction) ObligationAction {
		if _, ok := instruction.(*ssa.RunDefers); ok {
			called = true
			// Leave one visit for the nested census, so its next instruction
			// cuts off without interrupting this classifier's own answer.
			for budget.Remaining() > 1 {
				budget.Spend()
			}
		}
		return ObligationNone
	}}
	if got := EvaluateObligationFromEntry(function, flow); got != ObligationUncertain || !called || !budget.Exhausted() {
		t.Fatal("interrupted deferred termination cannot prove honored coverage")
	}
	flow.Budget = proofs.NewSearchBudget(proofs.QueryBudget)
	flow.Instruction = func(ssa.Instruction) ObligationAction { return ObligationNone }
	if got := EvaluateObligationFromEntry(function, flow); got != ObligationHonored || flow.Budget.Exhausted() {
		t.Fatal("fresh deferred exit must retain the default honored outcome")
	}
}
