package producerlifecycle

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// An unexported field of a struct declared in this package can only be read or
// written by this package, so scanning every function the package builds
// yields every use of a channel stored there. In a main package every field
// is closed this way, since the go command never lets another package import
// main. That closed world is the only evidence this file produces: which
// functions send on, receive from, and close the channel, and how it was
// made. Each check decides which of those facts it accepts. Reflection and
// unsafe can reach the field anyway; this analysis does not model them. A use
// it cannot classify at all makes the whole field unknown rather than
// guessing at the missing partner.

// ownedChannel is the package-wide inventory of one unexported channel field.
type ownedChannel struct {
	field *types.Var
	// unknown is the first use the inventory cannot classify: a store of
	// anything but a fresh channel, a by-value copy, or the channel or its
	// address escaping into other code.
	unknown  loopReason
	sends    []*ssa.Send
	receives []*ssa.Select
	// plainReceives are receives outside a select, including the receive
	// that drives a range loop.
	plainReceives []*ssa.UnOp
	// buffered records a fresh channel made with a nonzero or unknown size.
	buffered bool
	// closes lists every close, called or deferred.
	closes []ssa.CallInstruction
	// signalled records a send or close, in any form, so a stop arm that
	// receives from this field can actually fire.
	signalled bool
	closed    bool
	// signals lists every send, close, and signalling select on the field.
	signals []ssa.Instruction
}

// signalsHidden reports whether some signal may be outside the inventory.
func (owned *ownedChannel) signalsHidden() bool {
	switch owned.unknown {
	case loopReasonChannelEscapes, loopReasonChannelSupplied, loopReasonChannelCopied:
		return true
	default:
		return false
	}
}

// launches records how each function is started. A service loop must be
// started only by go statements; a synchronous call, a function value, or an
// interface method of the same name could run it in the caller's goroutine.
type launches struct {
	goOnly    map[*ssa.Function]bool
	sync      map[*ssa.Function]bool
	invoked   map[string]bool
	launchers map[*ssa.Function][]*ssa.Go
}

type channelInventory struct {
	fields map[*types.Var]*ownedChannel
	// addresses holds every address of every unexported field of a struct
	// declared in this package, which is the complete set of its accesses.
	addresses map[*types.Var][]*ssa.FieldAddr
	launches  launches
}

// newChannelInventory scans every function buildssa built for this package,
// including generated files and closures, because a use the scan skips would
// break the closed world. Test files are outside that world unless the
// test-file option includes them: the check is about production callers.
func newChannelInventory(pass *analysis.Pass) *channelInventory {
	inventory := &channelInventory{
		fields:    map[*types.Var]*ownedChannel{},
		addresses: map[*types.Var][]*ssa.FieldAddr{},
		launches: launches{
			goOnly: map[*ssa.Function]bool{}, sync: map[*ssa.Function]bool{},
			invoked: map[string]bool{}, launchers: map[*ssa.Function][]*ssa.Go{},
		},
	}
	for _, function := range ssaflow.PackageFunctions(pass) {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				inventory.launches.record(instruction)
				if address, ok := instruction.(*ssa.FieldAddr); ok {
					inventory.recordField(pass.Pkg, address)
				}
				inventory.recordValueCopy(pass.Pkg, instruction)
			}
		}
	}
	return inventory
}

func (owned *ownedChannel) mark(reason loopReason) {
	if owned.unknown == loopReasonNone {
		owned.unknown = reason
	}
}

func (inventory *channelInventory) channel(field *types.Var) *ownedChannel {
	if inventory.fields[field] == nil {
		inventory.fields[field] = &ownedChannel{field: field}
	}
	return inventory.fields[field]
}

func (inventory *channelInventory) recordField(pkg *types.Package, address *ssa.FieldAddr) {
	if field := ownedField(pkg, address); field != nil {
		inventory.addresses[field] = append(inventory.addresses[field], address)
	}
	if field := ownedChannelField(pkg, address); field != nil {
		inventory.channel(field).recordAddress(address)
	}
}

// ownedField returns the field an address selects when it is an unexported
// field of a non-generic struct declared in pkg.
func ownedField(pkg *types.Package, address *ssa.FieldAddr) *types.Var {
	structure, named := addressedStruct(address.X.Type())
	if structure == nil || named == nil || named.Obj().Pkg() != pkg || named.TypeParams().Len() != 0 {
		return nil
	}
	field := structure.Field(address.Field)
	if field.Exported() && pkg.Name() != "main" {
		return nil
	}
	return field
}

// ownedChannelField returns the field an address selects when it is an
// unexported channel field of a non-generic struct declared in pkg.
func ownedChannelField(pkg *types.Package, address *ssa.FieldAddr) *types.Var {
	field := ownedField(pkg, address)
	if field == nil {
		return nil
	}
	if _, channel := field.Type().Underlying().(*types.Chan); !channel {
		return nil
	}
	return field
}

func addressedStruct(pointerType types.Type) (*types.Struct, *types.Named) {
	pointer, ok := pointerType.Underlying().(*types.Pointer)
	if !ok {
		return nil, nil
	}
	named, _ := types.Unalias(pointer.Elem()).(*types.Named)
	structure, _ := pointer.Elem().Underlying().(*types.Struct)
	return structure, named
}

// recordValueCopy marks every channel field of a struct that is copied by
// value. A copied struct carries the channel somewhere the field scan cannot
// follow, including a whole-struct store that replaces the channel.
func (inventory *channelInventory) recordValueCopy(pkg *types.Package, instruction ssa.Instruction) {
	value, ok := instruction.(ssa.Value)
	if !ok {
		return
	}
	named, _ := types.Unalias(value.Type()).(*types.Named)
	structure, _ := value.Type().Underlying().(*types.Struct)
	if named == nil || structure == nil || named.Obj().Pkg() != pkg {
		return
	}
	for field := range structure.Fields() {
		if _, channel := field.Type().Underlying().(*types.Chan); channel && (!field.Exported() || pkg.Name() == "main") {
			inventory.channel(field).mark(loopReasonChannelCopied)
		}
	}
}

// recordAddress classifies every use of one field address.
func (owned *ownedChannel) recordAddress(address *ssa.FieldAddr) {
	for _, use := range *address.Referrers() {
		switch use := use.(type) {
		case *ssa.DebugRef:
		case *ssa.Store:
			owned.recordStore(address, use)
		case *ssa.UnOp:
			if use.Op != token.MUL {
				owned.mark(loopReasonChannelEscapes)
				continue
			}
			owned.recordLoad(use)
		default:
			owned.mark(loopReasonChannelEscapes)
		}
	}
}

// Only a fresh channel keeps the inventory closed: a supplied channel may have
// partners elsewhere. Whether it is buffered is recorded for each check.
func (owned *ownedChannel) recordStore(address *ssa.FieldAddr, store *ssa.Store) {
	made, ok := store.Val.(*ssa.MakeChan)
	if store.Addr != address {
		owned.mark(loopReasonChannelEscapes)
		return
	}
	if !ok {
		owned.mark(loopReasonChannelSupplied)
		return
	}
	size, constant := made.Size.(*ssa.Const)
	if !constant || size.Int64() != 0 {
		owned.buffered = true
	}
}

func (owned *ownedChannel) recordLoad(load *ssa.UnOp) {
	for _, use := range *load.Referrers() {
		switch use := use.(type) {
		case *ssa.DebugRef:
		case *ssa.Send:
			if use.X == load {
				owned.mark(loopReasonChannelEscapes)
				continue
			}
			owned.signalled = true
			owned.sends = append(owned.sends, use)
			owned.signals = append(owned.signals, use)
		case *ssa.UnOp:
			owned.plainReceives = append(owned.plainReceives, use)
		case *ssa.Select:
			owned.recordSelect(load, use)
		case *ssa.Call, *ssa.Defer:
			call := use.(ssa.CallInstruction)
			if !ssaflow.CallMatchesSymbol(call.Common(), syntax.Builtin("close")) {
				owned.mark(loopReasonChannelEscapes)
				continue
			}
			owned.closed, owned.signalled = true, true
			owned.closes = append(owned.closes, call)
			owned.signals = append(owned.signals, use)
		case *ssa.BinOp:
			// Comparing against nil neither transfers nor uses the channel.
			if !nilComparison(use) {
				owned.mark(loopReasonChannelEscapes)
			}
		default:
			owned.mark(loopReasonChannelEscapes)
		}
	}
}

func (owned *ownedChannel) recordSelect(load *ssa.UnOp, selected *ssa.Select) {
	for _, state := range selected.States {
		switch {
		case state.Send == load:
			owned.mark(loopReasonChannelEscapes)
		case state.Chan != load:
		case state.Dir == types.SendOnly:
			owned.signalled = true
			owned.signals = append(owned.signals, selected)
		default:
			owned.receives = append(owned.receives, selected)
		}
	}
}

func nilComparison(comparison *ssa.BinOp) bool {
	if comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false
	}
	for _, operand := range []ssa.Value{comparison.X, comparison.Y} {
		if constant, ok := operand.(*ssa.Const); ok && constant.IsNil() {
			return true
		}
	}
	return false
}

// record notes every way instruction can start or reference a function.
func (launches launches) record(instruction ssa.Instruction) {
	if launch, ok := instruction.(*ssa.Go); ok {
		if function := launchedFunction(launch.Common()); function != nil {
			launches.goOnly[function] = true
			launches.launchers[launch.Parent()] = append(launches.launchers[launch.Parent()], launch)
		}
	}
	if call := callCommon(instruction); call != nil && call.IsInvoke() {
		launches.invoked[call.Method.Name()] = true
	}
	for _, operand := range instruction.Operands(nil) {
		launches.recordReference(instruction, *operand)
	}
}

// recordReference marks a function that is used other than as the direct
// target of a go statement.
func (launches launches) recordReference(instruction ssa.Instruction, operand ssa.Value) {
	switch value := operand.(type) {
	case *ssa.Function:
		if launch, ok := instruction.(*ssa.Go); ok && launch.Call.Value == value {
			return
		}
		launches.sync[value] = true
		// A bound method value or wrapper runs its method synchronously.
		if value.Synthetic != "" && value.Object() != nil {
			launches.markObject(value.Object())
		}
	case *ssa.MakeClosure:
		if launch, ok := instruction.(*ssa.Go); ok && launch.Call.Value == value {
			return
		}
		if function, ok := value.Fn.(*ssa.Function); ok {
			launches.sync[function] = true
		}
	}
}

func (launches launches) markObject(object types.Object) {
	launches.invoked[object.Name()] = true
}

// startedOnlyByGo reports whether every reference to function starts it on a
// new goroutine. An exported function or method may also be called from
// another package, and an interface method of the same name may dispatch to
// it, so neither is a proven background loop.
func (launches launches) startedOnlyByGo(function *ssa.Function) bool {
	if function == nil || !launches.goOnly[function] || launches.sync[function] {
		return false
	}
	if object := function.Object(); object != nil && (ast.IsExported(object.Name()) || launches.invoked[object.Name()]) {
		return false
	}
	return true
}

func launchedFunction(call *ssa.CallCommon) *ssa.Function {
	switch value := call.Value.(type) {
	case *ssa.Function:
		return value
	case *ssa.MakeClosure:
		function, _ := value.Fn.(*ssa.Function)
		return function
	}
	return nil
}

func callCommon(instruction ssa.Instruction) *ssa.CallCommon {
	if call, ok := instruction.(ssa.CallInstruction); ok {
		return call.Common()
	}
	return nil
}
