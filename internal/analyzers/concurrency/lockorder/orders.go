package lockorder

// Ordering evidence is a package-local graph of declaration classes, not a
// points-to proof of a runtime deadlock. Each edge retains one acquisition
// witness. Bounded breadth-first searches explain short cycles without
// enumerating paths or changing the instance-based recursive-lock check.

import (
	"fmt"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

const (
	maxOrderDepth  = 8
	maxOrderSearch = 512
	maxOrderEdges  = 4096
)

type lockAcquisition struct {
	class    string
	position token.Pos
	read     bool
	variant  bool
	calls    []analysis.RelatedInformation
	resource ssaflow.EmbeddedFieldPath
	instance string
	widened  bool
}

func acquisitionAt(instruction ssa.Instruction, class string) lockAcquisition {
	receiver := ssaflow.CallReceiver(ssaflow.InstructionCall(instruction))
	resource, _ := lockResourcePath(receiver)
	instance := lockIdentityOf(receiver)
	return lockAcquisition{
		class: class, position: instruction.Pos(), read: readModeAcquisition(instruction), variant: loopVariantLock(instruction), resource: resource,
		instance: instance, widened: class != "" && class != instance,
	}
}

func (acquired lockAcquisition) through(call *ssa.Call) lockAcquisition {
	acquired.calls = append([]analysis.RelatedInformation{{Pos: call.Pos(), Message: "calls " + call.Common().StaticCallee().String()}}, acquired.calls...)
	return bindLockAcquisition(acquired, call)
}

func (acquired lockAcquisition) mode() string {
	if acquired.read {
		return "RLock"
	}
	return "Lock"
}

func (acquired lockAcquisition) site() token.Pos {
	if len(acquired.calls) != 0 {
		return acquired.calls[0].Pos
	}
	return acquired.position
}

type orderEdge struct {
	held, acquired lockAcquisition
	guards         []ssa.Value
}

type lockOrders struct {
	edges map[lockRelation]orderEdge
	out   map[string][]orderEdge
	// Function walks stage edges until their feasibility search completes.
	collectOnly bool
	staged      []orderEdge
}

func newLockOrders() *lockOrders {
	return &lockOrders{edges: map[lockRelation]orderEdge{}, out: map[string][]orderEdge{}}
}

func (orders *lockOrders) record(pass *analysis.Pass, held, acquired lockAcquisition, guards ...ssa.Value) {
	// Unknown classes and same-declaration instance ordering remain outside
	// this check's scope, before considering any cross-owner refinement.
	if held.class == "" || acquired.class == "" || held.class == acquired.class {
		return
	}
	// A declaration edge between fields of one owner type must not silently
	// turn two unproved participants into one object. Keep exact local evidence
	// instead; unknown aliasing is neither disjointness nor synchronization.
	// https://github.com/pion/dtls/blob/05e47ce632b71d7af3d334fde49b65a24413bb4f/conn_go_test.go#L96-L118
	if crossOwnerClassUncertain(held, acquired) {
		analysisTrace.For(pass, "lockorder", string(check.LockContradictoryOrder), acquired.site()).Decision(analysisTrace.Step{
			Reason: "cross-owner-class-unknown", Outcome: analysisTrace.OutcomeUnknown, Pos: acquired.site(),
		})
		held.class, acquired.class = held.instance, acquired.instance
	}
	// Two instances of one declaration class are not a class-order conflict.
	// Unknown identities must not bridge an otherwise disconnected cycle.
	if held.class == "" || acquired.class == "" || held.class == acquired.class {
		return
	}
	relation := lockRelation{from: held.class, to: acquired.class}
	if _, exists := orders.edges[relation]; exists || len(orders.edges) >= maxOrderEdges {
		return
	}
	edge := orderEdge{held: held, acquired: acquired, guards: slices.Clone(guards)}
	if orders.collectOnly {
		orders.edges[relation] = edge
		orders.staged = append(orders.staged, edge)
		return
	}
	path := orders.path(acquired.class, held.class)
	if len(path) != 0 && !serializedCycle(edge, path) && (len(path) == 1 || orders.novelCycle(edge, path)) {
		reportOrderCycle(pass, append([]orderEdge{edge}, path...))
	}
	orders.edges[relation] = edge
	orders.out[held.class] = append(orders.out[held.class], edge)
}

func crossOwnerClassUncertain(held, acquired lockAcquisition) bool {
	left, right := held.resource, acquired.resource
	// Only compare participants actually rooted in one caller. An unbound
	// callee snapshot has not supplied a second caller participant at all.
	if !held.widened || !acquired.widened || left.Root == nil || right.Root == nil || left.Depth == 0 || right.Depth == 0 ||
		left.Root.Parent() == nil || left.Root.Parent() != right.Root.Parent() || !types.Identical(left.Root.Type(), right.Root.Type()) {
		return false
	}
	return !ssaflow.NewStorage(nil).Same(left.Root, right.Root).Proven()
}

// Only exact global exclusive guards enter this set. A declaration-class
// match or read lock cannot establish serialization between executions.
func serializedCycle(closing orderEdge, path []orderEdge) bool {
	for _, guard := range closing.guards {
		if slices.ContainsFunc(path, func(edge orderEdge) bool { return !slices.Contains(edge.guards, guard) }) {
			continue
		}
		return true
	}
	return false
}

// Longer witnesses must not amplify iteration-dependent identity, or repeat
// a contradiction already explained by a reversed pair within that witness.
// Nylon's queue consumers lock different pooled containers; combining those
// classes with existing peer-state reversals invents misleading longer cycles:
// https://github.com/encodeous/nylon/blob/c4a96c804f7aa08512721dec7994907eab100bc8/polyamide/device/receive.go#L441-L488
func (orders *lockOrders) novelCycle(closing orderEdge, path []orderEdge) bool {
	if closing.held.variant || closing.acquired.variant {
		return false
	}
	for _, edge := range path {
		if edge.held.variant || edge.acquired.variant {
			return false
		}
		if _, reversed := orders.edges[lockRelation{from: edge.acquired.class, to: edge.held.class}]; reversed {
			return false
		}
	}
	return true
}

// A visited class is reached by its shortest witness. Depth and total edge
// visits are capped; exhaustion means no claim, never evidence of a cycle.
// Modes are retained as evidence, not used to drop read/read edges: a waiting
// writer blocks later RLock calls in Go, so recursive readers can deadlock too.
func (orders *lockOrders) path(from, to string) []orderEdge {
	if edge, ok := orders.edges[lockRelation{from: from, to: to}]; ok {
		return []orderEdge{edge}
	}
	type route struct {
		class string
		edges []orderEdge
	}
	queue := []route{{class: from}}
	seen := map[string]bool{from: true}
	budget := maxOrderSearch
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if len(current.edges) >= maxOrderDepth-1 {
			continue
		}
		for _, edge := range orders.out[current.class] {
			budget--
			if budget < 0 {
				return nil
			}
			if edge.held.variant || edge.acquired.variant {
				continue
			}
			next := edge.acquired.class
			if seen[next] {
				continue
			}
			path := append(slices.Clone(current.edges), edge)
			if next == to {
				return path
			}
			seen[next] = true
			queue = append(queue, route{class: next, edges: path})
		}
	}
	return nil
}

func reportOrderCycle(pass *analysis.Pass, cycle []orderEdge) {
	position := cycle[0].acquired.site()
	source := syntax.SourceRange(pass, position)
	names := []string{cycle[0].held.class}
	var related []analysis.RelatedInformation
	probe := analysisTrace.For(pass, "lockorder", string(check.LockContradictoryOrder), position)
	for index, edge := range cycle {
		names = append(names, edge.acquired.class)
		related = append(related, edge.held.calls...)
		related = append(related, analysis.RelatedInformation{
			Pos:     edge.held.position,
			Message: fmt.Sprintf("%s acquired with %s; held before %s", edge.held.class, edge.held.mode(), edge.acquired.class),
		})
		related = append(related, edge.acquired.calls...)
		related = append(related, analysis.RelatedInformation{
			Pos:     edge.acquired.position,
			Message: fmt.Sprintf("%s acquired with %s while %s is held", edge.acquired.class, edge.acquired.mode(), edge.held.class),
		})
		if probe.Enabled() {
			reason := "cycle-order-recorded"
			if len(cycle) == 2 && index != 0 {
				reason = "opposite-order-recorded"
			}
			probe.Evidence(analysisTrace.Step{
				Reason: reason, Outcome: analysisTrace.OutcomeRejected, Pos: edge.acquired.site(),
				Details: map[string]string{
					"held": edge.held.class, "acquired": edge.acquired.class,
					"held-mode": edge.held.mode(), "acquired-mode": edge.acquired.mode(),
				},
			})
		}
	}
	message := "contradictory lock order: " + strings.Join(names, " -> ")
	if len(cycle) == 2 {
		message = fmt.Sprintf("contradictory lock order: %s and %s", cycle[0].acquired.class, cycle[0].held.class)
	}
	check.Report(pass, check.LockContradictoryOrder, analysis.Diagnostic{Pos: source.Pos(), End: source.End(), Message: message, Related: related})
}
