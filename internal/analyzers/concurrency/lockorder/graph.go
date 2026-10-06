package lockorder

import (
	"cmp"
	"go/token"
	"maps"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/enumtext"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// The analyzer's result is the package's order graph as the walk recorded
// it, for gohawk dump locks. It is a read-only copy: nothing downstream
// decides anything from it, and the cycles it reports are the analyzer's own
// diagnostics, not a second search over these edges.

// LockMode identifies how a recorded lock was acquired. Labels belong to
// rendering and serialization; graph consumers use the numeric mode.
type LockMode uint8

const (
	_ LockMode = iota
	// ModeExclusive is an exclusive Lock acquisition.
	ModeExclusive
	// ModeRead is a shared RLock acquisition.
	ModeRead
)

var lockModeLabels = [...]string{0: "", ModeExclusive: "Lock", ModeRead: "RLock"}

// String returns the acquisition's stable display label.
func (mode LockMode) String() string { return enumtext.Name(mode, lockModeLabels[:]) }

// MarshalText preserves acquisition labels when graph edges are serialized.
func (mode LockMode) MarshalText() ([]byte, error) { return enumtext.Encode(mode, lockModeLabels[:]) }

// UnmarshalText accepts acquisition labels without replacing mode on error.
func (mode *LockMode) UnmarshalText(text []byte) error {
	return enumtext.Decode(mode, text, lockModeLabels[:])
}

// Graph lists the order edges recorded for a package: each pair of lock
// classes held together, with one witness of the order.
type Graph struct {
	Edges []GraphEdge
	// Cycles lists each reported cycle as indices into Edges, in the order
	// the diagnostic names them.
	Cycles [][]int
	// Full reports that the edge cap was reached, so later orders were not
	// recorded and cannot close a cycle.
	Full bool
}

// GraphEdge is one recorded order: Acquired was taken while Held was held.
type GraphEdge struct {
	// Held and Acquired name the lock classes, with this package's path
	// dropped.
	Held, Acquired string
	// HeldMode and AcquiredMode retain the acquisitions' numeric modes.
	HeldMode, AcquiredMode LockMode
	// HeldAt and AcquiredAt are the Lock calls, or the first helper call
	// on the route to one in another function.
	HeldAt, AcquiredAt token.Pos
	// Via lists the helper calls on the route to the acquired lock.
	Via []analysis.RelatedInformation
	// Guards names the other package-level mutexes held exclusively around
	// the order; a cycle whose every edge shares one is serialized by it.
	Guards []string
	// Variant reports a lock whose identity changes with each loop
	// iteration, which never extends a longer cycle.
	Variant bool
}

func (orders *lockOrders) graph(pass *analysis.Pass) *Graph {
	graph := &Graph{Full: len(orders.edges) >= maxOrderEdges}
	relations := slices.SortedFunc(maps.Keys(orders.edges), func(left, right lockRelation) int {
		return cmp.Or(cmp.Compare(left.from, right.from), cmp.Compare(left.to, right.to))
	})
	index := make(map[lockRelation]int, len(relations))
	for position, relation := range relations {
		index[relation] = position
		edge := orders.edges[relation]
		held := displayClass(pass, edge.held.class)
		var guards []string
		for _, guard := range edge.guards {
			if name := displayClass(pass, guard.String()); name != held {
				guards = append(guards, name)
			}
		}
		graph.Edges = append(graph.Edges, GraphEdge{
			Held: held, Acquired: displayClass(pass, edge.acquired.class),
			HeldMode: edge.held.mode(), AcquiredMode: edge.acquired.mode(),
			HeldAt: edge.held.site(), AcquiredAt: edge.acquired.site(),
			Via: slices.Clone(edge.acquired.calls), Guards: guards,
			Variant: edge.held.variant || edge.acquired.variant,
		})
	}
	for _, cycle := range orders.cycles {
		indices := make([]int, 0, len(cycle))
		for _, relation := range cycle {
			indices = append(indices, index[relation])
		}
		graph.Cycles = append(graph.Cycles, indices)
	}
	return graph
}

func relationsOf(cycle []orderEdge) []lockRelation {
	relations := make([]lockRelation, 0, len(cycle))
	for _, edge := range cycle {
		relations = append(relations, lockRelation{from: edge.held.class, to: edge.acquired.class})
	}
	return relations
}

// How a diagnostic names a lock. The analysis keys locks by identities and
// classes built from SSA, such as "(*pkg.Cache).Put.c.mu", which are exact
// but unreadable. A reader knows the lock by the receiver written at its Lock
// call, so diagnostics use that text when the call is in this package and
// fall back to the identity otherwise. Naming never affects what is reported.

func (acquired lockAcquisition) verb() string {
	if acquired.read {
		return "read-locked"
	}
	return "locked"
}

// displayName names an acquisition for a reader: the receiver as written at
// the Lock call, such as `l.mu`, or the lock's class when the call is in
// another package.
func (acquired lockAcquisition) displayName(pass *analysis.Pass) string {
	if text := syntax.CallReceiverText(pass, acquired.position); text != "" {
		return "`" + text + "`"
	}
	return displayClass(pass, acquired.class)
}

// cycleName preserves declaration names across functions, but names a local
// allocation by its acquisition receiver instead of exposing its SSA key.
// The local marker belongs to lockIdentityOf; it is only a display convention.
func (acquired lockAcquisition) cycleName(pass *analysis.Pass) string {
	if strings.Contains(acquired.class, ":local:") {
		return acquired.displayName(pass)
	}
	return displayClass(pass, acquired.class)
}

// displayClass drops the current package's path from a lock class, so
// "*example.com/shop.Ledger.mu" reads "Ledger.mu" inside package shop.
func displayClass(pass *analysis.Pass, class string) string {
	class = strings.TrimPrefix(class, "*")
	return strings.TrimPrefix(class, pass.Pkg.Path()+".")
}

// callEvidence widens each helper call on an acquisition's route to the
// span of the call expression.
func callEvidence(pass *analysis.Pass, calls []analysis.RelatedInformation) []analysis.RelatedInformation {
	evidence := make([]analysis.RelatedInformation, 0, len(calls))
	for _, call := range calls {
		evidence = append(evidence, check.Evidence(pass, call.Pos, call.Message))
	}
	return evidence
}

// lockName names the lock with identity by the receiver of its first direct
// Lock or RLock call in this function.
func (flow lockFlowContext) lockName(identity string) string {
	if acquisition := flow.directAcquisition(identity, false); acquisition != nil {
		if text := syntax.CallReceiverText(flow.pass, acquisition.Pos()); text != "" {
			return "`" + text + "`"
		}
	}
	return identity
}

// directAcquisition returns the function's direct Lock or RLock call for
// identity. With only set, it returns the call only when it is the one
// acquisition of that lock in the function, so evidence citing it is the
// acquisition on every path rather than one of several.
func (flow lockFlowContext) directAcquisition(identity string, only bool) ssa.Instruction {
	acquisitions := flow.acquisitions[identity]
	if only && len(acquisitions) != 1 {
		return nil
	}
	for _, acquisition := range acquisitions {
		if !flow.budget.Spend() {
			return nil
		}
		if _, direct := flow.setup.direct[acquisition]; direct {
			return acquisition
		}
	}
	return nil
}

// acquisitionEvidence cites the one acquisition of identity, or nothing when
// the function acquires it more than once.
func (flow lockFlowContext) acquisitionEvidence(identity string) []analysis.RelatedInformation {
	acquisition := flow.directAcquisition(identity, true)
	if acquisition == nil {
		return nil
	}
	effect := flow.setup.direct[acquisition]
	return []analysis.RelatedInformation{
		check.Evidence(flow.pass, acquisition.Pos(), flow.lockName(identity)+" is "+effect.acquired.verb()+" here"),
	}
}
