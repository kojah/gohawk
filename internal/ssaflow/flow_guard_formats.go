package ssaflow

import "golang.org/x/tools/go/ssa"

// Guard formatting is immutable during one SSA walk. The memo saves only
// rendered loaded-condition text: callers still resolve the address and charge
// every visit before using it. Unresolved addresses never populate the memo.
type guardFormats struct {
	loaded map[ssa.Value]string
}

func (formats *guardFormats) loadedIdentity(condition ssa.Value, address string, literal *ssa.Const, resolved bool) string {
	if formats != nil && resolved {
		if identity, ok := formats.loaded[condition]; ok {
			return identity
		}
	}
	var identity string
	if literal == nil {
		identity = "load(" + address + ")"
	} else {
		identity = "eq(load(" + address + ")," + guardOperandIdentity(literal) + ")"
	}
	if formats != nil && resolved {
		if formats.loaded == nil {
			formats.loaded = make(map[ssa.Value]string)
		}
		formats.loaded[condition] = identity
	}
	return identity
}
