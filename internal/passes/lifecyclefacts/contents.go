package lifecyclefacts

import (
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// A caller that hands an aggregate to a callee it cannot read needs to know
// whether a resource inside that aggregate can outlive the call. Retained
// answers that for the parameter itself and deliberately not for what is
// loaded out of it, so this file records the loose complement: the access
// paths beneath a struct-shaped parameter whose contents the callee may
// keep. The walk is loose in the same way Retained is. A store anywhere, a
// captured cell, a send, a return, a goroutine argument, an opaque or
// interface callee, and a callee summarized as keeping its argument all
// count, and a body the walk cannot finish keeps everything. It is exact
// only about where: each claim names the static path selected from the
// parameter, so a helper that closes the second file of a pair keeps the
// second path, whatever its Close does, and not the first. A basic-typed
// load holds no resource and is never a claim. Nothing here is an ownership
// transfer; the strict Stored bit is untouched.

// Kept is one loose content-retention claim: the value at Path beneath
// Parameter, or something loaded out of it, may be kept beyond the call. The
// empty path is the parameter itself or a whole copy of it, which keeps
// every path beneath it.
type Kept struct {
	Parameter int
	Path      string
}

// keptPathLimit bounds the paths one parameter claims before the claim
// collapses to the whole parameter, and keptPathDepth bounds how deep a
// claim reaches; a shorter prefix covers everything the longer path would.
const (
	keptPathLimit = 8
	keptPathDepth = 4
)

// keptEverything is the claim that keeps every path beneath the parameter.
var keptEverything = []string{""}

type keptKey struct {
	function  *ssa.Function
	parameter ssa.Value
}

type contentsWalk struct {
	pass   *analysis.Pass
	lookup func(ssa.Instruction) (Fact, bool)
	budget *ssaflow.SearchBudget
	memo   *ssaflow.CallGraphMemo[keptKey, []string]
}

// keptPaths returns the access paths beneath the parameter whose contents
// the function may keep, sorted, with the empty path standing for all of
// them.
func (cache *retentionCache) keptPaths(pass *analysis.Pass, function *ssa.Function, parameter ssa.Value) []string {
	walk := &contentsWalk{pass: pass, lookup: cache.lookup, budget: ssaflow.NewSearchBudget(retentionBudget), memo: cache.kept}
	return walk.paths(function, parameter)
}

// keepsContentsAt reports whether one of the kept paths reaches the value
// at path: the claim is the path itself, a prefix that keeps the aggregate
// around it, or a longer path inside it, since keeping part of a resource
// keeps the resource. The empty path on either side reaches everything.
func keepsContentsAt(kept []string, path string) bool {
	return slices.ContainsFunc(kept, func(claim string) bool {
		return claim == "" || path == "" || claim == path ||
			strings.HasPrefix(path, claim+"/") || strings.HasPrefix(claim, path+"/")
	})
}

func (walk *contentsWalk) paths(function *ssa.Function, parameter ssa.Value) []string {
	if function == nil || len(function.Blocks) == 0 {
		return keptEverything
	}
	key := keptKey{function: function, parameter: parameter}
	result := keptEverything
	// Guard before looking up the answer, as the retention walk does: a
	// recursive call contributes only the loose answer.
	walk.memo.WithFunction(function, func() {
		result = walk.memo.Compose(key, walk.budget, func() []string {
			return walk.collect(function, parameter)
		}, func(_ ssaflow.SummaryUnavailable, _ []string) []string {
			return keptEverything
		})
	})
	return result
}

func (walk *contentsWalk) collect(function *ssa.Function, parameter ssa.Value) []string {
	set := map[string]bool{}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !walk.budget.Spend() {
				return keptEverything
			}
			walk.instructionKeeps(function, instruction, parameter, set)
			if set[""] || len(set) > keptPathLimit {
				return keptEverything
			}
		}
	}
	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return paths
}

// contentPath returns the joined access path of value beneath the parameter
// when value is the parameter, a whole copy of it, or a static selection
// from it that could hold a resource. A value that derives from the
// parameter without a static path, such as a field selected from a local
// copy of the pointee, is claimed as the whole parameter: the walk must
// over-approximate, so what it cannot place it keeps everywhere.
func contentPath(value, parameter ssa.Value) (string, bool) {
	if value == nil {
		return "", false
	}
	if _, basic := value.Type().Underlying().(*types.Basic); basic {
		return "", false
	}
	if path, ok := ssaflow.AccessPathFromParameter(value, parameter); ok {
		return ssaflow.JoinAccessPath(path[:min(len(path), keptPathDepth)]), true
	}
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface
	return "", ssaflow.NewReachingWalk(forms).Any(value, func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		return selectedFrom(walk, value, parameter)
	})
}

// selectedFrom reports whether value was loaded or selected out of the
// parameter, through a local copy of its pointee as well as directly. Only
// loads and selections count: a call result is a new value, and what the
// callee kept of its arguments is claimed at the call.
func selectedFrom(walk ssaflow.ReachingWalk, value, parameter ssa.Value) bool {
	if value == parameter {
		return true
	}
	base, ok := selectionBase(value)
	if !ok {
		return false
	}
	if cell, ok := base.(*ssa.Alloc); ok && ssaflow.WholeWrittenCell(cell) {
		for stored := range ssaflow.StoredInto(cell) {
			if walk.Any(stored, func(walk ssaflow.ReachingWalk, stored ssa.Value) bool { return selectedFrom(walk, stored, parameter) }) {
				return true
			}
		}
		return false
	}
	return walk.Any(base, func(walk ssaflow.ReachingWalk, base ssa.Value) bool { return selectedFrom(walk, base, parameter) })
}

// selectionBase returns what a load, field, element, or slice selects from.
func selectionBase(value ssa.Value) (ssa.Value, bool) { //nolint:ireturn // SSA values keep their concrete forms.
	switch typed := value.(type) {
	case *ssa.UnOp:
		if typed.Op == token.MUL {
			return typed.X, true
		}
	case *ssa.FieldAddr:
		return typed.X, true
	case *ssa.IndexAddr:
		return typed.X, true
	case *ssa.Field:
		return typed.X, true
	case *ssa.Index:
		return typed.X, true
	case *ssa.Slice:
		return typed.X, true
	}
	return nil, false
}

func (walk *contentsWalk) instructionKeeps(function *ssa.Function, instruction ssa.Instruction, parameter ssa.Value, set map[string]bool) {
	keep := func(value ssa.Value) {
		if path, ok := contentPath(value, parameter); ok {
			set[path] = true
		}
	}
	switch typed := instruction.(type) {
	case *ssa.Store:
		// The builder's spill of the parameter into a cell only ever
		// written whole is the parameter's own copy, not storage; every
		// other store keeps the value at least as long as its cell lives.
		if local, ok := localStorage(typed.Addr, function); ok && local == typed.Addr && ssaflow.WholeWrittenCell(local) {
			return
		}
		keep(typed.Val)
	case *ssa.MakeClosure:
		for _, binding := range typed.Bindings {
			keep(binding)
			keep(ssaflow.CapturedBindingValue(binding))
		}
	case *ssa.Send:
		keep(typed.X)
	case *ssa.MapUpdate:
		keep(typed.Value)
	case *ssa.Return:
		for _, result := range typed.Results {
			keep(result)
		}
	case *ssa.Go:
		// Contents handed to a goroutine can be used after the call
		// returns, whatever the started function does with them.
		keep(typed.Common().Value)
		for _, argument := range typed.Common().Args {
			keep(argument)
		}
	case *ssa.Call, *ssa.Defer:
		walk.callKeeps(instruction, parameter, keep, set)
	}
}

// callKeeps follows contents into a callee: a summarized callee keeps what
// its summary says, at the caller's path joined with the callee's; a callee
// with a body is walked; anything else keeps what it is handed.
func (walk *contentsWalk) callKeeps(instruction ssa.Instruction, parameter ssa.Value, keep func(ssa.Value), set map[string]bool) {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return
	}
	if fact, ok := walk.fact(instruction); ok {
		for index, argument := range common.Args {
			path, ok := contentPath(argument, parameter)
			if !ok {
				continue
			}
			if fact.Retained.contains(index) {
				set[path] = true
			}
			for _, kept := range fact.Kept {
				if kept.Parameter == index {
					set[joinKeptPath(path, kept.Path)] = true
				}
			}
		}
		return
	}
	callee := common.StaticCallee()
	if callee == nil || len(callee.Blocks) == 0 {
		keep(common.Value)
		for _, argument := range common.Args {
			keep(argument)
		}
		return
	}
	for index, argument := range common.Args {
		path, ok := contentPath(argument, parameter)
		if !ok || index >= len(callee.Params) {
			continue
		}
		for _, sub := range walk.paths(callee, callee.Params[index]) {
			set[joinKeptPath(path, sub)] = true
		}
	}
}

func joinKeptPath(outer, inner string) string {
	switch {
	case outer == "":
		return inner
	case inner == "":
		return outer
	}
	steps := append(ssaflow.SplitAccessPath(outer), ssaflow.SplitAccessPath(inner)...)
	return ssaflow.JoinAccessPath(steps[:min(len(steps), keptPathDepth)])
}

func (walk *contentsWalk) fact(instruction ssa.Instruction) (Fact, bool) {
	if walk.lookup != nil {
		return walk.lookup(instruction)
	}
	return importFact(walk.pass, instruction)
}
