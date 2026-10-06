package lifecycle

import (
	"reflect"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
)

const resultGuardBudgetFixture = resultGuardFixture + `
func closeOnSuccess(fail bool)(err error){
 f,err:=open();if err!=nil{return err}
 defer func(){if err==nil{f.Close()}}()
 if fail{return &failure{}};return nil
}
func closeOnBool()(ok bool){
 f,_:=open();defer func(){if ok{f.Close()}}();return true
}
func unrelated(fail bool)(err error){
 f,err:=open();if err!=nil{return err}
 defer func(){if fail{f.Close()}}();return nil
}
func multiple()(err error){
 f,err:=open();if err!=nil{return err}
 defer func(){if err!=nil{f.Close()}}()
 defer func(){if err==nil{f.Close()}}();return nil
}
func wrongCell()(err error){
 f,err:=open();if err!=nil{return err};other:=err
 defer func(){if other!=nil{f.Close()}}();return nil
}
func opaque(cleanup func())(err error){
 f,err:=open();if err!=nil{return err};_=f
 defer func(){if err!=nil{cleanup()}}();return nil
}
`

func TestResultGuardDiscoveryAllowance(t *testing.T) {
	pkg := buildTestSSA(t, resultGuardBudgetFixture)
	for _, test := range []struct {
		name  string
		count int
	}{
		{"closeOnError", 1},
		{"closeOnSuccess", 1},
		{"closeOnBool", 1},
		{"closeAlways", 0},
		{"unrelated", 0},
		{"multiple", 2},
		{"wrongCell", 0},
		{"opaque", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			request := CompletionRequest{Target: openedFile(t, fn), Methods: []string{"Close"}}
			baseline := ProveResultGuards(fn, request)
			if !baseline.Proven() || len(baseline.Guards) != test.count {
				t.Fatalf("default discovery = %+v", baseline)
			}
			for limit := range proofs.SummaryBudget {
				budget := proofs.NewSearchBudget(limit)
				request.Budget = budget
				got := ProveResultGuards(fn, request)
				if budget.Exhausted() || budget.PoolExhausted() || limit == 0 {
					if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted || got.Guards != nil {
						t.Fatalf("cut %d published guard census: %+v", limit, got)
					}
					continue
				}
				if !reflect.DeepEqual(got, baseline) {
					t.Fatalf("complete discovery = %+v, want %+v", got, baseline)
				}
				return
			}
			t.Fatal("discovery never completed")
		})
	}
}

func TestResultGuardDiscoveryChildAndFresh(t *testing.T) {
	fn := buildTestSSA(t, resultGuardBudgetFixture).Func("multiple")
	pool := proofs.NewSearchBudget(10000)
	request := CompletionRequest{Target: openedFile(t, fn), Methods: []string{"Close"}, Budget: pool.Within(5)}
	if got := ProveResultGuards(fn, request); got.State != proofs.EvidenceUnknown || got.Guards != nil || pool.Exhausted() {
		t.Fatalf("child discovery = %+v, parent exhausted %v", got, pool.Exhausted())
	}
	request.Budget = pool.Within(proofs.SummaryBudget)
	if got := ProveResultGuards(fn, request); !got.Proven() || len(got.Guards) != 2 {
		t.Fatalf("fresh discovery = %+v", got)
	}
}

func TestResultGuardDiscoveryPartialList(t *testing.T) {
	fn := buildTestSSA(t, resultGuardBudgetFixture).Func("multiple")
	for limit := range proofs.SummaryBudget {
		budget := proofs.NewSearchBudget(limit)
		got := ProveResultGuards(fn, CompletionRequest{Target: openedFile(t, fn), Methods: []string{"Close"}, Budget: budget})
		if budget.Exhausted() || budget.PoolExhausted() {
			if len(got.Guards) > 0 {
				t.Fatalf("cut %d published %d partial guards", limit, len(got.Guards))
			}
			continue
		}
		if !got.Proven() || len(got.Guards) != 2 {
			t.Fatalf("complete multi-guard census = %+v", got)
		}
		return
	}
	t.Fatal("multi-guard discovery never completed")
}
