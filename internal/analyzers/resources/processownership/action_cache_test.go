package processownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestProcessActionCacheKeepsCommandAndCandidate(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "processactions", `package processactions
import "os/exec"
func subject(first,second *exec.Cmd){defer func(){first.Wait()}()}
`)
	fn := pkg.Func("subject")
	deferred := ssaflow.InstructionsOf[*ssa.Defer](fn)[0]
	t.Log(deferred.String())
	fresh := func(limit int) *commandProof {
		return &commandProof{pool: proofs.NewSearchBudget(limit), evidence: lifecyclefacts.NewLifecycleEvidence(nil, "test", "process-actions")}
	}
	proof := fresh(processPoolBudget)
	if got := proof.action(deferred, fn.Params[0]); got != proofs.EvidenceProven {
		t.Fatalf("fresh exact wait=%v", got)
	}
	for proof.pool.Spend() {
	}
	// Another path reaching this instruction must reuse its completed action,
	// rather than charge the completion query again after other work cut the pool.
	if got := proof.action(deferred, fn.Params[0]); got != proofs.EvidenceProven {
		t.Fatalf("revisited wait=%v", got)
	}
	if got := proof.action(deferred, fn.Params[1]); got != proofs.EvidenceUnknown {
		t.Fatalf("different command reused exact wait=%v", got)
	}
	if got := fresh(0).action(deferred, fn.Params[0]); got != proofs.EvidenceUnknown {
		t.Fatalf("fresh cut candidate borrowed wait=%v", got)
	}
	if got := fresh(processPoolBudget).action(deferred, fn.Params[0]); got != proofs.EvidenceProven {
		t.Fatalf("cutoff poisoned fresh candidate=%v", got)
	}
}
