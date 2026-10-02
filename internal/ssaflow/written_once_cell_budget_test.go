package ssaflow_test

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestWrittenOnceCellAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "cells", writtenOnceCellFixture)
	for _, name := range []string{"readOnly", "storedTwice", "writtenInClosure", "writtenInNestedClosure", "addressTaken", "neverStored"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			cell := ssaflow.InstructionsOf[*ssa.Alloc](fn)[0]
			want, once := ssaflow.WrittenOnceCell(cell)
			completed := false
			for limit := 0; limit <= ssaflow.QueryBudget; limit++ {
				budget := ssaflow.NewSearchBudget(limit)
				got, ok := ssaflow.WrittenOnceCellWithin(cell, budget)
				if budget.Exhausted() || limit == 0 {
					if got != nil || ok {
						t.Fatalf("cut%d published stored value %v", limit, got)
					}
					continue
				}
				if got != want || ok != once {
					t.Fatalf("complete=%v/%v want%v/%v", got, ok, want, once)
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("cell census never completed")
			}
			pool := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
			if got, ok := ssaflow.WrittenOnceCellWithin(cell, pool.Within(1)); got != nil || ok || pool.Exhausted() {
				t.Fatalf("child=%v/%v pool exhausted=%v", got, ok, pool.Exhausted())
			}
			if got, ok := ssaflow.WrittenOnceCellWithin(cell, pool.Within(ssaflow.QueryBudget)); got != want || ok != once {
				t.Fatalf("fresh=%v/%v", got, ok)
			}
		})
	}
}

func TestWrittenOnceNestedReaderAllowance(t *testing.T) {
	source := `package manyreads
 func read(n int)func(){v:=n;return func(){` + strings.Repeat("println(v);", ssaflow.QueryBudget+1) + `}}
 `
	fn := ssaflowtest.BuildPackage(t, "manyreads", source).Func("read")
	cell := ssaflow.InstructionsOf[*ssa.Alloc](fn)[0]
	pool := ssaflow.NewSearchBudget(10 * ssaflow.SummaryBudget)
	child := pool.Within(ssaflow.QueryBudget)
	if stored, ok := ssaflow.WrittenOnceCellWithin(cell, child); stored != nil || ok || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("nested reader cut=%v/%v, child exhausted=%v, parent exhausted=%v", stored, ok, child.Exhausted(), pool.Exhausted())
	}
	fresh := pool.Within(2 * ssaflow.SummaryBudget)
	if stored, ok := ssaflow.WrittenOnceCellWithin(cell, fresh); !ok || stored != fn.Params[0] {
		t.Fatalf("fresh nested reader=%v/%v", stored, ok)
	}
}
