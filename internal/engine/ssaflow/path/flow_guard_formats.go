package path

import (
	"fmt"

	"golang.org/x/tools/go/ssa"
)

// Guard formatting is immutable during one SSA walk. The memo saves only
// rendered address and loaded-condition text: callers still resolve addresses
// and charge every visit before using it. Unresolved addresses never populate
// the memo.
type guardFormats struct {
	loaded    map[ssa.Value]string
	addresses map[ssa.Value]string
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

// addressIdentity renders only after the caller traverses and charges the full
// address path. A warm entry must not turn an interrupted resolution into proof.
func (formats *guardFormats) addressIdentity(address ssa.Value, inner string, resolved bool) string {
	if formats != nil && resolved {
		if identity, ok := formats.addresses[address]; ok {
			return identity
		}
	}
	identity := guardAddressString(address, inner)
	if formats != nil && resolved {
		if formats.addresses == nil {
			formats.addresses = make(map[ssa.Value]string)
		}
		formats.addresses[address] = identity
	}
	return identity
}

func guardAddressString(address ssa.Value, inner string) string {
	switch typed := address.(type) {
	case *ssa.Alloc:
		return fmt.Sprintf("alloc:%p", typed)
	case *ssa.Parameter:
		return fmt.Sprintf("param:%p", typed)
	case *ssa.FreeVar:
		return fmt.Sprintf("free:%p", typed)
	case *ssa.Global:
		return "global:" + typed.String()
	case *ssa.FieldAddr:
		return fmt.Sprintf("field(%s,%d)", inner, typed.Field)
	case *ssa.UnOp:
		return "load(" + inner + ")"
	case *ssa.Call, *ssa.Extract:
		return guardOperandIdentity(typed)
	}
	return ""
}
