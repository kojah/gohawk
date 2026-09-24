package heapmodel

import (
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/ssa"
)

// A write-once field holds one value for the whole visible life of its
// object: it is set, if at all, while the object is being built, before any
// other code can see the object, and never again. A path through such a field
// then names the same object at every load, in every goroutine, so it can
// serve as a resource identity the way a parameter does.
//
// The proof is a closed-world inventory of one package and is deliberately
// narrow. The field must be unexported and declared in the package, so only
// the package can address it; tests are outside the inventory. Every address
// of the field may only be loaded, except by a store that initializes a fresh
// allocation before anything else uses that allocation, which is the shape of
// a composite literal. A store of a whole struct that holds the field by
// value, or a copy or append of such elements, overwrites it and disqualifies
// the field, again unless it initializes a fresh allocation. At least one
// initialization must exist: a field no code sets holds only its zero value,
// and a path through a nil pointer names no object. Another package
// can copy an exported struct wholesale, so the struct holding the field must
// be unexported and not held by value in an exported type, or must itself hold
// a sync primitive by value, which that primitive's documentation forbids
// copying after first use. Reflection and unsafe writes are outside the model.

// WriteOnceFields answers write-once queries for one package's fields.
type WriteOnceFields struct {
	pkg       *types.Package
	addresses map[*types.Var][]*ssa.FieldAddr
	owners    map[*types.Var]*types.Struct
	// wholeWrites are stores and builtin copies that may overwrite a struct
	// value in place, with the type they overwrite.
	wholeWrites []wholeWrite
	verdicts    map[*types.Var]bool
}

type wholeWrite struct {
	instruction ssa.Instruction
	written     types.Type
}

// NewWriteOnceFields indexes every field address and whole-value write in
// functions, which must be all of the package's production code.
func NewWriteOnceFields(pkg *types.Package, functions []*ssa.Function) *WriteOnceFields {
	fields := &WriteOnceFields{
		pkg: pkg, addresses: map[*types.Var][]*ssa.FieldAddr{}, owners: map[*types.Var]*types.Struct{}, verdicts: map[*types.Var]bool{},
	}
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				fields.record(instruction)
			}
		}
	}
	return fields
}

func (fields *WriteOnceFields) record(instruction ssa.Instruction) {
	switch instruction := instruction.(type) {
	case *ssa.FieldAddr:
		owner := addressedStruct(instruction.X.Type())
		if owner != nil {
			field := owner.Field(instruction.Field).Origin()
			fields.addresses[field] = append(fields.addresses[field], instruction)
			fields.owners[field] = owner
		}
	case *ssa.Store:
		fields.wholeWrites = append(fields.wholeWrites, wholeWrite{instruction, instruction.Val.Type()})
	case *ssa.Call:
		builtin, ok := instruction.Call.Value.(*ssa.Builtin)
		if ok && (builtin.Name() == "copy" || builtin.Name() == "append") && len(instruction.Call.Args) != 0 {
			// Both write slice elements in place.
			if slice, ok := instruction.Call.Args[0].Type().Underlying().(*types.Slice); ok {
				fields.wholeWrites = append(fields.wholeWrites, wholeWrite{instruction, slice.Elem()})
			}
		}
	}
}

// Fixed reports whether field is write-once. A nil receiver knows no fields.
func (fields *WriteOnceFields) Fixed(field *types.Var) bool {
	if fields == nil || field == nil {
		return false
	}
	field = field.Origin()
	verdict, known := fields.verdicts[field]
	if !known {
		verdict = fields.prove(field)
		fields.verdicts[field] = verdict
	}
	return verdict
}

func (fields *WriteOnceFields) prove(field *types.Var) bool {
	owner := fields.owners[field]
	if field.Exported() || field.Pkg() != fields.pkg || owner == nil || !fields.uncopiedElsewhere(owner) {
		return false
	}
	initialized := false
	for _, address := range fields.addresses[field] {
		for _, use := range *address.Referrers() {
			switch use := use.(type) {
			case *ssa.DebugRef:
			case *ssa.UnOp:
				if use.Op != token.MUL {
					return false
				}
			case *ssa.Store:
				if use.Addr != address || !initializesFresh(use) {
					return false
				}
				initialized = true
			default:
				return false
			}
		}
	}
	// A field no code sets holds only its zero value; a path through a nil
	// pointer ends in a panic, so it names no object.
	if !initialized {
		return false
	}
	for _, write := range fields.wholeWrites {
		if holdsByValue(write.written, owner) {
			store, isStore := write.instruction.(*ssa.Store)
			if !isStore || !initializesFresh(store) {
				return false
			}
		}
	}
	return true
}

// uncopiedElsewhere applies the cross-package copying rule to the struct that
// declares the field.
func (fields *WriteOnceFields) uncopiedElsewhere(owner *types.Struct) bool {
	if containsPrimitive(owner) {
		return true
	}
	named := fields.namedOwner(owner)
	if named == nil || named.Obj().Exported() {
		return false
	}
	scope := fields.pkg.Scope()
	for _, name := range scope.Names() {
		declared, ok := scope.Lookup(name).(*types.TypeName)
		if ok && declared.Exported() && holdsByValue(declared.Type(), owner) {
			return false
		}
	}
	return true
}

func (fields *WriteOnceFields) namedOwner(owner *types.Struct) *types.Named {
	scope := fields.pkg.Scope()
	for _, name := range scope.Names() {
		declared, ok := scope.Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		if named, ok := declared.Type().(*types.Named); ok && named.Underlying() == owner {
			return named
		}
	}
	return nil
}

// initializesFresh reports whether store writes into an allocation made in
// the same block, reached only through field selections, before any other
// instruction uses that allocation.
func initializesFresh(store *ssa.Store) bool {
	root := store.Addr
	for {
		field, ok := root.(*ssa.FieldAddr)
		if !ok {
			break
		}
		root = field.X
	}
	cell, ok := root.(*ssa.Alloc)
	if !ok || cell.Block() != store.Block() {
		return false
	}
	within := map[ssa.Value]bool{cell: true}
	started := false
	for _, instruction := range cell.Block().Instrs {
		if instruction == cell {
			started = true
			continue
		}
		if !started {
			continue
		}
		if instruction == store {
			return !within[store.Val]
		}
		switch instruction := instruction.(type) {
		case *ssa.FieldAddr:
			if within[instruction.X] {
				within[instruction] = true
			}
			continue
		case *ssa.Store:
			if within[instruction.Addr] && !within[instruction.Val] {
				continue
			}
		case *ssa.DebugRef:
			continue
		}
		for _, operand := range instruction.Operands(nil) {
			if within[*operand] {
				return false
			}
		}
	}
	return false
}

// holdsByValue reports whether a value of type value contains a struct of
// type owner without a pointer in between, so writing it overwrites owner.
func holdsByValue(value types.Type, owner *types.Struct) bool {
	switch value := value.Underlying().(type) {
	case *types.Struct:
		if types.Identical(value, owner) {
			return true
		}
		for field := range value.Fields() {
			if holdsByValue(field.Type(), owner) {
				return true
			}
		}
	case *types.Array:
		return holdsByValue(value.Elem(), owner)
	}
	return false
}

func containsPrimitive(value types.Type) bool {
	switch value := value.Underlying().(type) {
	case *types.Struct:
		for field := range value.Fields() {
			if syncPrimitive(field.Type()) || containsPrimitive(field.Type()) {
				return true
			}
		}
	case *types.Array:
		return containsPrimitive(value.Elem())
	}
	return false
}

func syncPrimitive(value types.Type) bool {
	named, ok := types.Unalias(value).(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "sync" {
		return false
	}
	return slices.Contains([]string{"Mutex", "RWMutex", "WaitGroup", "Once", "Cond"}, named.Obj().Name())
}

func addressedStruct(value types.Type) *types.Struct {
	pointer, ok := value.Underlying().(*types.Pointer)
	if !ok {
		return nil
	}
	structure, _ := pointer.Elem().Underlying().(*types.Struct)
	return structure
}
