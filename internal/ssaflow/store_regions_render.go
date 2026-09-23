package ssaflow

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// RenderRegions prints the points-to graph of a function for the ssa
// subcommand: each tracked value with the slots it may refer to, then the
// disjointness answers the graph has given. Regions are named by kind and
// origin; a stale entry, carried around a loop's back edge, is marked.
func RenderRegions(function *ssa.Function) string {
	graph := regionsOfFunction(function)
	defer graph.lock()()
	var buffer strings.Builder
	if !graph.available {
		buffer.WriteString("// regions: unavailable\n")
		return buffer.String()
	}
	buffer.WriteString("// regions:\n")
	for _, parameter := range function.Params {
		graph.renderValue(&buffer, parameter)
	}
	for _, free := range function.FreeVars {
		graph.renderValue(&buffer, free)
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if value, ok := instruction.(ssa.Value); ok {
				graph.renderValue(&buffer, value)
			}
		}
	}
	for _, entry := range graph.applied {
		fmt.Fprintf(&buffer, "//   applied %s at %s: %d edges, %d effects, %d truncated\n",
			entry.callee.String(), entry.instruction.String(), entry.edges, entry.effects, entry.truncated)
	}
	for _, entry := range graph.widened {
		at := "join"
		if entry.at != nil {
			at = entry.at.String()
		}
		fmt.Fprintf(&buffer, "//   widened %s to unknown at %s: %d pointees\n", slotName(entry.target), at, entry.size)
	}
	origins := make([]escapeOrigin, 0, len(graph.escapeOrigins))
	for key := range graph.escapeOrigins {
		origins = append(origins, key)
	}
	sort.Slice(origins, func(i, j int) bool {
		left, right := graph.escapeOrigins[origins[i]], graph.escapeOrigins[origins[j]]
		if left.Pos() != right.Pos() {
			return left.Pos() < right.Pos()
		}
		if origins[i].kind != origins[j].kind {
			return origins[i].kind < origins[j].kind
		}
		return slotName(origins[i].target) < slotName(origins[j].target)
	})
	for _, key := range origins {
		fmt.Fprintf(&buffer, "//   escaped %s %s at %s\n", slotName(key.target), key.kind, graph.escapeOrigins[key])
	}
	for _, decision := range graph.disjoint {
		fmt.Fprintf(&buffer, "//   disjoint %s %s: %s\n", decision.Value.Name(), decision.Target.Name(), decision.Reason)
	}
	return buffer.String()
}

func (graph *regionGraph) renderValue(buffer *strings.Builder, value ssa.Value) {
	set, ok := graph.values[value]
	if !ok || len(set) == 0 {
		return
	}
	entries := make([]string, 0, len(set))
	for target, stale := range set {
		entry := regionName(target.region)
		if target.path != "" {
			entry += " " + target.path
		}
		if stale {
			entry += " (stale)"
		}
		entries = append(entries, entry)
	}
	sort.Strings(entries)
	fmt.Fprintf(buffer, "//   %s -> %s\n", value.Name(), strings.Join(entries, ", "))
}

// slotName names a slot for the dump: its object, then its path.
func slotName(target slot) string {
	if target.path == "" {
		return regionName(target.region)
	}
	return regionName(target.region) + " " + target.path
}

func regionName(object *region) string {
	switch object.kind {
	case regionNil:
		return "nil"
	case regionUnknown:
		return "?"
	case regionSite:
		return "local:" + object.origin.Name()
	case regionExternal:
		if object.label != "" {
			return "foreign:" + object.label
		}
		switch object.origin.(type) {
		case *ssa.Global:
			return "global:" + object.origin.Name()
		case *ssa.FreeVar:
			return "free:" + object.origin.Name()
		}
		return "param:" + object.origin.Name()
	case regionOpaque:
		// An object a deferred call's summary created has no value to be
		// named by, only its label.
		name := "opaque:"
		if object.origin != nil {
			name += object.origin.Name()
		}
		if object.label != "" {
			return name + "[" + object.label + "]"
		}
		return name
	case regionPlaceholder:
		return fmt.Sprintf("content(%s %s @%d.%d)", regionName(object.source.region), object.source.path, object.stamp.epoch, object.stamp.step)
	case regionSnapshot:
		return "copy:" + object.origin.Name()
	case regionClosure:
		return "closure:" + object.origin.Name()
	}
	return "?"
}
