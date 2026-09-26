package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestRangeElementLoop(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "elements", `package elements

func each(items []int) (total int) {
	for _, item := range items {
		total += item
	}
	return total
}

func indexed(items []int) (total int) {
	for i := range items {
		total += items[i]
	}
	return total
}

func breaks(items []int) (total int) {
	for _, item := range items {
		if item < 0 {
			break
		}
		total += item
	}
	return total
}

func returns(items []int) (total int) {
	for _, item := range items {
		if item < 0 {
			return -1
		}
		total += item
	}
	return total
}

func skipsFirst(items []int) (total int) {
	for i := 1; i < len(items); i++ {
		total += items[i]
	}
	return total
}

func counted(items []int) (total int) {
	for i := 0; i < 3; i++ {
		total += i
	}
	return total
}
`)
	for name, want := range map[string]bool{"each": true, "indexed": true, "breaks": false, "returns": false, "skipsFirst": false, "counted": false} {
		function := pkg.Func(name)
		found := false
		for _, block := range function.Blocks {
			loop, ok := ssaflow.RangeElementLoop(block, ssaflow.NewSearchBudget(ssaflow.QueryBudget))
			if !ok {
				continue
			}
			found = true
			if loop.Slice != function.Params[0] {
				t.Errorf("%s: loop ranges over %v, want the parameter", name, loop.Slice)
			}
			if !readsElementIn(loop) {
				t.Errorf("%s: no element read matched the loop index", name)
			}
		}
		if found != want {
			t.Errorf("%s: RangeElementLoop found = %v, want %v", name, found, want)
		}
	}
}

func readsElementIn(loop ssaflow.ElementLoop) bool {
	for _, block := range loop.Loop.Blocks {
		for _, instruction := range block.Instrs {
			if address, ok := instruction.(*ssa.IndexAddr); ok && loop.ReadsElement(address) {
				return true
			}
		}
	}
	return false
}

func TestAppendedValuesAndSliceVersions(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "versions", `package versions

func grow(items []int, extra []int) []int {
	var all []int
	for _, item := range items {
		all = append(all, item, item+1)
	}
	all = append(all, extra...)
	return all
}
`)
	function := pkg.Func("grow")
	var separate, spread *ssa.Call
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok {
				continue
			}
			if builtin, ok := call.Call.Value.(*ssa.Builtin); !ok || builtin.Name() != "append" {
				continue
			}
			if _, ok := call.Call.Args[1].(*ssa.Slice); ok {
				separate = call
			} else {
				spread = call
			}
		}
	}
	if separate == nil || spread == nil {
		t.Fatalf("appends not found: separate=%v spread=%v", separate, spread)
	}
	values, ok := ssaflow.AppendedValues(separate)
	if !ok || len(values) != 2 {
		t.Errorf("AppendedValues(%v) = %v, %v; want the two separate arguments", separate, values, ok)
	}
	if _, ok := ssaflow.AppendedValues(spread); ok {
		t.Errorf("AppendedValues(%v) followed a spread slice", spread)
	}
	versions := ssaflow.SliceVersions(separate)
	if len(versions) < 2 {
		t.Fatalf("SliceVersions = %v, want the append and the loop phi", versions)
	}
	if _, ok := versions[1].(*ssa.Phi); !ok {
		t.Errorf("second version = %v, want the loop phi", versions[1])
	}
	for _, version := range versions {
		if version == ssa.Value(spread) {
			t.Errorf("SliceVersions followed a spread append, which is not a separate-argument append: %v", versions)
		}
	}
}
