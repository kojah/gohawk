package processownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	analysisTrace "github.com/kojah/gohawk/internal/reporting/trace"
)

func BenchmarkDisabledImmediateGuardTrace(b *testing.B) {
	pkg := ssaflowtest.BuildPackage(b, "guardtrace", `package guardtrace
 func subject(p *int){}
 `)
	function := pkg.Func("subject")
	guard := processGuardProof{NonNil: function.Params[0], Reason: reasonSuccessfulStartProcessNonNil}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		emitImmediateProcessGuard(analysisTrace.Probe{}, function, guard)
	}
}

func TestDisabledImmediateGuardTraceDoesNotBuildMetadata(t *testing.T) {
	// A disabled probe must return before accessing trace-only SSA metadata.
	if allocations := testing.AllocsPerRun(100, func() {
		emitImmediateProcessGuard(analysisTrace.Probe{}, nil, processGuardProof{})
	}); allocations != 0 {
		t.Fatalf("disabled guard trace allocated %g times", allocations)
	}
}
