package ssaflow

import (
	"slices"
	"sort"
	"strconv"
	"strings"
)

// A heap summary is the projection of a function's points-to graph onto
// what a caller can name from outside: the parameters, results, globals,
// and captured variables, with a bounded set of paths beneath each. Every
// internal object collapses to fresh, nil, or unknown. The summary says
// where each named slot may point at exit, how each named object left
// local control, which slots the function read before writing, and where
// the projection was cut. It carries no internal structure, so applying it
// at a call site is substitution: the callee's parameter becomes the
// argument's slot, its result the call's, and fresh a new object.

// HeapRootKind names the kinds of object a caller can refer to.
type HeapRootKind uint8

const (
	// HeapParameter is the object parameter Index refers to; the receiver
	// is parameter zero.
	HeapParameter HeapRootKind = iota
	// HeapResult is the object result Index refers to.
	HeapResult
	// HeapGlobal is the package variable Name.
	HeapGlobal
	// HeapFreeVar is the captured variable Index of a literal.
	HeapFreeVar
)

// HeapRoot is one object a caller can name. A global is named by its
// package path and name, so a caller's graph can find the same variable.
type HeapRoot struct {
	Kind    HeapRootKind
	Index   int
	Package string
	Name    string
}

// HeapSlot is a location beneath a root: the root's object itself when
// Path is empty, else the field or element the joined access path selects.
type HeapSlot struct {
	Root HeapRoot
	Path string
}

// HeapTargetKind names what a slot may hold.
type HeapTargetKind uint8

const (
	// HeapTargetSlot is whatever the caller holds at Slot, or the object
	// Slot itself when its path is empty.
	HeapTargetSlot HeapTargetKind = iota
	// HeapTargetFresh is an object the function created, Origin naming the
	// call or literal that produced it when the graph could see one.
	HeapTargetFresh
	// HeapTargetNil is the nil pointer.
	HeapTargetNil
	// HeapTargetUnknown may be anything.
	HeapTargetUnknown
)

// HeapTarget is what a slot may hold. Object numbers a fresh object within
// its summary, so two fresh objects with one origin, such as the two
// results of one call, stay two objects when the summary is applied.
type HeapTarget struct {
	Kind   HeapTargetKind
	Slot   HeapSlot
	Origin string
	Object int
}

// HeapEdge says the slot may hold the target at exit; Must says it does on
// every normal return, and that nothing else does.
type HeapEdge struct {
	From HeapSlot
	To   HeapTarget
	Must bool
}

// HeapEscape is the set of ways an object left local control.
type HeapEscape uint8

const (
	// HeapEscapedGlobal: stored into a package variable.
	HeapEscapedGlobal HeapEscape = 1 << iota
	// HeapEscapedField: stored into an object the caller can reach, a map,
	// or a collection handed on.
	HeapEscapedField
	// HeapEscapedCall: handed to a call the graph could not see through.
	HeapEscapedCall
	// HeapEscapedAsync: handed to a goroutine.
	HeapEscapedAsync
	// HeapEscapedSend: sent on a channel.
	HeapEscapedSend
)

// HeapEffect records what happened to the object at a slot: how it escaped,
// or which lifecycle method released it. Every says the effect holds on
// every normal return.
type HeapEffect struct {
	Slot    HeapSlot
	Escape  HeapEscape
	Release string
	Every   bool
}

// HeapSummary is the projection of one function's heap.
type HeapSummary struct {
	Edges     []HeapEdge
	Effects   []HeapEffect
	Holds     []HeapHold
	Reads     []HeapSlot
	Requires  []HeapRequirement
	Truncated []HeapSlot
}

// HeapHold says result Result holds the object of parameter Parameter:
// it is that object, or a slot beneath it holds that object. Must says so
// on every normal return where the result is not nil. It is a per-return
// claim the joined edges cannot express: a constructor may return the
// parameter itself on one path and a wrapper holding it on another.
type HeapHold struct {
	Result    int
	Parameter int
	Must      bool
}

// heapPathDepth bounds the paths a summary names beneath a root, and
// heapSlotLimit the slots per root before the root is truncated instead.
const (
	heapPathDepth = 3
	heapSlotLimit = 16
)

func sortedSlots(set map[HeapSlot]bool) []HeapSlot {
	slots := make([]HeapSlot, 0, len(set))
	for at := range set {
		slots = append(slots, at)
	}
	sort.Slice(slots, func(i, j int) bool { return heapSlotLess(slots[i], slots[j]) })
	return slots
}

func heapSlotLess(left, right HeapSlot) bool {
	if left.Root != right.Root {
		if left.Root.Kind != right.Root.Kind {
			return left.Root.Kind < right.Root.Kind
		}
		if left.Root.Index != right.Root.Index {
			return left.Root.Index < right.Root.Index
		}
		if left.Root.Package != right.Root.Package {
			return left.Root.Package < right.Root.Package
		}
		return left.Root.Name < right.Root.Name
	}
	return left.Path < right.Path
}

func heapEdgeLess(left, right HeapEdge) bool {
	if left.From != right.From {
		return heapSlotLess(left.From, right.From)
	}
	if left.To.Kind != right.To.Kind {
		return left.To.Kind < right.To.Kind
	}
	if left.To.Slot != right.To.Slot {
		return heapSlotLess(left.To.Slot, right.To.Slot)
	}
	return left.To.Origin < right.To.Origin
}

func heapEffectLess(left, right HeapEffect) bool {
	if left.Slot != right.Slot {
		return heapSlotLess(left.Slot, right.Slot)
	}
	if left.Escape != right.Escape {
		return left.Escape < right.Escape
	}
	return left.Release < right.Release
}

// String renders a slot as P0/field:1, R0, G:pkg.name, or F1.
func (at HeapSlot) String() string {
	var root string
	switch at.Root.Kind {
	case HeapParameter:
		root = "P" + strconv.Itoa(at.Root.Index)
	case HeapResult:
		root = "R" + strconv.Itoa(at.Root.Index)
	case HeapGlobal:
		root = "G:" + at.Root.Package + "." + at.Root.Name
	case HeapFreeVar:
		root = "F" + strconv.Itoa(at.Root.Index)
	}
	if at.Path == "" {
		return root
	}
	return root + "/" + at.Path
}

// String renders a target.
func (target HeapTarget) String() string {
	switch target.Kind {
	case HeapTargetSlot:
		return target.Slot.String()
	case HeapTargetFresh:
		return "fresh(" + target.Origin + "#" + strconv.Itoa(target.Object) + ")"
	case HeapTargetNil:
		return "nil"
	case HeapTargetUnknown:
	}
	return "unknown"
}

// String renders the summary one entry per line, for tests and the dump.
func (summary HeapSummary) String() string {
	var lines []string
	for _, edge := range summary.Edges {
		mode := "may"
		if edge.Must {
			mode = "must"
		}
		lines = append(lines, "edge "+edge.From.String()+" -> "+edge.To.String()+" "+mode)
	}
	for _, hold := range summary.Holds {
		mode := "may"
		if hold.Must {
			mode = "must"
		}
		lines = append(lines, "holds R"+strconv.Itoa(hold.Result)+" P"+strconv.Itoa(hold.Parameter)+" "+mode)
	}
	for _, effect := range summary.Effects {
		mode := "some"
		if effect.Every {
			mode = "every"
		}
		what := "released " + effect.Release
		if effect.Release == "" {
			what = "escaped " + effect.Escape.String()
		}
		lines = append(lines, "effect "+effect.Slot.String()+" "+what+" "+mode)
	}
	for _, at := range summary.Reads {
		lines = append(lines, "read "+at.String())
	}
	for _, requirement := range summary.Requires {
		lines = append(lines, "requires "+requirement.String()+" every")
	}
	for _, at := range summary.Truncated {
		lines = append(lines, "truncated "+at.String())
	}
	return strings.Join(lines, "\n")
}

// String renders the escape kinds.
func (escape HeapEscape) String() string {
	var names []string
	for kind, name := range map[HeapEscape]string{
		HeapEscapedGlobal: "global", HeapEscapedField: "field", HeapEscapedCall: "call", HeapEscapedAsync: "async", HeapEscapedSend: "send",
	} {
		if escape&kind != 0 {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return strings.Join(names, "+")
}
