package architecture

import (
	"go/ast"
	"testing"
)

// A trace event is only useful if a reader can ask for one candidate's proof
// and get it. The trace package makes that structural: its event type is
// unexported, so the only way to emit is through a Probe, and a Probe is bound
// to a candidate when it is built. ForPackage is the single deliberate escape,
// for work that belongs to no candidate. Each use is listed here so widening
// the escape is a review decision rather than a convenience.
var packageScopedTraceProbes = map[string]string{
	"internal/passes/lifecyclefacts/evidence.go": "the shared evidence layer is built once per package and rebound to each candidate through ForCandidate",
}

func TestTraceEventsAreAttributedToACandidate(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	for _, source := range inventory.productionGoFiles(t, "internal") {
		if source.repositoryPath == "internal/trace/trace.go" {
			continue
		}
		ast.Inspect(source.file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "ForPackage" {
				return true
			}
			if _, allowed := packageScopedTraceProbes[source.repositoryPath]; allowed {
				return true
			}
			position := source.fileSet.Position(selector.Sel.Pos())
			t.Errorf("%s:%d traces without a candidate; build the probe with trace.For so the proof can be selected, "+
				"or record the reason in packageScopedTraceProbes", source.repositoryPath, position.Line)
			return true
		})
	}
}
