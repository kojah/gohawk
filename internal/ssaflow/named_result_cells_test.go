package ssaflow_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const namedCellFixture = `package namedcells
 type number int
 func named(flag bool)(n int, err error, ok bool){
  defer func(){println(n,err,ok)}()
  if flag{return 1,nil,true};return 2,nil,false
 }
 func duplicated()(int,int){n:=1;_ = func(){println(n)};return n,n}
 func swapped(flag bool)(int,int){a,b:=1,2;_ = func(){println(a,b)};if flag{return a,b};return b,a}
 func missing(flag bool)int{n:=1;_ = func(){println(n)};if flag{return n};return 2}
 func wrapped()number{n:=1;_ = func(){println(n)};return number(n)}
 func never()(n int){_ = func(){println(n)};for{}}
 func noResults(){n:=1;defer func(){println(n)}()}
`

func TestNamedResultCellCensus(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "namedcells", namedCellFixture)
	for _, test := range []struct {
		name  string
		count int
	}{
		{"named", 3},
		{"duplicated", 1},
		{"swapped", 0},
		{"missing", 0},
		{"wrapped", 0},
		{"never", 0},
		{"noResults", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			baseline := ssaflow.ProveNamedResultCellsWithin(fn, nil)
			if !baseline.Proven() || len(baseline.Cells) != test.count {
				t.Fatalf("baseline=%+v", baseline)
			}
			if test.name == "named" {
				assertNamedCellSlots(t, baseline, 3)
			}
			if test.name == "duplicated" {
				assertNamedCellSlots(t, baseline, 1)
			}
			assertNamedCellCutoffs(t, fn, baseline)
		})
	}
	foreign := ssaflow.InstructionsOf[*ssa.Alloc](pkg.Func("named"))[0]
	if _, found := ssaflow.ProveNamedResultCellsWithin(pkg.Func("duplicated"), nil).Cells[foreign]; found {
		t.Fatal("foreign cell identified")
	}
	if got := ssaflow.ProveNamedResultCellsWithin(nil, nil); got.Proven() || got.Cells != nil {
		t.Fatalf("missing function=%+v", got)
	}
}

func assertNamedCellSlots(t *testing.T, proof ssaflow.NamedResultCellsProof, count int) {
	t.Helper()
	slots := make(map[int]bool)
	for _, index := range proof.Cells {
		slots[index] = true
	}
	for index := range count {
		if !slots[index] {
			t.Fatalf("missing slot %d in %+v", index, proof)
		}
	}
}

func assertNamedCellCutoffs(t *testing.T, fn *ssa.Function, baseline ssaflow.NamedResultCellsProof) {
	t.Helper()
	pool := ssaflow.NewSearchBudget(100000)
	for limit := range ssaflow.SummaryBudget {
		budget := pool.Within(limit)
		got := ssaflow.ProveNamedResultCellsWithin(fn, budget)
		if budget.Exhausted() || budget.PoolExhausted() || limit == 0 {
			if got.Proven() || got.Reason != ssaflow.EvidenceBudgetExhausted || got.Cells != nil {
				t.Fatalf("cut%d=%+v", limit, got)
			}
		} else if !reflect.DeepEqual(got, baseline) {
			t.Fatalf("complete%d=%+v baseline=%+v", limit, got, baseline)
		}
		freshBudget := pool.Within(ssaflow.SummaryBudget)
		fresh := ssaflow.ProveNamedResultCellsWithin(fn, freshBudget)
		if freshBudget.Exhausted() || pool.Exhausted() || !reflect.DeepEqual(fresh, baseline) {
			t.Fatalf("fresh%d=%+v", limit, fresh)
		}
		if !budget.Exhausted() {
			return
		}
	}
	t.Fatal("census never completed")
}
