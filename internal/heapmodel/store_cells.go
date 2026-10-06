package heapmodel

import (
	"go/token"
	"go/types"
	"slices"
	"strings"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Cell queries resolve occupants across writes and observations, including
// deferred invocation and lifetime stability. Each API states whether it needs
// point-in-time contents or a whole-lifetime guarantee; cutoff proves neither.

// A backwards query stops at the first write on each predecessor path. Earlier
// assignments cannot defeat an unconditional overwrite, and joins retain only
// one agreed value. Cycles and missing initializations remain unknown.
func (storage *Storage) reachingContent(location storageLocation, observation ssa.Instruction, stores []*ssa.Store) StoredValue {
	writes := make(map[*ssa.Store]storageWrite)
	for _, store := range stores {
		written, ok := storage.location(store.Addr)
		if !ok || written.root != location.root {
			return storage.unknown(proofs.EvidenceStorageWriteThroughAlias, store)
		}
		if slotBeneath(location.path, written.path) {
			writes[store] = storageWrite{suffix: strings.TrimPrefix(location.path, written.path)}
		} else if slotBeneath(written.path, location.path) {
			writes[store] = storageWrite{partial: true}
		}
	}
	// A sole dominating initializer remains valid across unrelated loops.
	// There is no competing write to discover by walking their backedges.
	// A callback defined outside a range loop can therefore be bound inside it:
	// https://github.com/mariadb-operator/mariadb-operator/blob/e8ece7a8076954674e10e0381571bd80278ac35f/licenses/go-licenses/github.com/go-sql-driver/mysql/driver_test.go#L191-L205
	if store, write, only := soleStorageWrite(writes); only && !write.partial {
		dominates := cfg.InstructionDominatesWithin(store, observation, storage.budget)
		if storage.budget.Exhausted() {
			return storage.unknown(proofs.EvidenceBudgetExhausted, observation)
		}
		if dominates {
			return storage.projectStored(store.Val, write.suffix)
		}
	}
	index := cfg.InstructionIndexWithin(observation, storage.budget)
	if index < 0 {
		return storage.unknown(proofs.EvidenceStorageNoReachingWrite, observation)
	}
	query := reachingStorage{storage: storage, location: location, writes: writes, active: make(map[*ssa.BasicBlock]bool)}
	return query.before(observation.Block(), index)
}

func soleStorageWrite(writes map[*ssa.Store]storageWrite) (*ssa.Store, storageWrite, bool) {
	if len(writes) == 1 {
		for store, write := range writes {
			return store, write, true
		}
	}
	return nil, storageWrite{}, false
}

// A field write invalidates a whole-aggregate value. It cannot be mistaken for
// that aggregate's previous initializer, nor for a write to a sibling field.
type storageWrite struct {
	suffix  string
	partial bool
}

type reachingStorage struct {
	storage  *Storage
	location storageLocation
	writes   map[*ssa.Store]storageWrite
	active   map[*ssa.BasicBlock]bool
}

func (query *reachingStorage) before(block *ssa.BasicBlock, index int) StoredValue {
	if block == nil {
		return query.storage.unknown(proofs.EvidenceStorageNoReachingWrite, nil)
	}
	if query.active[block] {
		return query.storage.unknown(proofs.EvidenceStorageWriteInCycle, block.Instrs[len(block.Instrs)-1])
	}
	if !query.storage.budget.Spend() {
		return query.storage.unknown(proofs.EvidenceBudgetExhausted, nil)
	}
	query.active[block] = true
	defer delete(query.active, block)
	for i := index - 1; i >= 0; i-- {
		if !query.storage.budget.Spend() {
			return query.storage.unknown(proofs.EvidenceBudgetExhausted, nil)
		}
		if block.Instrs[i] == query.location.root {
			return query.storage.unknown(proofs.EvidenceStorageNoReachingWrite, query.location.root)
		}
		if store, ok := block.Instrs[i].(*ssa.Store); ok {
			if write, relevant := query.writes[store]; relevant {
				if write.partial {
					return query.storage.unknown(proofs.EvidenceStoragePartialWrite, store)
				}
				return query.storage.projectStored(store.Val, write.suffix)
			}
		}
	}
	var agreed StoredValue
	for _, predecessor := range block.Preds {
		incoming := query.before(predecessor, len(predecessor.Instrs))
		if !incoming.Proven() {
			return incoming
		}
		if agreed.Proven() && !query.storage.Same(agreed.Value, incoming.Value).Proven() {
			return query.storage.unknown(proofs.EvidenceStorageConflictingWrites, predecessor.Instrs[len(predecessor.Instrs)-1])
		}
		agreed = incoming
	}
	if !agreed.Proven() {
		return query.storage.unknown(proofs.EvidenceStorageNoReachingWrite, nil)
	}
	return agreed
}

// Deferred-cell relations require complete observation and occupant evidence.
// Request allowance covers selection, reachability and relation visits; graph
// construction and state replay retain their independent costs. An interrupted
// relation is unavailable, so completion cannot fall back to another mapping.

// DeferredCellMatch distinguishes a captured cell that contains exactly the
// target from one whose every possible occupant contains it indirectly.
type DeferredCellMatch uint8

const (
	DeferredCellUnknown DeferredCellMatch = iota
	DeferredCellExact
	DeferredCellContains
)

// DeferredCellRelationWithin reads the cell when deferred calls execute. Known is
// false when either side could not be read; callers must not use a fallback
// proof in that case. A stale or unrelated occupant prevents an exact claim.
// Census, reachability, union and history visits share budget; cutoff publishes
// no relation. Graph construction/replay and points-to internals retain separate
// costs. A nil budget retains the unbounded observation policy.
func DeferredCellRelationWithin(cell *ssa.Alloc, target ssa.Value, invocation ssa.Instruction, budget *proofs.SearchBudget) (DeferredCellMatch, bool) {
	if !budget.Spend() {
		return DeferredCellUnknown, false
	}
	graph := regionsOf(cell)
	held, ok := graph.contentWhenDeferredRunWithin(cell, invocation, budget)
	if !ok || len(held) == 0 {
		return DeferredCellUnknown, false
	}
	object, ok := graph.pointsTo(target)
	if !ok {
		return DeferredCellUnknown, false
	}
	targetSlot, exact := singleSlot(object)
	isTarget, contains := exact, true
	for entry, stale := range held {
		if !budget.Spend() {
			return DeferredCellUnknown, false
		}
		if entry.region.kind == regionNil {
			continue
		}
		if entry != targetSlot || stale {
			isTarget = false
		}
		if !graph.everContainedWithin(entry, object, budget) {
			contains = false
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return DeferredCellUnknown, false
	}
	if isTarget {
		return DeferredCellExact, true
	}
	if contains {
		return DeferredCellContains, true
	}
	return DeferredCellUnknown, true
}

// contentWhenDeferredRunWithin returns what the addressed slots hold when the
// function's deferred calls run: the union over every RunDefers the
// registration can reach, read before the deferred calls' own effects, or
// over every reachable return when the function defers nothing and the
// callback was registered with a test instead. A deferred literal observes
// its captured cell then, not at the registration.
func (graph *regionGraph) contentWhenDeferredRunWithin(address ssa.Value, registration ssa.Instruction, budget *proofs.SearchBudget) (pointees, bool) {
	defer graph.lock()()
	if !graph.available || registration == nil {
		return nil, false
	}
	points, available := deferredObservationPoints(graph.function, budget)
	if !available {
		return nil, false
	}
	result := pointees{}
	found := false
	for _, point := range points {
		if !cfg.InstructionMayFollowWithin(registration, point, budget) {
			continue
		}
		set, ok := graph.contentAtUnlocked(address, point)
		if !ok {
			return nil, false
		}
		for target, stale := range set {
			if !budget.Spend() {
				return nil, false
			}
			result.add(target, stale)
		}
		found = true
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return nil, false
	}
	return result, found
}

// deferredObservationPoints completes one shared census before selecting
// RunDefers or, when none exist, returns for test-registered callbacks. A prefix
// cannot establish that a later deferred execution point or return is absent.
func deferredObservationPoints(function *ssa.Function, budget *proofs.SearchBudget) ([]ssa.Instruction, bool) {
	var runs, returns []ssa.Instruction
	for instruction := range ssaflow.InstructionsWithin(function, budget) {
		switch instruction.(type) {
		case *ssa.RunDefers:
			runs = append(runs, instruction)
		case *ssa.Return:
			returns = append(returns, instruction)
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return nil, false
	}
	if len(runs) != 0 {
		return runs, true
	}
	return returns, true
}

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
		owner := syntax.PointerStruct(instruction.X.Type())
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
	return anyByValueType(value, func(value types.Type) bool {
		structure, ok := value.Underlying().(*types.Struct)
		return ok && types.Identical(structure, owner)
	})
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

// StableFieldContent proves the contents of a fresh owner's embedded field
// remain unchanged through every visible use, including observation itself.
// Unlike StableContent, unrelated sibling fields are outside the question.
// Known asynchronous readers may read this slot but must not retain its
// address or write it. This says nothing about mutation of the loaded object.
func (storage *Storage) StableFieldContent(address ssa.Value, observation ssa.Instruction) StoredValue {
	path, known := ssaflow.ResolveEmbeddedFieldPath(ssaflow.NewReachingWalk(ssaflow.TransparentNone).Within(storage.budget), address,
		func(root ssa.Value) bool { _, fresh := root.(*ssa.Alloc); return fresh })
	location, local := storage.location(address)
	if !known || path.Depth == 0 || !local || observation == nil || path.Root.Parent() != observation.Parent() {
		return storage.unknown(proofs.EvidenceStorageNotLocal, observation)
	}
	var stores []*ssa.Store
	if blocked, ok := storage.collectField(path, &stores); !ok {
		return storage.unknown(proofs.EvidenceStorageAddressEscapes, blocked)
	}
	for _, store := range stores {
		written, exact := storage.location(store.Addr)
		if !exact || !slotBeneath(location.path, written.path) {
			return storage.unknown(proofs.EvidenceStoragePartialWrite, store)
		}
		follows := StoreMayFollowWithin(location.root, observation, store, storage.budget)
		if storage.budget.Exhausted() || storage.budget.PoolExhausted() {
			return storage.unknown(proofs.EvidenceBudgetExhausted, store)
		}
		if store == observation || follows || cfg.BlockInCycle(store.Block()) {
			return storage.unknown(proofs.EvidenceStorageWriteAfterObservation, store)
		}
	}
	return storage.reachingContent(location, observation, stores)
}

// Inspect all address uses, not only those before observation: a worker may
// read later. The field-effect query preserves whole-owner escapes and writes
// while proving that operations through a different embedded field are disjoint.
func (storage *Storage) collectField(path ssaflow.EmbeddedFieldPath, stores *[]*ssa.Store) (ssa.Instruction, bool) {
	if path.Root.Referrers() == nil {
		return nil, false
	}
	for _, use := range *path.Root.Referrers() {
		if !storage.budget.Spend() {
			return use, false
		}
		switch use := use.(type) {
		case *ssa.FieldAddr:
			if path.Depth == 0 {
				return use, false
			}
			if use.Field != path.Fields[0] {
				continue
			}
			child := ssaflow.EmbeddedFieldPath{Root: use, Depth: path.Depth - 1}
			copy(child.Fields[:], path.Fields[1:path.Depth])
			if blocked, ok := storage.collectField(child, stores); !ok {
				return blocked, false
			}
		case *ssa.Store:
			if use.Addr != path.Root {
				return use, false
			}
			*stores = append(*stores, use)
		default:
			if !storage.readOnlyFieldUse(path, use) {
				return use, false
			}
		}
	}
	return nil, true
}

func (storage *Storage) readOnlyFieldUse(path ssaflow.EmbeddedFieldPath, use ssa.Instruction) bool {
	switch use := use.(type) {
	case *ssa.DebugRef:
		return true
	case *ssa.UnOp:
		return use.Op == token.MUL
	case *ssa.Call, *ssa.Go, *ssa.Defer:
		return storage.effects.FieldCall(use, path).PreservesField()
	}
	return false
}
