package lifecycle

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
)

func TestCallbackCapabilityPoliciesAndAllowance(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 import "sync"
 func carry(cb func())func()
 func direct(mu *sync.Mutex)func(){return mu.Unlock}
 func literal(mu *sync.Mutex)func(){return func(){mu.Unlock()}}
 func mixed(mu *sync.Mutex,flag bool)func(){cb:=func(){};if flag{cb=mu.Unlock};return cb}
 func wrapped(mu *sync.Mutex)func(){return carry(mu.Unlock)}
 func boxed(mu *sync.Mutex)any{return mu.Unlock}
 func stored(mu *sync.Mutex)func(){var cb func();cb=mu.Unlock;defer func(){}();return cb}
 func different(mu,other *sync.Mutex)func(){return other.Unlock}
 func unknown(mu *sync.Mutex,cb func())func(){return cb}
 func recursive(mu *sync.Mutex)func(){var cb func();cb=func(){cb()};return cb}
 `)
	for _, test := range []struct {
		name string
		may  bool
	}{
		{"direct", true},
		{"literal", true},
		{"mixed", true},
		{"wrapped", true},
		{"boxed", true},
		{"stored", true},
		{"different", false},
		{"unknown", false},
		{"recursive", false},
	} {
		fn := pkg.Func(test.name)
		value := returnedValue(t, fn)
		proof := ProveValueCallsMethodWithin(value, "Unlock", fn.Params[0], proofs.NewSearchBudget(proofs.SummaryBudget))
		if proof.Proven() != test.may || ValueCallsMethod(value, "Unlock", fn.Params[0]) != test.may {
			t.Fatalf("%s policy: %+v", test.name, proof)
		}
		zero := proofs.NewSearchBudget(0)
		if proof := ProveValueCallsMethodWithin(value, "Unlock", fn.Params[0], zero); proof.Proven() ||
			proof.Reason != proofs.EvidenceBudgetExhausted || !zero.Exhausted() {
			t.Fatalf("%s zero allowance: %+v", test.name, proof)
		}
		if !test.may {
			continue
		}
		finished := false
		for limit := range proofs.SummaryBudget {
			pool := proofs.NewSearchBudget(100 * proofs.SummaryBudget)
			child := pool.Within(limit)
			proof := ProveValueCallsMethodWithin(value, "Unlock", fn.Params[0], child)
			if proof.Proven() {
				if child.Exhausted() || proof.Reason != proofs.EvidenceCallbackCompletion {
					t.Fatalf("%s proved interrupted capability: %+v", test.name, proof)
				}
				finished = true
				break
			}
			if proof.Reason != proofs.EvidenceBudgetExhausted || !child.Exhausted() || pool.Exhausted() {
				t.Fatalf("%s cut%d: %+v exhausted=%v/%v", test.name, limit, proof, child.Exhausted(), pool.Exhausted())
			}
			if !ProveValueCallsMethodWithin(value, "Unlock", fn.Params[0], pool.Within(proofs.SummaryBudget)).Proven() {
				t.Fatalf("%s fresh query fails", test.name)
			}
		}
		if !finished {
			t.Fatalf("%s capability never completes", test.name)
		}
	}
}

func TestCallbackCapabilityNestedCoverageChargesAllowance(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 import "sync"
 var total int
 func padded(mu *sync.Mutex,n int)func(){return func(){`+strings.Repeat("n+=n;total=n\n", 60)+`mu.Unlock()}}
 `)
	fn := pkg.Func("padded")
	value := returnedValue(t, fn)
	pool := proofs.NewSearchBudget(100 * proofs.SummaryBudget)
	child := pool.Within(30)
	proof := ProveValueCallsMethodWithin(value, "Unlock", fn.Params[0], child)
	if proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("nested coverage bypassed allowance: %+v exhausted=%v/%v", proof, child.Exhausted(), pool.Exhausted())
	}
	// Mapping the padded numeric capture now shares this allowance too.
	// The thirty-step cutoff remains the coverage control; recovery gets
	// a separate test allowance without changing any production limit.
	fresh := pool.Within(2 * proofs.SummaryBudget)
	if proof := ProveValueCallsMethodWithin(value, "Unlock", fn.Params[0], fresh); !proof.Proven() {
		t.Fatalf("fresh nested coverage=%+v, exhausted=%v/%v", proof, fresh.Exhausted(), pool.Exhausted())
	}
}

func TestCallbackOriginRevisitInvalidatesEnclosingMemo(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 import "sync"
 func callback(mu *sync.Mutex)func(){return mu.Unlock}
 `)
	fn := pkg.Func("callback")
	value, target := returnedValue(t, fn), fn.Params[0]
	search := newCompletionSearch("Unlock", CoverageEveryReturn, proofs.NewSearchBudget(proofs.SummaryBudget))
	if !search.valueCallsMethod(value, target) {
		t.Fatal("initial callback capability not found")
	}
	key := completionKey{target: target}
	attempts := 0
	for range 2 {
		answer := search.memo.Answer(key, func() completionAnswer {
			attempts++
			return completionAnswer{proven: search.valueCallsMethod(value, target)}
		})
		if answer.proven {
			t.Fatal("a revisited origin supplies new capability evidence")
		}
	}
	if attempts != 2 {
		t.Fatal("memo retained an answer shortened by the origin guard")
	}
}
