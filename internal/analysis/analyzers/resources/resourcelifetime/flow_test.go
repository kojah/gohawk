package resourcelifetime

import (
	"go/token"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/resultfacts"
	"github.com/kojah/gohawk/internal/analysis/summaries"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/resourcemodel"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestResourceKeyKeepsObligationAtSameLocation(t *testing.T) {
	analysis, _ := helperBudgetAnalysis(t, "println(value)")
	block := analysis.acquisition.Block()
	obligation := resourcemodel.Acquired()
	states := []resourceFlowState{
		{block: block, obligation: obligation},
		{block: block, obligation: obligation.Discharged()},
		{block: block, obligation: obligation.Uncertain()},
		{block: block, obligation: obligation.Absent()},
	}
	expanded := 0
	cfg.WalkStates(states, func(state resourceFlowState) resourceFlowKey { return resourceStateKey(state, nil) },
		func(resourceFlowState) ([]resourceFlowState, bool) { expanded++; return nil, true })
	if expanded != len(states) {
		t.Fatalf("obligation collapsed at one guarded position: expanded=%d", expanded)
	}
}

func TestResourceFlowCandidateAllowance(t *testing.T) {
	for _, test := range []struct {
		body string
		want proofs.EvidenceState
	}{{"value.Close()", proofs.EvidenceDisproven}, {"println(value)", proofs.EvidenceProven}} {
		t.Run(test.body, func(t *testing.T) {
			analysis, _ := helperBudgetAnalysis(t, test.body)
			analysis.actions = make(map[ssa.Instruction]resourceAction)
			for _, block := range analysis.function.Blocks {
				for _, instruction := range block.Instrs {
					t.Log(instruction.String())
				}
			}
			got := analysis.proveResourceFlow(nil)
			if got.state != test.want || analysis.pool.Exhausted() || (got.leak != nil) != (test.want == proofs.EvidenceProven) {
				t.Fatalf("fresh flow = %+v, want state %v", got, test.want)
			}
			checkResourceFlowCutoffs(t, test.body, test.want)
		})
	}
}

func TestResourceFlowGuardedAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "guardbudget", `package guardbudget
type resource struct{}
func (*resource) Close() {}
func acquire() *resource { return new(resource) }
func leaky(flag bool) { if !flag { return }; value := acquire(); println(value) }
func closed(flag bool) { if !flag { return }; value := acquire(); value.Close() }
func presence(flag bool) { value := acquire(); if value != nil { value.Close() } }
`)
	for _, test := range []struct {
		name string
		want proofs.EvidenceState
	}{{"leaky", proofs.EvidenceProven}, {"closed", proofs.EvidenceDisproven}, {"presence", proofs.EvidenceDisproven}} {
		function := pkg.Func(test.name)
		acquisition := ssaflow.InstructionsOf[*ssa.Call](function)[0]
		if test.name != "presence" && len(ssapath.GuardsDominatingWithin(acquisition, nil)) == 0 {
			t.Fatal("fixture must establish a dominating guard in actual SSA")
		}
		completed := false
		for limit := 0; limit <= proofs.SummaryBudget; limit++ {
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			analysis := &resourceAnalysis{
				function: function, acquisition: acquisition, resource: acquisition,
				evidence: evidence, summaries: provider, contract: resourceContract{cleanup: []string{"Close"}},
				actions: make(map[ssa.Instruction]resourceAction), pool: proofs.NewSearchBudget(limit),
			}
			got := analysis.proveResourceFlow(nil)
			if !analysis.pool.Exhausted() {
				if got.state != test.want {
					t.Fatalf("guarded complete proof = %+v", got)
				}
				completed = true
				break
			}
			if got.state != proofs.EvidenceUnknown || got.reason != resourceReasonBudgetExhausted || got.leak != nil {
				t.Fatalf("guarded cutoff admitted proof: %+v", got)
			}
		}
		if !completed {
			t.Fatal("guarded proof never completed")
		}
	}
}

func checkResourceFlowCutoffs(t *testing.T, body string, want proofs.EvidenceState) {
	t.Helper()
	completed := false
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		analysis, _ := helperBudgetAnalysis(t, body)
		analysis.actions = make(map[ssa.Instruction]resourceAction)
		analysis.pool = proofs.NewSearchBudget(limit)
		got := analysis.proveResourceFlow(nil)
		if !analysis.pool.Exhausted() {
			if got.state != want {
				t.Fatalf("completed flow = %+v, want %v", got, want)
			}
			completed = true
			break
		}
		if got.state != proofs.EvidenceUnknown || got.reason != resourceReasonBudgetExhausted || got.leak != nil || analysis.leak != nil {
			t.Fatalf("allowance %d admitted incomplete flow: %+v", limit, got)
		}
	}
	if !completed {
		t.Fatal("flow never completed")
	}
}

func TestResourcePresenceAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "presencebudget", `package presencebudget
type resource struct{}
func direct(value *resource) { if value != nil { println(value) } }
func reversed(value *resource) { if nil == value { println(value) } }
func asserted(value *resource) { var boxed any = value; if _, ok := boxed.(*resource); ok { println(value) } }
func unrelated(value, other *resource) { if other != nil { println(value) } }
func incompatible(value *resource) { var boxed any = value; if _, ok := boxed.(*int); ok { println(value) } }
`)
	for _, test := range []struct {
		name    string
		known   bool
		present bool
	}{{"direct", true, true}, {"reversed", true, false}, {"asserted", true, true}, {"unrelated", false, false}, {"incompatible", false, false}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			branch := ssaflow.InstructionsOf[*ssa.If](fn)[0]
			block := branch.Block()
			t.Log(branch.String())
			for arm, successor := range block.Succs {
				wantPresent := test.present == (arm == 0)
				completed := false
				for limit := 0; limit <= proofs.SummaryBudget; limit++ {
					pool := proofs.NewSearchBudget(limit)
					budget := pool.Within(proofs.SummaryBudget)
					got := proveResourcePresenceBranch(block, nil, successor, fn.Params[0], budget)
					if resourceFlowExhausted(budget) {
						if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
							t.Fatalf("allowance %d admitted interrupted presence: %+v", limit, got)
						}
						continue
					}
					if got.Proven() != test.known || test.known && got.Present != wantPresent {
						t.Fatalf("completed arm %d presence = %+v", arm, got)
					}
					completed = true
					break
				}
				if !completed {
					t.Fatal("presence proof never completed")
				}
			}
		})
	}
}

func TestResourcePresenceUsesIncomingPath(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "presencepath", `package presencepath
type resource struct{}
func guarded(value *resource, flag bool) {
	condition := flag && value != nil
	if condition { println(value) }
}
`)
	fn := pkg.Func("guarded")
	var branch *ssa.If
	for _, candidate := range ssaflow.InstructionsOf[*ssa.If](fn) {
		if _, ok := candidate.Cond.(*ssa.Phi); ok {
			branch = candidate
		}
	}
	if branch == nil {
		t.Fatal("fixture lacks saved short-circuit phi")
	}
	block := branch.Block()
	for _, predecessor := range append([]*ssa.BasicBlock{nil, {}}, block.Preds...) {
		incoming := ssapath.BranchValueWithin(branch.Cond, block, predecessor, nil)
		_, known := incoming.(*ssa.BinOp)
		for arm, successor := range block.Succs {
			got := proveResourcePresenceBranch(block, predecessor, successor, fn.Params[0], proofs.NewSearchBudget(proofs.SummaryBudget))
			if got.Proven() != known || known && got.Present != (arm == 0) {
				t.Fatalf("incoming %v, arm %d presence = %+v", incoming, arm, got)
			}
		}
	}
}

func TestResourceReturnAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returnbudget", `package returnbudget
type resource struct{}
func (*resource) Close() {}
type owner struct { value *resource }
func direct(value, other *resource) *resource { return value }
func nested(value, other *resource) *owner { return &owner{value} }
func unrelated(value, other *resource) *resource { return other }
func opaque(*resource) *resource
func derived(value, other *resource) *resource { return opaque(value) }
func scalar(*resource) int
func scalarResult(value, other *resource) int { return scalar(value) }
`)
	for _, test := range []struct {
		name  string
		owner bool
		want  proofs.EvidenceState
	}{
		{"direct", false, proofs.EvidenceDisproven},
		{"nested", false, proofs.EvidenceDisproven},
		{"unrelated", false, proofs.EvidenceProven},
		{"unrelated", true, proofs.EvidenceDisproven},
		{"derived", false, proofs.EvidenceDisproven},
		{"scalarResult", false, proofs.EvidenceProven},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			analysis := &resourceAnalysis{
				function: fn, resource: fn.Params[0], summaries: provider, evidence: evidence,
				contract: resourceContract{cleanup: []string{"Close"}},
			}
			if test.owner {
				analysis.owners = []ssa.Value{fn.Params[1]}
			}
			completed := false
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				analysis.pool = proofs.NewSearchBudget(limit)
				budget := analysis.budget(proofs.SummaryBudget)
				got := analysis.proveResourceReturn(returned, budget)
				if resourceFlowExhausted(budget) {
					if got.state != proofs.EvidenceUnknown || got.reason != resourceReasonBudgetExhausted || got.leak != nil {
						t.Fatalf("allowance %d admitted interrupted return: %+v", limit, got)
					}
					continue
				}
				if limit == 0 || got.state != test.want || (got.leak != nil) != (test.want == proofs.EvidenceProven) {
					t.Fatalf("complete return proof = %+v at allowance %d", got, limit)
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("return proof never completed")
			}
		})
	}
}

func TestResourceBranchAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "branchbudget", `package branchbudget
import (
 "errors"
 "os"
)
func identity(err error) error { return err }
func direct(err error) { if err != nil { println(err) } }
func reversed(err error) { if nil == err { println(err) } }
func unrelated(err, other error) { if other != nil { println(err) } }
func asserted(err error) { if _, ok := err.(interface{ Timeout() bool }); ok { println(err) } }
func sentinel(err error) { if err == os.ErrNotExist { println(err) } }
func matching(err error) { if errors.Is(err, os.ErrNotExist) { println(err) } }
func wrapped(err error) { if os.IsNotExist(identity(err)) { println(err) } }
func joined(err error) { if errors.Is(errors.Join(err, os.ErrNotExist), os.ErrNotExist) { println(err) } }
func derived(err error) {
`+strings.Repeat("err = identity(err)\n", 64)+`if err != nil { println(err) }
}
func predicate(err error) bool {
`+strings.Repeat("println(err)\n", 32)+`return err != nil
}
func summarized(err error) { if predicate(err) { println(err) } }
func captured(err error) { check := predicate; func() { if check(err) { println(err) } }() }
`)
	for _, test := range []struct {
		name    string
		known   bool
		success bool
	}{
		{"direct", true, false},
		{"reversed", true, true},
		{"unrelated", false, false},
		{"asserted", true, false},
		{"derived", true, false},
		{"summarized", true, false},
		{"captured", true, false},
		{"sentinel", true, false},
		{"matching", true, false},
		{"wrapped", true, false},
		{"joined", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			errorValue := ssa.Value(fn.Params[0])
			if test.name == "captured" {
				fn = fn.AnonFuncs[0]
				errorValue = ssaflow.InstructionsOf[*ssa.Call](fn)[0].Common().Args[0]
			}
			branch := ssaflow.InstructionsOf[*ssa.If](fn)[0]
			if test.name == "derived" && len(ssaflow.InstructionsOf[*ssa.Call](fn)) != 65 {
				t.Fatal("fixture must retain 64 provenance calls and println in actual SSA")
			}
			t.Log(branch.String())
			if test.name == "derived" || test.name == "summarized" || test.name == "captured" {
				// These actual bodies require more than sixteen structural visits;
				// standalone derivation or summary budgets would wrongly accept them.
				got := proveResourceSuccessBranch(nil, branchTestKnowledge(), branch.Block(), branch.Block().Succs[0],
					errorValue, token.NoPos, proofs.NewSearchBudget(16))
				if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
					t.Fatalf("nested work bypassed the branch allowance: %+v", got)
				}
			}
			for arm, successor := range branch.Block().Succs {
				wantSuccess := test.success == (arm == 0)
				// Non-nil predicate evidence only decides the true edge.
				wantKnown := test.known && (arm == 0 || test.name == "direct" || test.name == "reversed" || test.name == "derived")
				checkResourceBranchArm(t, branch, successor, errorValue, wantKnown, wantSuccess)
			}
		})
	}
}

func checkResourceBranchArm(t *testing.T, branch *ssa.If, successor *ssa.BasicBlock, errorValue ssa.Value, wantKnown, wantSuccess bool) {
	t.Helper()
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		got := proveResourceSuccessBranch(nil, branchTestKnowledge(), branch.Block(), successor, errorValue, token.NoPos, budget)
		if resourceFlowExhausted(budget) {
			if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || got.success {
				t.Fatalf("allowance %d admitted interrupted branch: %+v", limit, got)
			}
			continue
		}
		if got.Proven() != wantKnown || wantKnown && got.success != wantSuccess {
			t.Fatalf("completed branch = %+v", got)
		}
		return
	}
	t.Fatal("fresh branch never completed")
}

func branchTestKnowledge() *summaries.Provider {
	return resourceSummaries.Provider(&analysis.Pass{ResultOf: map[*analysis.Analyzer]any{resultfacts.Analyzer: resultfacts.NewEngine()}})
}

func TestResourceBranchChildCutoffDiscardsSuccessors(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "branchcutoff", `package branchcutoff
type resource struct{}
func acquire() *resource { return new(resource) }
func identity(err error) error { return err }
func branch(err error) {
value := acquire()
`+strings.Repeat("err = identity(err)\n", proofs.SummaryBudget)+`if err != nil { println(value) }
}`)
	fn := pkg.Func("branch")
	branch := ssaflow.InstructionsOf[*ssa.If](fn)[0]
	acquisition := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	state := resourceFlowState{block: branch.Block()}
	knowledge := branchTestKnowledge()
	evidence, _ := knowledge.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	query := &resourceAnalysis{
		summaries: knowledge, evidence: evidence, function: fn, acquisition: acquisition, resource: acquisition, pool: pool,
		contract: resourceContract{cleanup: []string{"Close"}}, actions: map[ssa.Instruction]resourceAction{},
	}
	got := resourceSuccessorStates(query, state, fn.Params[0], pool)
	if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || got.states != nil || pool.Exhausted() {
		t.Fatalf("branch child cutoff must discard all successors while outer pool remains live: %+v, outer exhausted %v", got, pool.Exhausted())
	}
	// Isolate the edge proof from instruction classification so this control
	// requires the final flow to retain the child query's availability.
	for instruction := range ssaflow.InstructionsWithin(fn, nil) {
		query.actions[instruction] = actionNone
	}
	query.pool = proofs.NewSearchBudget(resourcePoolBudget)
	flow := query.proveResourceFlow(fn.Params[0])
	if flow.state != proofs.EvidenceUnknown || flow.reason != resourceReasonBudgetExhausted || flow.leak != nil || query.pool.Exhausted() {
		t.Fatalf("final flow admitted a child cutoff: %+v, outer exhausted %v", flow, query.pool.Exhausted())
	}
}

func TestSQLRowsEdgeAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "rowsbudget", `package rowsbudget
import "database/sql"
func next(rows, other *sql.Rows) { if rows.Next() { println(rows) } }
`)
	fn := pkg.Func("next")
	branch := ssaflow.InstructionsOf[*ssa.If](fn)[0]
	for arm, successor := range branch.Block().Succs {
		for index, resource := range fn.Params {
			completed := false
			for limit := 0; limit <= proofs.QueryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				got := proveSQLRowsExhaustionEdge(branch.Block(), successor, resource, budget)
				if resourceFlowExhausted(budget) {
					if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
						t.Fatalf("Rows cutoff admitted proof: %+v", got)
					}
					continue
				}
				if got.Proven() != (arm == 1 && index == 0) {
					t.Fatalf("Rows arm %d receiver %d = %+v", arm, index, got)
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("fresh Rows query never completed")
			}
		}
	}
}
