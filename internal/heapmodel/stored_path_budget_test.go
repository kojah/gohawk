package heapmodel

import (
	"slices"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestStoredPathAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "pathallowance", `package pathallowance
type resource struct { n int }
type holder struct { value, other *resource }
type nested struct { inner holder }
type deep struct { inner nested }
func observe(any) {}
func exact(p, other *resource) { h := &holder{p,other}; observe(h) }
func replaced(p, other *resource) { h := &holder{p,other}; h.value = other; observe(h) }
func two(p, other *resource) { h := &nested{holder{p,other}}; observe(h) }
func three(p, other *resource) { h := &deep{nested{holder{p,other}}}; observe(h) }
`)
	for _, test := range []struct {
		name string
		path []string
	}{
		{"exact", []string{"field:0"}},
		{"replaced", nil},
		{"two", []string{"field:0", "field:0"}},
		{"three", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := heapObservation(t, fn)
			root := call.Common().Args[0]
			// observe's interface box is transparent to the graph but not to
			// the structural selection walk. Use its concrete source for both.
			root, _ = ssaflow.UnwrapTransparentValue(root, ssaflow.TransparentMakeInterface)
			baseline := ProveStoredPathWithin(root, fn.Params[0], call, nil)
			if baseline.Proven() != (test.path != nil) || !slices.Equal(baseline.Path, test.path) {
				t.Fatalf("default stored path = %+v, want %v", baseline, test.path)
			}
			for limit := 0; limit <= ssaflow.QueryBudget; limit++ {
				budget := ssaflow.NewSearchBudget(limit)
				got := ProveStoredPathWithin(root, fn.Params[0], call, budget)
				if limit == 0 || budget.Exhausted() || budget.PoolExhausted() {
					if got.Proven() || got.Reason != ssaflow.EvidenceBudgetExhausted {
						t.Fatalf("interrupted stored path = %+v", got)
					}
					continue
				}
				if got.State != baseline.State || got.Reason != baseline.Reason || !slices.Equal(got.Path, baseline.Path) {
					t.Fatalf("complete stored path = %+v, want %+v", got, baseline)
				}
				return
			}
			t.Fatal("stored path never completed")
		})
	}
}

func TestStoredPathStructuralChildCap(t *testing.T) {
	source := `package pathchild
type holder struct { value *int }
func noop() {}
func observe(*holder) {}
func caller(p *int) { h := &holder{p};
` + strings.Repeat("noop()\n", ssaflow.QueryBudget+100) + "observe(h) }"
	fn := ssaflowtest.BuildPackage(t, "pathchild", source).Func("caller")
	call := heapObservation(t, fn)
	root := call.Common().Args[0]
	pool := ssaflow.NewSearchBudget(10 * ssaflow.SummaryBudget)
	child := pool.Within(ssaflow.QueryBudget)
	query := storedPathQuery{budget: pool, storageBudget: child, storage: NewStorage(child), target: fn.Params[0], observation: call}
	proof := storedPathProof(query.walk(root, nil, 2), pool, child)
	if proof.Proven() || proof.Reason != ssaflow.EvidenceBudgetExhausted || pool.Exhausted() || pool.PoolExhausted() {
		t.Fatalf("structural storage child cutoff = %+v, parent exhausted %v", proof, pool.Exhausted())
	}
	fresh := ssaflow.NewSearchBudget(10 * ssaflow.SummaryBudget)
	query = storedPathQuery{budget: fresh, storageBudget: fresh, storage: NewStorage(fresh), target: fn.Params[0], observation: call}
	if path := query.walk(root, nil, 2); !slices.Equal(path, []string{"field:0"}) || fresh.Exhausted() {
		t.Fatalf("fresh structural path = %v, exhausted %v", path, fresh.Exhausted())
	}
}
