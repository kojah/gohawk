package ssaflow

import (
	"go/constant"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// Slice elements: the mechanics a caller needs to follow values into a slice
// and back out. An append adds values through a variadic array; the slice is
// a new SSA value after each append, and a loop merges those versions in a
// phi. A range statement over a slice lowers to an element loop: a counter
// that starts at -1, rises by one at the top of each iteration, and ends the
// loop once it reaches the slice's length, taken before the loop. When such a
// loop leaves only through that test, every iteration reads a different
// element and the iterations together read every element, whatever the
// length. This file recognizes these shapes and nothing more; whether a
// collection is owned, or what a loop body does with each element, is the
// caller's question.

// AppendedValues returns the values a call to append adds, when they are
// written as separate arguments, as append(s, a, b) is. SSA passes them in a
// fresh array; a spread slice, as in append(s, t...), is not followed.
func AppendedValues(call *ssa.Call) ([]ssa.Value, bool) {
	builtin, ok := call.Call.Value.(*ssa.Builtin)
	if !ok || builtin.Name() != "append" || len(call.Call.Args) != 2 {
		return nil, false
	}
	spread, ok := call.Call.Args[1].(*ssa.Slice)
	if !ok || len(*spread.Referrers()) != 1 {
		return nil, false
	}
	array, ok := spread.X.(*ssa.Alloc)
	if !ok || array.Comment != "varargs" {
		return nil, false
	}
	var values []ssa.Value
	for _, user := range *array.Referrers() {
		if user == spread {
			continue
		}
		address, ok := user.(*ssa.IndexAddr)
		if !ok {
			return nil, false
		}
		for _, write := range *address.Referrers() {
			store, ok := write.(*ssa.Store)
			if !ok || store.Addr != address {
				return nil, false
			}
			values = append(values, store.Val)
		}
	}
	return values, true
}

// SliceVersions returns every SSA value that is the same growing slice as
// start: start, the phis it flows into, and the results of appending to any
// of them, closed under both. The values are in the order they were found.
func SliceVersions(start ssa.Value) []ssa.Value {
	versions := []ssa.Value{start}
	member := map[ssa.Value]bool{start: true}
	for next := 0; next < len(versions); next++ {
		for _, user := range *versions[next].Referrers() {
			var version ssa.Value
			switch typed := user.(type) {
			case *ssa.Phi:
				version = typed
			case *ssa.Call:
				if _, ok := AppendedValues(typed); ok && typed.Call.Args[0] == versions[next] {
					version = typed
				}
			}
			if version != nil && !member[version] {
				member[version] = true
				versions = append(versions, version)
			}
		}
	}
	return versions
}

// ElementLoop is a range loop over a slice that leaves only through its
// header's length test.
type ElementLoop struct {
	Loop NaturalLoop
	// Body is the header's successor while elements remain; Done is the
	// successor once they run out, the loop's only exit.
	Body, Done *ssa.BasicBlock
	// Slice is the value whose length bounds the loop, and Index the
	// position of the element the current iteration reads.
	Slice, Index ssa.Value
}

// RangeElementLoop recognizes the element loop whose header is header. It
// declines a loop with another exit, such as a break or a return in the
// body, and a bound that is not the length of one slice value.
func RangeElementLoop(header *ssa.BasicBlock, budget *SearchBudget) (ElementLoop, bool) {
	if len(header.Instrs) == 0 || len(header.Succs) != 2 {
		return ElementLoop{}, false
	}
	branch, ok := header.Instrs[len(header.Instrs)-1].(*ssa.If)
	if !ok {
		return ElementLoop{}, false
	}
	test, ok := branch.Cond.(*ssa.BinOp)
	if !ok || test.Op != token.LSS || test.Block() != header {
		return ElementLoop{}, false
	}
	index, counter, ok := rangeCounter(test.X, header)
	if !ok {
		return ElementLoop{}, false
	}
	slice, ok := lengthOf(test.Y)
	if !ok {
		return ElementLoop{}, false
	}
	loop, ok := NaturalLoopAt(header, budget)
	if !ok || !loop.Contains(header.Succs[0]) || loop.Contains(header.Succs[1]) || !onlyHeaderExits(loop) {
		return ElementLoop{}, false
	}
	// The bound is fixed before the loop starts, and the counter starts
	// before the first element on every entry.
	if bound, ok := test.Y.(ssa.Instruction); !ok || loop.Contains(bound.Block()) || !counterStartsBeforeFirst(counter, index, loop) {
		return ElementLoop{}, false
	}
	return ElementLoop{Loop: loop, Body: header.Succs[0], Done: header.Succs[1], Slice: slice, Index: index}, true
}

// ReadsElement reports whether address is the address of the element the
// current iteration reads.
func (loop ElementLoop) ReadsElement(address *ssa.IndexAddr) bool {
	return address.X == loop.Slice && address.Index == loop.Index && loop.Loop.Contains(address.Block())
}

// onlyHeaderExits reports whether no block but the header leaves the loop:
// a break reaches the same block as the header's test, so the exits alone
// cannot tell the two apart.
func onlyHeaderExits(loop NaturalLoop) bool {
	for _, block := range loop.Blocks {
		if block == loop.Header {
			continue
		}
		for _, successor := range block.Succs {
			if !loop.Contains(successor) {
				return false
			}
		}
	}
	return true
}

// rangeCounter matches index = counter + 1, where counter is a phi in the
// header, and returns both.
func rangeCounter(value ssa.Value, header *ssa.BasicBlock) (ssa.Value, *ssa.Phi, bool) {
	step, ok := value.(*ssa.BinOp)
	if !ok || step.Op != token.ADD || step.Block() != header || !integerConstant(step.Y, 1) {
		return nil, nil, false
	}
	counter, ok := step.X.(*ssa.Phi)
	if !ok || counter.Block() != header {
		return nil, nil, false
	}
	return step, counter, true
}

// counterStartsBeforeFirst reports whether the counter is -1 on every edge
// into the loop and exactly the incremented index on every back edge.
func counterStartsBeforeFirst(counter *ssa.Phi, index ssa.Value, loop NaturalLoop) bool {
	for predecessor, edge := range PhiIncoming(counter) {
		if loop.Contains(predecessor) {
			if edge != index {
				return false
			}
			continue
		}
		if !integerConstant(edge, -1) {
			return false
		}
	}
	return true
}

// lengthOf returns the slice whose length value is.
func lengthOf(value ssa.Value) (ssa.Value, bool) {
	call, ok := value.(*ssa.Call)
	if !ok {
		return nil, false
	}
	builtin, ok := call.Call.Value.(*ssa.Builtin)
	if !ok || builtin.Name() != "len" || len(call.Call.Args) != 1 {
		return nil, false
	}
	return call.Call.Args[0], true
}

func integerConstant(value ssa.Value, want int64) bool {
	literal, ok := value.(*ssa.Const)
	if !ok || literal.Value == nil || literal.Value.Kind() != constant.Int {
		return false
	}
	got, exact := constant.Int64Val(literal.Value)
	return exact && got == want
}
