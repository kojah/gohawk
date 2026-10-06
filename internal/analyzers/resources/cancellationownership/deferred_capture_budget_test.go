package cancellationownership

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/lifecycle"

	analysisproof "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/ssa"
)

const deferredCaptureFixture = `package capturecells
 func direct(cancel func()){saved:=cancel;defer func(){saved()}()}
 func multiple(cancel func()){saved:=cancel;defer func(){saved()}();defer func(){saved()}()}
 func nested(cancel func()){saved:=cancel;defer func(){func(){saved()}()}()}
 func loaded(cancel func()){saved:=cancel;defer func(){saved()}();saved()}
 func rewritten(cancel func()){saved:=cancel;defer func(){saved()}();saved=func(){}}
 func nestedWrite(cancel func()){saved:=cancel;defer func(){func(){saved();saved=func(){}}()}()}
 func beforeStore(cancel func()){var saved func();defer func(){saved()}();saved=cancel}
 func launched(cancel func()){saved:=cancel;go func(){saved()}()}
 func handed(cancel func(),run func(func())){saved:=cancel;run(func(){saved()})}
`

func TestDeferredCaptureAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "capturecells", deferredCaptureFixture)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"direct", true},
		{"multiple", true},
		{"nested", true},
		{"loaded", false},
		{"rewritten", false},
		{"nestedWrite", false},
		{"beforeStore", false},
		{"launched", false},
		{"handed", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			query := &cancellationClassifier{cancel: fn.Params[0]}
			store := cancelCaptureStore(t, fn)
			closures := ssaflow.InstructionsOf[*ssa.MakeClosure](fn)
			if len(closures) == 0 {
				t.Fatal("no closure")
			}
			proofs := []func(*analysisproof.SearchBudget) analysisproof.Proof{
				func(budget *analysisproof.SearchBudget) analysisproof.Proof {
					return query.proveDeferredCaptureCellWithin(store, budget)
				},
				func(budget *analysisproof.SearchBudget) analysisproof.Proof {
					return query.proveDeferredCaptureWithin(closures[0], budget)
				},
			}
			for _, prove := range proofs {
				checkDeferredCaptureAllowance(t, prove, test.want)
			}
			wrong := &cancellationClassifier{}
			if proof := wrong.proveDeferredCaptureCellWithin(store, nil); proof.Proven() {
				t.Fatalf("unrelated target=%+v", proof)
			}
		})
	}
}

func cancelCaptureStore(t *testing.T, fn *ssa.Function) *ssa.Store {
	t.Helper()
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](fn) {
		if store.Val == fn.Params[0] {
			return store
		}
	}
	t.Fatal("no cancel store")
	return nil
}

func checkDeferredCaptureAllowance(t *testing.T, prove func(*analysisproof.SearchBudget) analysisproof.Proof, want bool) {
	t.Helper()
	baseline := prove(nil)
	if baseline.Proven() != want {
		t.Fatalf("default=%+v want%v", baseline, want)
	}
	completed := false
	for limit := 0; limit <= analysisproof.SummaryBudget; limit++ {
		budget := analysisproof.NewSearchBudget(limit)
		got := prove(budget)
		if budget.Exhausted() || limit == 0 {
			if got.State != analysisproof.EvidenceUnknown || got.Reason != analysisproof.EvidenceBudgetExhausted {
				t.Fatalf("cut%d=%+v", limit, got)
			}
			continue
		}
		if got != baseline {
			t.Fatalf("complete=%+v want%+v", got, baseline)
		}
		completed = true
		break
	}
	if !completed {
		t.Fatal("capture never completed")
	}
	pool := analysisproof.NewSearchBudget(10 * analysisproof.SummaryBudget)
	child := pool.Within(1)
	if proof := prove(child); proof.State != analysisproof.EvidenceUnknown || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("child=%+v", proof)
	}
	if proof := prove(pool.Within(analysisproof.SummaryBudget)); proof != baseline {
		t.Fatalf("fresh=%+v want%+v", proof, baseline)
	}
}

func TestDeferredCaptureClassifierCutoff(t *testing.T) {
	fn := ssaflowtest.BuildPackage(t, "capturecells", deferredCaptureFixture).Func("direct")
	query := &cancellationClassifier{cancel: fn.Params[0], pool: analysisproof.NewSearchBudget(0)}
	store := cancelCaptureStore(t, fn)
	if label := query.classifyAction(store); label.action != cancellationActionUnknown {
		t.Fatalf("cut store=%+v", label)
	}
	deferred := ssaflow.InstructionsOf[*ssa.Defer](fn)[0]
	if label, ok := query.deferredLiteralLabel(deferred); !ok || label.action != cancellationActionUnknown {
		t.Fatalf("cut defer=%+v/%v", label, ok)
	}
	query.pool = analysisproof.NewSearchBudget(cancellationPoolBudget)
	if label := query.classifyAction(store); label.action != cancellationActionNone {
		t.Fatalf("fresh store=%+v", label)
	}
	if label, ok := query.deferredLiteralLabel(deferred); !ok || label.action != cancellationActionRelease {
		t.Fatalf("fresh defer=%+v/%v", label, ok)
	}
}

func TestDeferredCaptureFlowControls(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "captureflows", `package captureflows
 import "context"
 func released(){_,cancel:=context.WithCancel(context.Background());defer func(){cancel()}()}
 func lost()(err error){_,cancel:=context.WithCancel(context.Background());defer func(){if err!=nil{cancel()}}();return nil}
 func rewritten(){_,cancel:=context.WithCancel(context.Background());defer func(){cancel()}();cancel=func(){}}
 func handed(run func(func())){_,cancel:=context.WithCancel(context.Background());run(func(){cancel()})}
 `)
	for _, test := range []struct {
		name string
		want CancellationOutcome
	}{
		{"released", CancellationReleased}, {"lost", CancellationLost}, {"rewritten", CancellationUnknown}, {"handed", CancellationUnknown},
	} {
		fn := pkg.Func(test.name)
		checked := false
		for _, extract := range ssaflow.InstructionsOf[*ssa.Extract](fn) {
			if extract.Index != 1 {
				continue
			}
			call, ok := extract.Tuple.(*ssa.Call)
			if !ok {
				continue
			}
			got := proveCancellation(call, extract, analysisTrace.Probe{}, nil, nil)
			if got.Outcome != test.want {
				t.Fatalf("%s=%+v want%v", test.name, got, test.want)
			}
			checked = true
			break
		}
		if !checked {
			t.Fatal("no acquisition")
		}
	}
}

func TestDeferredCaptureGuardCensusAllowance(t *testing.T) {
	fn := ssaflowtest.BuildPackage(t, "captureguards", `package captureguards
 import "context"
 func multiple()(err error){
  _,cancel:=context.WithCancel(context.Background())
  defer func(){if err!=nil{cancel()}}()
  defer func(){if err==nil{cancel()}}()
  return nil
 }
 `).Func("multiple")
	var cancel ssa.Value
	for _, extract := range ssaflow.InstructionsOf[*ssa.Extract](fn) {
		if extract.Index == 1 {
			cancel = extract
			break
		}
	}
	if cancel == nil {
		t.Fatal("no cancel")
	}
	discovery := lifecycle.ProveResultGuards(fn, lifecycle.CompletionRequest{Target: cancel, InvokeTarget: true})
	if !discovery.Proven() || len(discovery.Guards) != 2 {
		t.Fatalf("discovery=%+v", discovery)
	}
	for limit := 0; limit <= analysisproof.SummaryBudget; limit++ {
		query := &cancellationClassifier{cancel: cancel}
		pool := analysisproof.NewSearchBudget(10 * analysisproof.SummaryBudget)
		child := pool.Within(limit)
		proof := query.retainResultGuardsWithin(discovery.Guards, child)
		if child.Exhausted() || limit == 0 {
			if proof.State != analysisproof.EvidenceUnknown || query.guards != nil || pool.Exhausted() {
				t.Fatalf("cut%d=%+v guards%v", limit, proof, query.guards)
			}
			fresh := query.retainResultGuardsWithin(discovery.Guards, pool.Within(analysisproof.SummaryBudget))
			if !fresh.Proven() || len(query.guards) != 2 {
				t.Fatalf("fresh=%+v guards%v", fresh, query.guards)
			}
			continue
		}
		if !proof.Proven() || len(query.guards) != 2 {
			t.Fatalf("complete=%+v guards%v", proof, query.guards)
		}
		return
	}
	t.Fatal("filtered census never completed")
}
