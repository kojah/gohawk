package lifecycle

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
)

func TestStoredCallbackCompletionAllowance(t *testing.T) {
	pkg := buildTestSSA(t, callbackBindingsFixture)
	for _, name := range []string{"field", "element", "dynamic"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			var dump strings.Builder
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			request := CompletionRequest{Instruction: findLaunch(t, function), Target: function.Params[0], Methods: []string{"Close"}}
			complete := ProveCompletion(request)
			if !complete.Proven() {
				t.Fatalf("complete callback %+v", complete)
			}
			for limit := range 1000 {
				pool := proofs.NewSearchBudget(10000)
				request.Budget = pool.Within(limit)
				proof := ProveCompletion(request)
				if request.Budget.Exhausted() {
					if proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted || pool.Exhausted() {
						t.Fatalf("cut%d supplied callback: %+v", limit, proof)
					}
					request.Budget = pool.Within(1000)
					if fresh := ProveCompletion(request); fresh != complete {
						t.Fatalf("fresh callback %+v want %+v", fresh, complete)
					}
					continue
				}
				if proof != complete {
					t.Fatalf("complete callback %+v want %+v", proof, complete)
				}
				return
			}
			t.Fatal("callback never completes")
		})
	}
}
