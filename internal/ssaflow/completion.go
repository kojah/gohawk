package ssaflow

import (
	"slices"

	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

// Completion evidence proves that the callee an instruction launches calls a
// lifecycle method on the caller's target. There is one search: resolve the
// callee, map the caller's target onto the callee's parameters and captured
// variables, and check how much of the callee's control flow a matching call
// covers. Every launch form demands the same coverage: the callee must call
// before each of its normal returns, on the paths feasible when the mapped
// local is non-nil. A callee that only conditionally releases proves nothing,
// whether it is deferred, called, or started. Nested launches inside the
// callee are proved by the same search, so helper chains, deferred callbacks,
// cleanup registrations, and launched waiters need no separate rule.

// CompletionCoverage selects how much of a callee's control flow a lifecycle
// call must cover before it counts as completion.
type CompletionCoverage uint8

const (
	// CoverageEveryReturn, the default, accepts a call that precedes every
	// normal return; a callee that never returns or never calls proves
	// nothing. It answers whether the callee must complete the target.
	CoverageEveryReturn CompletionCoverage = iota
	// CoverageAnywhere accepts a call on any path. It answers only whether
	// the callee may complete the target, which suppresses a diagnostic that
	// would otherwise assume the obligation is still open.
	CoverageAnywhere
)

// MethodCallCoverage reports whether calls holds over function's normal paths
// with the requested coverage. nonNil, when set, restricts every-return
// analysis to paths feasible when that value is non-nil at entry.
func MethodCallCoverage(function *ssa.Function, calls func(ssa.Instruction) bool, coverage CompletionCoverage, nonNil ssa.Value) bool {
	if function == nil || len(function.Blocks) == 0 {
		return false
	}
	switch coverage {
	case CoverageEveryReturn:
		hasReturn, hasCall := false, false
		for _, block := range function.Blocks {
			for _, candidate := range block.Instrs {
				if _, ok := candidate.(*ssa.Return); ok {
					hasReturn = true
				}
				hasCall = hasCall || calls(candidate)
			}
		}
		return hasReturn && hasCall && !unownedReturnFromEntry(function, calls, nil, nonNil)
	case CoverageAnywhere:
	}
	return slices.ContainsFunc(function.Blocks, func(block *ssa.BasicBlock) bool {
		return slices.ContainsFunc(block.Instrs, calls)
	})
}

// launchKind is how an instruction runs its callee.
type launchKind uint8

const (
	launchNone launchKind = iota
	// launchDeferred covers defer statements and testing Cleanup
	// registrations: the callee runs when the parent returns.
	launchDeferred
	// launchCalled is a synchronous call whose callee returns to the parent.
	launchCalled
	// launchStarted covers go statements and sync.WaitGroup.Go: the callee
	// runs on its own goroutine.
	launchStarted
	// launchCallback is a function literal examined as a value, without any
	// launch; when it runs is the caller's concern. It is never resolved from
	// an instruction, so a literal that is merely created or stored proves
	// nothing on its own.
	launchCallback
)

func (launch launchKind) reason() EvidenceReason {
	switch launch {
	case launchDeferred:
		return EvidenceDeferredCompletion
	case launchCalled:
		return EvidenceCalledCompletion
	case launchStarted:
		return EvidenceStartedCompletion
	case launchCallback:
		return EvidenceCallbackCompletion
	case launchNone:
	}
	return EvidenceNone
}

// completionCallee is one resolved body to search. A callee reached through a
// closure value maps the caller's target through its bindings; a callee
// reached through a call also maps it through the call's arguments.
type completionCallee struct {
	launch   launchKind
	common   *ssa.CallCommon
	closure  *ssa.MakeClosure
	function *ssa.Function
}

var waitGroupGoMethod = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Go"})

// resolveCallees returns every body the instruction may run, or ok=false when
// some path reaches a callee the analysis cannot see. A deferred callback may
// be resolved through the documented sync.OnceFunc contract; a callback
// invoked now may not, because an earlier invocation could already have
// consumed the wrapper.
func resolveCallees(instruction ssa.Instruction) ([]completionCallee, bool) {
	switch typed := instruction.(type) {
	case *ssa.Defer:
		return calleesOf(typed.Common(), launchDeferred, instruction, true)
	case *ssa.Go:
		return calleesOf(typed.Common(), launchStarted, instruction, false)
	case *ssa.Call:
		common := typed.Common()
		if CallMatchesSymbol(common, waitGroupGoMethod) && len(common.Args) == 2 {
			return closureCallees(common.Args[1], launchStarted)
		}
		if HasLibraryContract(common, ContractTestingCleanup) && len(common.Args) > 0 {
			// testing.TB guarantees that Cleanup callbacks run when the test
			// and its subtests complete, so a registered callback is deferred.
			return closureCallees(common.Args[len(common.Args)-1], launchDeferred)
		}
		return calleesOf(common, launchCalled, instruction, false)
	}
	return nil, false
}

func calleesOf(common *ssa.CallCommon, launch launchKind, invocation ssa.Instruction, allowOnceFunc bool) ([]completionCallee, bool) {
	if closure, ok := common.Value.(*ssa.MakeClosure); ok {
		function, _ := closure.Fn.(*ssa.Function)
		return []completionCallee{{launch: launch, common: common, closure: closure, function: function}}, function != nil
	}
	if function := common.StaticCallee(); function != nil {
		return []completionCallee{{launch: launch, common: common, function: function}}, true
	}
	closures, ok := exactCallbacks(common.Value, invocation, allowOnceFunc, map[ssa.Value]bool{})
	if !ok {
		return nil, false
	}
	result := make([]completionCallee, 0, len(closures))
	for _, closure := range closures {
		function, _ := closure.Fn.(*ssa.Function)
		if function == nil {
			return nil, false
		}
		result = append(result, completionCallee{launch: launch, common: common, closure: closure, function: function})
	}
	return result, true
}

func closureCallees(value ssa.Value, launch launchKind) ([]completionCallee, bool) {
	closure, ok := value.(*ssa.MakeClosure)
	if !ok {
		return nil, false
	}
	function, _ := closure.Fn.(*ssa.Function)
	return []completionCallee{{launch: launch, closure: closure, function: function}}, function != nil
}

// exactCallbacks resolves a callback value to the function literals it may
// hold. Loads require one dominating store, phi edges must all resolve, and
// other call results are opaque.
func exactCallbacks(value ssa.Value, invocation ssa.Instruction, allowOnceFunc bool, seen map[ssa.Value]bool) ([]*ssa.MakeClosure, bool) {
	if value == nil || seen[value] {
		return nil, false
	}
	seen[value] = true
	if inner, ok := UnwrapTransparentValue(
		value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return exactCallbacks(inner, invocation, allowOnceFunc, seen)
	}
	switch typed := value.(type) {
	case *ssa.MakeClosure:
		return []*ssa.MakeClosure{typed}, true
	case *ssa.Call:
		common := typed.Common()
		if allowOnceFunc && CallMatchesSymbol(common, syncOnceFunc) && len(common.Args) == 1 {
			return exactCallbacks(common.Args[0], invocation, allowOnceFunc, seen)
		}
	case *ssa.UnOp:
		if stored, ok := uniquelyStoredValueBefore(typed.X, invocation); ok {
			return exactCallbacks(stored, invocation, allowOnceFunc, seen)
		}
	case *ssa.Alloc:
		if stored, ok := uniquelyStoredValueBefore(typed, invocation); ok {
			return exactCallbacks(stored, invocation, allowOnceFunc, seen)
		}
	case *ssa.Phi:
		var result []*ssa.MakeClosure
		for _, edge := range typed.Edges {
			closures, ok := exactCallbacks(edge, invocation, allowOnceFunc, cloneValueSet(seen))
			if !ok {
				return nil, false
			}
			result = append(result, closures...)
		}
		return result, len(result) > 0
	}
	return nil, false
}

// mappedLocal is one callee parameter or captured variable that stands for
// the caller's target, together with the caller value supplied to it.
type mappedLocal struct {
	local    ssa.Value
	supplied ssa.Value
	kind     localKind
}

// localKind is how the supplied caller value relates to the target, which
// decides which callee receivers stand for the target.
type localKind uint8

const (
	// localExact: the supplied value is the target, or an aggregate that
	// stores it, so any receiver derived from the local completes it.
	localExact localKind = iota
	// localProjection: the supplied value is a stable field or index path
	// beneath the target. Only the local itself may discharge the
	// obligation, because further projections, phis, and loads inside the
	// callee could select a different owner.
	localProjection
	// localOwner: the target is a stable path beneath the supplied value.
	// The receiver must select the mirrored path beneath the local.
	localOwner
	// localCallback: the supplied value is a callback bound to the method on
	// the target, so invoking the local is the completion.
	localCallback
)

func (search *completionSearch) mappedLocals(callee completionCallee, target ssa.Value, invocation ssa.Instruction) []mappedLocal {
	var result []mappedLocal
	if callee.closure != nil {
		for index, free := range callee.function.FreeVars {
			if index >= len(callee.closure.Bindings) {
				break
			}
			if local, ok := search.capturedLocal(callee, free, callee.closure.Bindings[index], target, invocation); ok {
				result = append(result, local)
			}
		}
	}
	if callee.common != nil {
		for index, parameter := range callee.function.Params {
			if index >= len(callee.common.Args) {
				break
			}
			if local, ok := search.argumentLocal(parameter, callee.common.Args[index], target); ok {
				result = append(result, local)
			}
		}
	}
	return result
}

// capturedLocal maps a captured variable. A deferred closure observes an
// addressable capture when it runs, after any reassignment that follows the
// defer, so such captures must have exactly one dominating store.
func (search *completionSearch) capturedLocal(
	callee completionCallee,
	free, binding ssa.Value,
	target ssa.Value,
	invocation ssa.Instruction,
) (mappedLocal, bool) {
	value := CapturedBindingValue(binding)
	exact := CapturedBindingMatches(binding, target)
	if callee.launch == launchDeferred && invocation != nil {
		stable, ok := deferredBindingValue(binding, target, invocation)
		if !ok {
			return mappedLocal{}, false
		}
		value, exact = stable, SameValue(stable, target)
	}
	switch {
	case exact:
		return mappedLocal{local: free, supplied: value, kind: localExact}, true
	case search.valueCallsMethod(value, target):
		return mappedLocal{local: free, supplied: value, kind: localCallback}, true
	case ValueDerivesFrom(value, target, map[ssa.Value]bool{}):
		// The closure captured a projection of the target, such as a body
		// selected from a response before the literal was created.
		return mappedLocal{local: free, supplied: value, kind: localExact}, true
	case ValueIsAccessPathFrom(target, value):
		return mappedLocal{local: free, supplied: value, kind: localOwner}, true
	}
	return mappedLocal{}, false
}

func (search *completionSearch) argumentLocal(parameter, argument, target ssa.Value) (mappedLocal, bool) {
	switch {
	case ProveIdentity(AccessPath{Value: argument}, AccessPath{Value: target}).Proven():
		return mappedLocal{local: parameter, supplied: argument, kind: localExact}, true
	case search.valueCallsMethod(argument, target):
		return mappedLocal{local: parameter, supplied: argument, kind: localCallback}, true
	case strictNonEmptyAccessPath(argument, target):
		return mappedLocal{local: parameter, supplied: argument, kind: localProjection}, true
	case ValueContainsValue(argument, target):
		return mappedLocal{local: parameter, supplied: argument, kind: localExact}, true
	case ValueIsAccessPathFrom(target, argument):
		return mappedLocal{local: parameter, supplied: argument, kind: localOwner}, true
	}
	return mappedLocal{}, false
}

// receives reports whether a call receiver inside the callee stands for the
// caller's target through this local.
func (local mappedLocal) receives(receiver, target ssa.Value) bool {
	if receiver == nil {
		return false
	}
	switch local.kind {
	case localExact:
		return ValueDerivesFrom(receiver, local.local, map[ssa.Value]bool{})
	case localProjection:
		return exactCleanupReceiver(receiver, local.local)
	case localOwner:
		// The callee closes the path beneath its local that mirrors the
		// target's path beneath the supplied owner, such as resp.Body from a
		// captured resp.
		return ProveIdentity(
			AccessPath{Value: receiver, Root: local.local},
			AccessPath{Value: target, Root: local.supplied},
		).Proven()
	case localCallback:
	}
	return false
}

func exactCleanupReceiver(receiver, parameter ssa.Value) bool {
	if receiver == nil || parameter == nil {
		return false
	}
	if inner, ok := UnwrapTransparentValue(
		receiver,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return exactCleanupReceiver(inner, parameter)
	}
	return receiver == parameter
}

// completionSearch proves one method for one instruction and target. seen
// stops recursion through helper cycles.
type completionSearch struct {
	method   string
	coverage CompletionCoverage
	// seen holds the callee bodies on the current search path and the
	// callback values already examined. A recursive literal captures the
	// variable that holds itself, so both guards are needed to terminate.
	seen       map[*ssa.Function]bool
	seenValues map[ssa.Value]bool
}

func newCompletionSearch(method string, coverage CompletionCoverage) *completionSearch {
	return &completionSearch{
		method:     method,
		coverage:   coverage,
		seen:       map[*ssa.Function]bool{},
		seenValues: map[ssa.Value]bool{},
	}
}

// completes reports whether the instruction's callees all call method on the
// target with the coverage their launch demands. The second result is false
// when no callee body was available to search.
func (search *completionSearch) completes(instruction ssa.Instruction, target ssa.Value) (launchKind, bool, bool) {
	callees, ok := resolveCallees(instruction)
	if !ok || len(callees) == 0 {
		return launchNone, false, false
	}
	searched := false
	for _, callee := range callees {
		if callee.function == nil || len(callee.function.Blocks) == 0 || search.seen[callee.function] {
			return callee.launch, false, searched
		}
		searched = true
		if !search.calleeCompletes(callee, target, instruction) {
			return callee.launch, false, true
		}
	}
	return callees[0].launch, true, true
}

func (search *completionSearch) calleeCompletes(callee completionCallee, target ssa.Value, invocation ssa.Instruction) bool {
	if search.seen[callee.function] {
		return false
	}
	search.seen[callee.function] = true
	defer delete(search.seen, callee.function)
	locals := search.mappedLocals(callee, target, invocation)
	if len(locals) == 0 {
		return false
	}
	var nonNil ssa.Value
	for _, local := range locals {
		if local.kind == localExact || local.kind == localProjection {
			nonNil = local.local
			break
		}
	}
	calls := func(candidate ssa.Instruction) bool {
		return search.instructionCompletes(candidate, locals, target)
	}
	return MethodCallCoverage(callee.function, calls, search.coverage, nonNil)
}

// instructionCompletes reports whether one callee instruction discharges the
// obligation: it calls the method on a mapped local, it invokes a callback
// parameter that was bound to the method on the target, or it launches a
// nested callee that completes the local by the same proof.
func (search *completionSearch) instructionCompletes(candidate ssa.Instruction, locals []mappedLocal, target ssa.Value) bool {
	called := InstructionCall(candidate)
	if called != nil && CallName(called) == search.method {
		receiver := CallReceiver(called)
		for _, local := range locals {
			if local.receives(receiver, target) {
				return true
			}
		}
	}
	for _, local := range locals {
		// A callback bound to the method on the target, such as rows.Close,
		// completes when the callee invokes that exact local.
		if local.kind == localCallback {
			if called != nil && SameValue(called.Value, local.local) {
				return true
			}
			continue
		}
		if _, proven, _ := search.completes(candidate, local.local); proven {
			return true
		}
	}
	return false
}

// ValueCallsMethod reports whether value is, or carries, a callback that
// calls method on target when invoked: a function literal whose body
// completes the target, a bound method value, or such a callback held in a
// local, passed through a call result, or merged by a phi.
func ValueCallsMethod(value ssa.Value, method string, target ssa.Value) bool {
	return newCompletionSearch(method, CoverageEveryReturn).valueCallsMethod(value, target)
}

func (search *completionSearch) valueCallsMethod(value, target ssa.Value) bool {
	if value == nil || search.seenValues[value] {
		return false
	}
	search.seenValues[value] = true
	if inner, ok := UnwrapTransparentValue(
		value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return search.valueCallsMethod(inner, target)
	}
	switch typed := value.(type) {
	case *ssa.MakeClosure:
		callees, ok := closureCallees(typed, launchCallback)
		return ok && search.calleeCompletes(callees[0], target, nil)
	case *ssa.Alloc:
		return search.storedValueCallsMethod(typed, target)
	case *ssa.UnOp:
		return search.storedValueCallsMethod(typed.X, target)
	case *ssa.Call:
		// A wrapper that receives the callback is assumed to preserve it, so
		// the wrapped result still counts as carrying the cleanup.
		return slices.ContainsFunc(typed.Common().Args, func(argument ssa.Value) bool {
			return search.valueCallsMethod(argument, target)
		})
	case *ssa.Phi:
		return slices.ContainsFunc(typed.Edges, func(edge ssa.Value) bool {
			return search.valueCallsMethod(edge, target)
		})
	}
	return false
}

func (search *completionSearch) storedValueCallsMethod(address, target ssa.Value) bool {
	if address == nil || address.Referrers() == nil {
		return false
	}
	for _, reference := range *address.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == address && search.valueCallsMethod(store.Val, target) {
			return true
		}
	}
	return false
}
