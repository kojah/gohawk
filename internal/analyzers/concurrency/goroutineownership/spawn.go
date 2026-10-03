package goroutineownership

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Spawn state owns candidate attribution, query allowances and discovery
// availability. The probe precedes every constructor query; partial discovery
// at a cutoff cannot become a diagnostic through later classification.

type spawnAnalysis struct {
	pass          *analysis.Pass
	function      *ssa.Function
	spawn         *ssa.Go
	checkID       check.ID
	signals       []ssa.Value
	groups        []ssa.Value
	owners        []ssa.Value
	pipePeers     []trackedValue
	relayGroup    ssa.Value
	tracked       []trackedValue
	unsettledDone ssa.Instruction
	actions       map[ssa.Instruction]ownershipAction
	edgeEvidence  map[[2]int]joinEdgeEvidence
	// tracing gates the record of ruled-out steps, which is worth keeping only
	// when a reader will see it.
	tracing    bool
	considered []goroutineOwnershipReason
	// probe attributes shared-engine give-ups to this spawn; it is inert when
	// the spawn is not being traced, so the budgets it observes stay silent.
	probe analysisTrace.Probe
	// pool accumulates requests drawn through budget. Default flow and graph
	// helpers retain separate costs. Lazy creation attaches the probe observer.
	pool *ssaflow.SearchBudget
	// discoveryBudget preserves cutoff availability even after partial evidence.
	discoveryBudget *ssaflow.SearchBudget
}

// spawnQueryBudget bounds each shared storage or origin query a spawn proof
// asks; exhaustion is unknown evidence, never a join or a leak.
const spawnQueryBudget = ssaflow.QueryBudget

// spawnPoolBudget bounds selected requests for one spawn. Their per-query
// limits alone do not bound repeated questions per tracked value/instruction.
// A hundred full queries is far beyond an ordinary candidate. Exhaustion is
// unknown, as for one exhausted query; this does not bound every transitive
// graph, flow, type or allocation operation.
const spawnPoolBudget = 100 * spawnQueryBudget

// budget draws one shared query's allowance from this spawn's pool so the
// storage, summary, and completion give-ups inside it reach the trace and
// the selected requests accumulate under one candidate allowance.
func (analysis *spawnAnalysis) budget() *ssaflow.SearchBudget {
	return analysis.queryBudget(spawnQueryBudget)
}

func (analysis *spawnAnalysis) queryBudget(limit int) *ssaflow.SearchBudget {
	if analysis.pool == nil {
		analysis.pool = ssaflow.NewSearchBudget(spawnPoolBudget).Observed(analysis.probe.Observer())
	}
	return analysis.pool.Within(limit)
}

func newSpawnAnalysis(pass *analysis.Pass, function *ssa.Function, spawn *ssa.Go) *spawnAnalysis {
	analysis := &spawnAnalysis{
		pass:     pass,
		function: function,
		spawn:    spawn,
		actions:  make(map[ssa.Instruction]ownershipAction),
	}
	analysis.checkID = check.GoroutineJoin
	analysis.tracing = analysisTrace.Enabled("goroutineownership", string(analysis.checkID))
	analysis.probe = analysisTrace.For(pass, "goroutineownership", string(analysis.checkID), spawn.Pos())
	analysis.discoverCompletion()
	if analysis.discoveryUnavailable(queryCompletionDiscovery) {
		return analysis
	}
	analysis.discoverAdapters()
	if analysis.discoveryBudget.Exhausted() {
		return analysis
	}
	for _, signal := range analysis.signals {
		analysis.tracked = append(analysis.tracked, trackedValue{value: signal, kind: trackedSignal})
	}
	for _, group := range analysis.groups {
		analysis.tracked = append(analysis.tracked, trackedValue{value: group, kind: trackedGroup})
	}
	for _, owner := range analysis.owners {
		analysis.tracked = append(analysis.tracked, trackedValue{value: owner, kind: trackedOwner})
	}
	return analysis
}

// discoverAdapters retains completion alternatives and possible lifecycle
// participants under the same allowance as the original promise census.
func (analysis *spawnAnalysis) discoverAdapters() {
	analysis.relayGroup = analysis.relayCompletionGroup(analysis.discoveryBudget)
	if analysis.relayGroup != nil {
		analysis.groups = append(analysis.groups, analysis.relayGroup)
	}
	if analysis.discoveryUnavailable(queryRelayDiscovery) {
		return
	}
	analysis.owners = spawnedLifecycleOwners(analysis.pass, analysis.spawn, analysis.discoveryBudget)
	if analysis.discoveryUnavailable(queryOwnerDiscovery) {
		return
	}
	analysis.pipePeers = analysis.spawnedPipePeers(analysis.discoveryBudget)
	if analysis.discoveryUnavailable(queryPipePeerDiscovery) {
		return
	}
}

func (analysis *spawnAnalysis) discoverCompletion() {
	analysis.discoveryBudget = analysis.queryBudget(ssaflow.SummaryBudget)
	analysis.signals, analysis.groups, analysis.unsettledDone = spawnedCompletionValues(analysis.pass, analysis.spawn, analysis.discoveryBudget)
}

// discoveryUnavailable preserves cutoff availability across constructor adapters.
// Partial owner/peer evidence cannot establish that another handle is absent.
func (analysis *spawnAnalysis) discoveryUnavailable(phase queryPhase) bool {
	if !analysis.discoveryBudget.Exhausted() {
		return false
	}
	analysis.discoveryBudget.Observe(ssaflow.EvidenceBudgetExhausted, analysis.spawn.Pos(), func() map[string]string {
		pool := "false"
		if analysis.discoveryBudget.PoolExhausted() {
			pool = "true"
		}
		return map[string]string{"phase": phase.String(), "pool_exhausted": pool}
	})
	return true
}
