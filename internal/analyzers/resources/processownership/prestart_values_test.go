package processownership

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestStartupOwnersRequireReferenceResults(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ownerresults", `package ownerresults
import "os/exec"
type holder struct{ cmd *exec.Cmd }
func void(cmd *exec.Cmd) {}
func scalar(cmd *exec.Cmd) int { return 1 }
func pair(cmd *exec.Cmd) (int, bool) { return 1, true }
func pointer(cmd *exec.Cmd) *holder { return &holder{cmd} }
func aggregate(cmd *exec.Cmd) holder { return holder{cmd} }
func multiple(cmd *exec.Cmd) (int, *holder, func()) { return 1, &holder{cmd}, func(){ cmd.Wait() } }
func subject(cmd *exec.Cmd) {
void(cmd); scalar(cmd); pair(cmd); pointer(cmd); aggregate(cmd)
_, owner, cleanup := multiple(cmd); defer cleanup(); cmd.Start(); println(owner)
}

`)
	fn := pkg.Func("subject")
	var dump strings.Builder
	if _, err := fn.WriteTo(&dump); err != nil {
		t.Fatal(err)
	}
	t.Log(dump.String())
	start := startupTestCall(t, fn)
	result := collectProcessStartInstructions(start, fn.Params[0], proofs.NewSearchBudget(processQueryBudget))
	if !result.Proven() {
		t.Fatalf("owner census unavailable: %+v", result)
	}
	counts := map[string]int{}
	for _, owner := range result.owners {
		call, ok := owner.(*ssa.Call)
		if extract, extracted := owner.(*ssa.Extract); extracted {
			call, ok = extract.Tuple.(*ssa.Call)
		}
		if !ok {
			t.Fatalf("unexpected owner: %T", owner)
		}
		counts[call.Common().StaticCallee().Name()]++
	}
	for name, want := range map[string]int{"void": 0, "scalar": 0, "pair": 0, "pointer": 1, "aggregate": 1, "multiple": 3} {
		if counts[name] != want {
			t.Errorf("%s owners=%d, want %d", name, counts[name], want)
		}
	}
}

func TestPreStartNonReferenceHandoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "prestartretention", `package prestartretention
import "os/exec"
var saved *exec.Cmd
func retain(cmd *exec.Cmd) { saved = cmd }
func configure(cmd *exec.Cmd) { cmd.Dir = "tmp" }
func scalar(cmd *exec.Cmd) int { return 1 }
func tuple(cmd *exec.Cmd) (int, bool) { return 1, true }
func opaque(*exec.Cmd)
func registered(cmd *exec.Cmd) { retain(cmd) }
func configured(cmd *exec.Cmd) { configure(cmd) }
func observed(cmd *exec.Cmd) { scalar(cmd) }
func paired(cmd *exec.Cmd) { tuple(cmd) }
func unreadable(cmd *exec.Cmd) { opaque(cmd) }
`)
	for _, test := range []struct {
		name string
		want proofs.EvidenceState
	}{
		{"registered", proofs.EvidenceUnknown},
		{"unreadable", proofs.EvidenceUnknown},
		{"configured", proofs.EvidenceDisproven},
		{"observed", proofs.EvidenceDisproven},
		{"paired", proofs.EvidenceDisproven},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			for limit := 0; limit <= processQueryBudget; limit++ {
				pool := proofs.NewSearchBudget(limit)
				proof := &commandProof{pool: pool, evidence: lifecyclefacts.NewLifecycleEvidence(nil, "test", "prestart")}
				got := possiblePreStartResultlessHandoff(proof, call, fn.Params[0])
				if got.Reason == proofs.EvidenceBudgetExhausted {
					if got.State != proofs.EvidenceUnknown {
						t.Fatalf("cutoff %d: %+v", limit, got)
					}
					continue
				}
				if got.State != test.want {
					t.Fatalf("completed allowance %d: %+v, want %v", limit, got, test.want)
				}
				return
			}
			t.Fatal("handoff query never completed")
		})
	}
}
