package resourcelifetime

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestAcquisitionErrorAllowance(t *testing.T) {
	for _, path := range []string{"github.com/stretchr/testify/require", "github.com/stretchr/testify/assert"} {
		t.Run(path, func(t *testing.T) {
			pkg := acquisitionErrorFixture(t, path)
			fatal := path == "github.com/stretchr/testify/require"
			for _, test := range []struct {
				name       string
				http, want bool
			}{
				{"exact", false, fatal},
				{"notNil", false, fatal},
				{"method", false, fatal},
				{"pair", true, true},
				{"pair", false, fatal},
				{"reverse", true, fatal},
				{"sibling", true, fatal},
				{"otherResource", true, fatal},
				{"otherError", true, false},
				{"earlier", true, false},
				{"ordinary", true, false},
			} {
				t.Run(test.name+map[bool]string{true: "HTTP", false: "ordinary"}[test.http], func(t *testing.T) {
					call, resource, errValue := acquiredResourceInputs(t, pkg.Func(test.name))
					checkResourceProofAllowance(t, func(budget *ssaflow.SearchBudget) resourceProof {
						return proveAcquisitionErrorWithin(call, resource, errValue, test.http, budget)
					}, test.want)
				})
			}
		})
	}
}

func TestAcquisitionErrorChildCutoff(t *testing.T) {
	call, resource, errValue := acquiredResourceInputs(t, acquisitionErrorFixture(t, "github.com/stretchr/testify/assert").Func("pair"))
	pool := ssaflow.NewSearchBudget(resourcePoolBudget)
	proof := proveAcquisitionErrorWithin(call, resource, errValue, true, pool.Within(2))
	if proof.State != ssaflow.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("assertion cutoff=%+v, parent exhausted %v", proof, pool.Exhausted())
	}
	if fresh := proveAcquisitionErrorWithin(call, resource, errValue, true, pool.Within(releaseSearchBudget)); !fresh.Proven() {
		t.Fatalf("fresh assertion=%+v", fresh)
	}
}

func TestAcquisitionErrorCensusDiscardsPartial(t *testing.T) {
	call, resource, errValue := acquiredResourceInputs(t, acquisitionErrorFixture(t, "github.com/stretchr/testify/assert").Func("pair"))
	for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
		budget := ssaflow.NewSearchBudget(limit)
		errors, nils := acquisitionErrorAssertionsWithin(call, resource, errValue, budget)
		if resourceFlowExhausted(budget) {
			if errors != nil || nils != nil {
				t.Fatalf("interrupted census published errors=%v nils=%v", errors, nils)
			}
			continue
		}
		if len(errors) != 1 || len(nils) != 1 {
			t.Fatalf("complete census errors=%v nils=%v", errors, nils)
		}
		return
	}
	t.Fatal("assertion census never completed")
}

func TestAcquisitionErrorFlow(t *testing.T) {
	pkg := acquisitionErrorFixture(t, "github.com/stretchr/testify/require")
	for _, name := range []string{"exact", "ordinary"} {
		t.Run(name, func(t *testing.T) {
			call, resource, _ := acquiredResourceInputs(t, pkg.Func(name))
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			proof := evaluateResourceFlow(nil, evidence, call, resource, resourceContract{cleanup: []string{"Close"}})
			if name == "exact" {
				if proof.state != ssaflow.EvidenceDisproven || proof.reason != resourceReasonReleaseProven || proof.leak != nil {
					t.Fatalf("fatal error assertion=%+v", proof)
				}
			} else if proof.state != ssaflow.EvidenceProven || proof.leak == nil {
				t.Fatalf("ordinary leak lost=%+v", proof)
			}
		})
	}
}

func acquisitionErrorFixture(t *testing.T, path string) *ssa.Package {
	t.Helper()
	name := "assert"
	if path == "github.com/stretchr/testify/require" {
		name = "require"
	}
	return ssaflowtest.BuildPackage(t, path, "package "+name+`
 type resource struct{n int}
 func acquire()(*resource,error){return &resource{},nil}
 func Error(t any,err error){}
 func NotNil(t any,v any){}
 func Nil(t any,v any){}
 type Assertions struct{}
 func (*Assertions) Error(err error){}
 func exact(){p,e:=acquire();Error(nil,e);p.n++}
 func notNil(){p,e:=acquire();NotNil(nil,e);p.n++}
 func method(a *Assertions){p,e:=acquire();a.Error(e);p.n++}
 func pair(){p,e:=acquire();Error(nil,e);Nil(nil,p)}
 func reverse(){p,e:=acquire();Nil(nil,p);Error(nil,e)}
 func sibling(yes bool){p,e:=acquire();if yes{Error(nil,e)}else{Nil(nil,p)}}
 func otherResource(other *resource){p,e:=acquire();Error(nil,e);Nil(nil,other);p.n++}
 func otherError(other error){p,e:=acquire();Error(nil,other);Nil(nil,p);_=e}
 func earlier(other error){Error(nil,other);p,e:=acquire();Nil(nil,p);_=e}
 func ordinary(){p,e:=acquire();p.n++;_=e}
 `)
}
