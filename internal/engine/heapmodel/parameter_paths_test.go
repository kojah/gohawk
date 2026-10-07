package heapmodel

import (
	"slices"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestParameterSpillPathsShareAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "spillpaths", `package spillpaths
 type box struct { first,second *int }
 func expose(*box)
 func direct(b *box)*int{return b.first}
 func spilled(b box)*int{return b.first}
 func copied(b box)*int{k:=b;return k.second}
 func indexed(b [2]*int)*int{return b[1]}
 func replaced(b box,p *int)*int{b.first=p;return b.first}
 func escaped(b box)*int{expose(&b);return b.first}
 func dynamic(b []*int,i int)*int{return b[i]}
 func unrelated(b box,p *int)*int{var k box;k.first=p;return k.first}
 `)
	for _, test := range []struct {
		name  string
		path  []string
		known bool
	}{
		{"direct", []string{"field:0"}, true},
		{"spilled", []string{"field:0"}, true},
		{"copied", []string{"field:1"}, true},
		{"indexed", []string{"index:1"}, true},
		{"replaced", nil, false},
		{"escaped", nil, false},
		{"dynamic", nil, false},
		{"unrelated", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
			baseline, known := AccessPathFromParameter(returned, fn.Params[0])
			if known != test.known || !slices.Equal(baseline, test.path) {
				t.Fatalf("default path=%v/%v", baseline, known)
			}
			if known {
				cut := proofs.NewSearchBudget(1)
				path, ok := AccessPathFromParameterWithin(returned, fn.Params[0], cut)
				if ok || path != nil || !cut.Exhausted() {
					t.Fatalf("path bypassed caller allowance: %v/%v", path, ok)
				}
			}
			for limit := 1; limit <= proofs.QueryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				path, ok := AccessPathFromParameterWithin(returned, fn.Params[0], budget)
				if budget.Exhausted() {
					if ok || path != nil {
						t.Fatalf("cut %d publishes path=%v/%v", limit, path, ok)
					}
					continue
				}
				if ok != known || !slices.Equal(path, baseline) {
					t.Fatalf("completed %d path=%v/%v", limit, path, ok)
				}
				return
			}
			t.Fatal("path query never completed")
		})
	}
}

func TestWholeWrittenSpillCellAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "spillcell", `package spillcell
 type box struct {first *int}
 func spilled(b box)*int{return b.first}
 `)
	fn := pkg.Func("spilled")
	cells := ssaflow.InstructionsOf[*ssa.Alloc](fn)
	if len(cells) != 1 {
		t.Fatalf("spill cell count=%d", len(cells))
	}
	pool := proofs.NewSearchBudget(proofs.QueryBudget)
	cut := pool.Within(1)
	if ssaflow.WholeWrittenCellWithin(cells[0], cut) || !cut.Exhausted() || pool.Exhausted() {
		t.Fatal("whole-cell cutoff not retained")
	}
	if !ssaflow.WholeWrittenCellWithin(cells[0], pool.Within(proofs.QueryBudget)) {
		t.Fatal("fresh whole-cell query did not recover")
	}
}

func TestSpillPathsRequireOriginalContentsAtRead(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "spillreplacement", `package spillreplacement
 type box struct {first *int; child *box}
 func replaced(b,c box)*int{b=c;return b.first}
 func earlier(b,c box)*int{p:=b.first;b=c;return p}
 func wrapped(b,c box)interface{}{p:=b.first;b=c;return p}
 func restoredAfterRead(b,c box)*int{original:=b;b=c;p:=b.first;b=original;return p}
 func nestedEarlier(b,c box)*int{child:=b.child;b=c;return child.first}
 func agreeing(b box,flag bool)*int{original:=b;if flag{b=original};return b.first}
 func ambiguous(b,c box,flag bool)*int{if flag{b=c};return b.first}
 func addressBefore(b,c box)**int{p:=&b.first;b=c;return p}
 `)
	for _, test := range []struct {
		name      string
		parameter int
		path      []string
		known     bool
	}{
		{"replaced", 0, nil, false},
		{"replaced", 1, []string{"field:0"}, true},
		{"earlier", 0, []string{"field:0"}, true},
		{"earlier", 1, nil, false},
		{"wrapped", 0, []string{"field:0"}, true},
		{"restoredAfterRead", 0, nil, false},
		{"restoredAfterRead", 1, []string{"field:0"}, true},
		{"nestedEarlier", 0, []string{"field:1", "field:0"}, true},
		{"agreeing", 0, []string{"field:0"}, true},
		{"ambiguous", 0, nil, false},
		{"ambiguous", 1, nil, false},
		{"addressBefore", 0, nil, false},
	} {
		fn := pkg.Func(test.name)
		value := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
		path, known := AccessPathFromParameterWithin(value, fn.Params[test.parameter], proofs.NewSearchBudget(proofs.QueryBudget))
		regionGraphs.Lock()
		_, built := regionGraphs.entries[fn]
		regionGraphs.Unlock()
		if built {
			t.Fatalf("%s path discovery built a graph", test.name)
		}
		if known != test.known || !slices.Equal(path, test.path) {
			t.Errorf("%s parameter%d path=%v/%v, want%v/%v", test.name, test.parameter, path, known, test.path, test.known)
		}
		completed := false
		for limit := 1; limit <= proofs.QueryBudget; limit++ {
			budget := proofs.NewSearchBudget(limit)
			path, known := AccessPathFromParameterWithin(value, fn.Params[test.parameter], budget)
			if budget.Exhausted() {
				if known || path != nil {
					t.Fatalf("%s cutoff %d published path %v", test.name, limit, path)
				}
				continue
			}
			if known != test.known || !slices.Equal(path, test.path) {
				t.Fatalf("%s fresh allowance %d path=%v/%v", test.name, limit, path, known)
			}
			completed = true
			break
		}
		if !completed {
			t.Fatalf("%s never completed within allowance", test.name)
		}
	}
}

func TestStrictParameterProjectionKeepsReadPath(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "strictparams", `package strictparams
 type box struct{first *int}
 func observe(interface{}){}
 func saved(b,c box){p:=b.first;b=c;observe(p)}
 func wrapped(b,c box){p:=b.first;b=c;var v interface{}=p;observe(v)}
 func replacement(b,c box){b=c;observe(b.first)}
 func ambiguous(b,c box,flag bool){if flag{b=c};observe(b.first)}
 func agreeing(b box,flag bool){original:=b;if flag{b=original};observe(b.first)}
 `)
	for _, test := range []struct {
		name   string
		proven bool
	}{
		{"saved", true},
		{"wrapped", true},
		{"replacement", false},
		{"ambiguous", false},
		{"agreeing", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			value := strictObservedValue(t, fn)
			sawCut := false
			for limit := 1; limit <= proofs.QueryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				proof := ProveStrictProjectionPathWithin(value, fn.Params[0], budget)
				if budget.Exhausted() {
					sawCut = true
					if proof.State != proofs.EvidenceUnknown || proof.Path != nil {
						t.Fatalf("cutoff %d published %+v", limit, proof)
					}
					continue
				}
				if !sawCut || proof.Proven() != test.proven {
					t.Fatalf("fresh allowance %d: %+v", limit, proof)
				}
				if test.proven && !slices.Equal(proof.Path, []string{"field:0"}) || !test.proven && proof.Path != nil {
					t.Fatalf("projection lost exact path: %+v", proof)
				}
				return
			}
			t.Fatal("projection did not recover")
		})
	}
}

func TestStrictProjectionPathAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "strictprobe", `package strictprobe
 type node struct { child *node; value *int; items []*int }
 func observe(*int){}
 func field(p *node){observe(p.child.value)}
 func dynamic(p *node,i int){observe(p.items[i])}
 func other(p,q *node){observe(q.value)}
 `)
	for _, test := range []struct {
		name string
		want bool
	}{{"field", true}, {"dynamic", false}, {"other", false}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			value := strictObservedValue(t, fn)
			if ProveStrictProjectionPathWithin(value, fn.Params[0], nil).Proven() != test.want {
				t.Fatal("default projection differs")
			}
			if test.want {
				cut := proofs.NewSearchBudget(1)
				proof := ProveStrictProjectionPathWithin(value, fn.Params[0], cut)
				if proof.State != proofs.EvidenceUnknown || !cut.Exhausted() {
					t.Fatalf("path bypassed caller allowance: %+v", proof)
				}
			}
			for allowance := 1; allowance <= proofs.QueryBudget; allowance++ {
				budget := proofs.NewSearchBudget(allowance)
				proof := ProveStrictProjectionPathWithin(value, fn.Params[0], budget)
				if budget.Exhausted() {
					if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted {
						t.Fatalf("cut %d: %+v", allowance, proof)
					}
					continue
				}
				if proof.State == proofs.EvidenceUnknown || proof.Proven() != test.want {
					t.Fatalf("complete %d: %+v", allowance, proof)
				}
				return
			}
			t.Fatal("small projection never completed")
		})
	}
}

func TestStrictProjectionRetainsChildCap(t *testing.T) {
	source := `package strictprobe
 type node struct { child *node; value *int }
 func observe(*int){}
 func deep(p *node){observe(p.` + strings.Repeat("child.", proofs.QueryBudget+1) + `value)}
 func shallow(p *node){observe(p.value)}
 `
	pkg := ssaflowtest.BuildPackage(t, "strictprobe", source)
	fn := pkg.Func("deep")
	pool := proofs.NewSearchBudget(10 * proofs.QueryBudget)
	proof := ProveStrictProjectionPathWithin(strictObservedValue(t, fn), fn.Params[0], pool)
	if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted || pool.Exhausted() {
		t.Fatalf("child cutoff=%+v, parent exhausted=%v", proof, pool.Exhausted())
	}
	fn = pkg.Func("shallow")
	if proof := ProveStrictProjectionPathWithin(strictObservedValue(t, fn), fn.Params[0], pool); !proof.Proven() {
		t.Fatalf("fresh query=%+v", proof)
	}
}

func strictObservedValue(t *testing.T, fn *ssa.Function) ssa.Value {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if ssaflow.CallName(call.Common()) == "observe" {
			return call.Common().Args[0]
		}
	}
	t.Fatal("observation not found")
	return nil
}
