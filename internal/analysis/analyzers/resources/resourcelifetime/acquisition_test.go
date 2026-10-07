package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
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
					checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
						return proveAcquisitionErrorWithin(call, resource, errValue, test.http, budget)
					}, test.want)
				})
			}
		})
	}
}

func TestAcquisitionErrorChildCutoff(t *testing.T) {
	call, resource, errValue := acquiredResourceInputs(t, acquisitionErrorFixture(t, "github.com/stretchr/testify/assert").Func("pair"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	proof := proveAcquisitionErrorWithin(call, resource, errValue, true, pool.Within(2))
	if proof.State != proofs.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("assertion cutoff=%+v, parent exhausted %v", proof, pool.Exhausted())
	}
	if fresh := proveAcquisitionErrorWithin(call, resource, errValue, true, pool.Within(releaseSearchBudget)); !fresh.Proven() {
		t.Fatalf("fresh assertion=%+v", fresh)
	}
}

func TestAcquisitionErrorCensusDiscardsPartial(t *testing.T) {
	call, resource, errValue := acquiredResourceInputs(t, acquisitionErrorFixture(t, "github.com/stretchr/testify/assert").Func("pair"))
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
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
				if proof.state != proofs.EvidenceDisproven || proof.reason != resourceReasonReleaseProven || proof.leak != nil {
					t.Fatalf("fatal error assertion=%+v", proof)
				}
			} else if proof.state != proofs.EvidenceProven || proof.leak == nil {
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

func TestAcquisitionContextCancellationAllowance(t *testing.T) {
	pkg := acquisitionContextFixture(t)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"exact", true},
		{"cause", true},
		{"begin", true},
		{"query", true},
		{"later", false},
		{"deferred", false},
		{"conditional", false},
		{"sibling", false},
		{"replaced", false},
		{"conn", false},
		{"ordinary", false},
		{"txRows", false},
		{"stmtRows", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := contextAcquisitionCall(t, pkg.Func(test.name))
			if test.name == "conn" || test.name == "ordinary" || test.name == "txRows" || test.name == "stmtRows" || test.name == "opaque" {
				if proof := proveAcquisitionContextCanceledWithin(call, proofs.NewSearchBudget(0)); proof.State != proofs.EvidenceDisproven {
					t.Fatalf("noneligible API queried cancellation: %+v", proof)
				}
				return
			}
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				return proveAcquisitionContextCanceledWithin(call, budget)
			}, test.want)
		})
	}
}

func TestAcquisitionContextCancellationChildCutoff(t *testing.T) {
	call := contextAcquisitionCall(t, acquisitionContextFixture(t).Func("exact"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	got := proveAcquisitionContextCanceledWithin(call, pool.Within(1))
	if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("interrupted cancellation = %+v; parent exhausted %v", got, pool.Exhausted())
	}
	fresh := proveAcquisitionContextCanceledWithin(call, pool.Within(releaseSearchBudget))
	if !fresh.Proven() || fresh.Reason != resourceReasonCanceledAcquisition {
		t.Fatalf("fresh cancellation = %+v", fresh)
	}
}

func TestAcquisitionContextCancellationFlow(t *testing.T) {
	pkg := acquisitionContextFixture(t)
	for _, name := range []string{"exact", "later"} {
		t.Run(name, func(t *testing.T) {
			call := contextAcquisitionCall(t, pkg.Func(name))
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			resource := ssaflow.InstructionsOf[*ssa.Extract](call.Parent())
			var result ssa.Value
			for _, value := range resource {
				if value.Tuple == call && value.Index == 0 {
					result = value
				}
			}
			if result == nil {
				t.Fatal("missing resource result")
			}
			got := evaluateResourceFlow(nil, evidence, call, result, resourceContract{
				family: resourceFamilySQL, packagePath: "database/sql", cleanup: []string{"Close"},
			})
			if name == "exact" {
				if got.state != proofs.EvidenceDisproven || got.reason != resourceReasonCanceledAcquisition || got.leak != nil {
					t.Fatalf("canceled acquisition reported = %+v", got)
				}
			} else if got.state != proofs.EvidenceProven || got.leak == nil {
				t.Fatalf("later cancellation lost independent statement leak = %+v", got)
			}
		})
	}
}

func contextAcquisitionCall(t *testing.T, fn *ssa.Function) *ssa.Call {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		switch ssaflow.CallName(call.Common()) {
		case "PrepareContext", "Prepare", "BeginTx", "QueryContext":
			return call
		}
	}
	t.Fatal("missing acquisition")
	return nil
}

func acquisitionContextFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "acquisitioncontext", `package acquisitioncontext
import("context";"database/sql")
func opaque(db *sql.DB,p context.Context){_,_=db.PrepareContext(p,"q")}
func exact(db *sql.DB,p context.Context){ctx,cancel:=context.WithCancel(p);cancel();_,_=db.PrepareContext(ctx,"q")}
func cause(db *sql.DB,p context.Context){ctx,cancel:=context.WithCancelCause(p);cancel(nil);_,_=db.PrepareContext(ctx,"q")}
func begin(db *sql.DB,p context.Context){ctx,cancel:=context.WithCancel(p);cancel();_,_=db.BeginTx(ctx,nil)}
func query(db *sql.DB,p context.Context){ctx,cancel:=context.WithCancel(p);cancel();_,_=db.QueryContext(ctx,"q")}
func later(db *sql.DB,p context.Context){ctx,cancel:=context.WithCancel(p);_,_=db.PrepareContext(ctx,"q");cancel()}
func deferred(db *sql.DB,p context.Context){ctx,cancel:=context.WithCancel(p);defer cancel();_,_=db.PrepareContext(ctx,"q")}
func conditional(db *sql.DB,p context.Context,yes bool){ctx,cancel:=context.WithCancel(p);if yes{cancel()};_,_=db.PrepareContext(ctx,"q")}
func sibling(db *sql.DB,p context.Context){ctx,_:=context.WithCancel(p);_,cancel:=context.WithCancel(p);cancel();_,_=db.PrepareContext(ctx,"q")}
func replaced(db *sql.DB,p context.Context){ctx,cancel:=context.WithCancel(p);other,_:=context.WithCancel(p);cancel();ctx=other;_,_=db.PrepareContext(ctx,"q")}
func conn(db *sql.Conn,p context.Context){ctx,cancel:=context.WithCancel(p);cancel();_,_=db.PrepareContext(ctx,"q")}
func ordinary(db *sql.DB,p context.Context){_,cancel:=context.WithCancel(p);cancel();_,_=db.Prepare("q")}
func txRows(db *sql.Tx,p context.Context){ctx,cancel:=context.WithCancel(p);cancel();_,_=db.QueryContext(ctx,"q")}
func stmtRows(db *sql.Stmt,p context.Context){ctx,cancel:=context.WithCancel(p);cancel();_,_=db.QueryContext(ctx)}
`)
}

func TestAcquisitionErrorResultAllowance(t *testing.T) {
	pkg := acquisitionResultFixture(t)
	for _, test := range []struct {
		name    string
		slot    int
		queried bool
	}{
		{"pair", 1, true},
		{"triple", 2, true},
		{"blank", 1, true},
		{"discarded", -1, false},
		{"unused", 1, true},
		{"scalar", -1, false},
		{"noError", -1, false},
		{"nonLast", -1, false},
		{"concreteError", -1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := acquisitionResultCall(t, pkg.Func(test.name))
			baseline := proveAcquisitionErrorResultWithin(call, nil)
			var want ssa.Value
			if test.slot >= 0 {
				want = ssacall.CallResult(call, test.slot)
			}
			if baseline.proof.State == proofs.EvidenceUnknown || baseline.value != want || baseline.proof.Proven() != (want != nil) {
				t.Fatalf("result = %+v, want %v; SSA:\n%s", baseline, want, carriedSSA(t, call.Parent()))
			}
			for limit := range 21 {
				budget := proofs.NewSearchBudget(limit)
				got := proveAcquisitionErrorResultWithin(call, budget)
				if resourceFlowExhausted(budget) {
					if !test.queried || got.proof.State != proofs.EvidenceUnknown || got.proof.Reason != resourceReasonBudgetExhausted || got.value != nil {
						t.Fatalf("cut %d = %+v", limit, got)
					}
					continue
				}
				if test.queried && limit == 0 {
					t.Fatal("eligible result lookup ignored allowance")
				}
				if got != baseline {
					t.Fatalf("completed result = %+v, want %+v", got, baseline)
				}
				return
			}
			t.Fatal("result search never completed")
		})
	}
}

func TestAcquisitionErrorResultChildCutoff(t *testing.T) {
	call := acquisitionResultCall(t, acquisitionResultFixture(t).Func("triple"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	got := proveAcquisitionErrorResultWithin(call, pool.Within(1))
	if got.proof.State != proofs.EvidenceUnknown || got.proof.Reason != resourceReasonBudgetExhausted || got.value != nil || pool.Exhausted() {
		t.Fatalf("child cutoff = %+v, parent exhausted %v", got, pool.Exhausted())
	}
	if fresh := proveAcquisitionErrorResultWithin(call, pool.Within(releaseSearchBudget)); fresh != proveAcquisitionErrorResultWithin(call, nil) {
		t.Fatalf("fresh result = %+v", fresh)
	}
}

func TestAcquisitionErrorResultParentCutoff(t *testing.T) {
	call := acquisitionResultCall(t, acquisitionResultFixture(t).Func("pair"))
	pool := proofs.NewSearchBudget(1)
	child := pool.Within(releaseSearchBudget)
	sibling := pool.Within(releaseSearchBudget)
	sibling.Spend()
	sibling.Spend()
	got := proveAcquisitionErrorResultWithin(call, child)
	if got.proof.State != proofs.EvidenceUnknown || got.value != nil {
		t.Fatalf("exhausted shared parent supplied error result: %+v", got)
	}
}

func TestAcquisitionErrorResultFlow(t *testing.T) {
	pkg := acquisitionResultFixture(t)
	for _, name := range []string{"pair", "triple", "leak"} {
		t.Run(name, func(t *testing.T) {
			call := acquisitionResultCall(t, pkg.Func(name))
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			got := evaluateResourceFlow(nil, evidence, call, ssacall.CallResult(call, 0), resourceContract{cleanup: []string{"Close"}})
			if name == "leak" {
				if got.state != proofs.EvidenceProven || got.leak == nil {
					t.Fatalf("leak lost: %+v", got)
				}
			} else if got.state != proofs.EvidenceDisproven || got.leak != nil {
				t.Fatalf("successful cleanup reported: %+v", got)
			}
		})
	}
}

func acquisitionResultCall(t *testing.T, fn *ssa.Function) *ssa.Call {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if ssaflow.CallName(call.Common()) != "Close" {
			return call
		}
	}
	t.Fatal("missing acquisition")
	return nil
}

func acquisitionResultFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "acquisitionresult", `package acquisitionresult
 type resource struct{n int}
 func (*resource) Close(){}
 type failure struct{}
 func (failure) Error()string{return "failure"}
 func acquire()(*resource,error){return &resource{},nil}
 func acquireThree()(*resource,*resource,error){return &resource{},&resource{},nil}
 func acquireOne()*resource{return &resource{}}
 func acquireNoError()(*resource,int){return &resource{},0}
 func acquireNonLast()(*resource,error,int){return &resource{},nil,0}
 func acquireConcrete()(*resource,failure){return &resource{},failure{}}
 func pair(){p,e:=acquire();if e!=nil{return};p.Close()}
 func triple(){p,q,e:=acquireThree();if e!=nil{return};p.Close();q.Close()}
 func discarded(){acquire()}
 func blank(){p,_:=acquire();p.Close()}
 func unused(){p,e:=acquire();_=e;p.Close()}
 func scalar(){p:=acquireOne();p.Close()}
 func noError(){p,_:=acquireNoError();p.Close()}
 func nonLast(){p,_,_:=acquireNonLast();p.Close()}
 func concreteError(){p,_:=acquireConcrete();p.Close()}
 func leak(){p,q,e:=acquireThree();if e!=nil{return};p.n++;q.Close()}
 `)
}

func TestOptionalAcquisitionAllowance(t *testing.T) {
	pkg := optionalAcquisitionFixture(t)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"exact", true},
		{"inverse", true},
		{"leak", true},
		{"otherGuard", false},
		{"otherResource", false},
		{"otherError", false},
		{"boxedError", false},
		{"cyclic", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			call, resource, errValue := acquiredResourceInputs(t, pkg.Func(test.name))
			baseline := proveOptionalAcquisitionWithin(call, resource, errValue, nil)
			if baseline.Proven() != test.want || baseline.proof.State == proofs.EvidenceUnknown {
				t.Fatalf("default diamond proof = %+v; SSA:\n%s", baseline, carriedSSA(t, call.Parent()))
			}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				got := proveOptionalAcquisitionWithin(call, resource, errValue, budget)
				if resourceFlowExhausted(budget) || limit == 0 {
					if got.proof.State != proofs.EvidenceUnknown || got.proof.Reason != resourceReasonBudgetExhausted ||
						got.resourcePhi != nil || got.merge != nil || got.acquisitionBlock != nil || got.acquiredSuccessor != nil {
						t.Fatalf("interrupted diamond retains correlation: %+v", got)
					}
					continue
				}
				if got != baseline {
					t.Fatalf("complete diamond = %+v, want %+v", got, baseline)
				}
				return
			}
			t.Fatal("diamond proof never completed")
		})
	}
}

func TestOptionalAcquisitionChildCutoff(t *testing.T) {
	call, resource, errValue := acquiredResourceInputs(t, optionalAcquisitionFixture(t).Func("exact"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	got := proveOptionalAcquisitionWithin(call, resource, errValue, pool.Within(2))
	if got.proof.State != proofs.EvidenceUnknown || got.proof.Reason != resourceReasonBudgetExhausted || got.resourcePhi != nil || pool.Exhausted() {
		t.Fatalf("optional child cutoff = %+v; parent exhausted %v", got, pool.Exhausted())
	}
	if fresh := proveOptionalAcquisitionWithin(call, resource, errValue, pool.Within(releaseSearchBudget)); !fresh.Proven() {
		t.Fatalf("fresh diamond = %+v", fresh)
	}
}

func TestOptionalAcquisitionFlow(t *testing.T) {
	pkg := optionalAcquisitionFixture(t)
	for _, name := range []string{"exact", "inverse", "leak"} {
		t.Run(name, func(t *testing.T) {
			call, resource, _ := acquiredResourceInputs(t, pkg.Func(name))
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			got := evaluateResourceFlow(nil, evidence, call, resource, resourceContract{cleanup: []string{"Close"}})
			if name == "leak" {
				if got.state != proofs.EvidenceProven || got.leak == nil {
					t.Fatalf("optional leak lost = %+v", got)
				}
			} else if got.state != proofs.EvidenceDisproven || got.leak != nil {
				t.Fatalf("optional cleanup reported = %+v", got)
			}
		})
	}
}

func acquiredResourceInputs(t *testing.T, fn *ssa.Function) (*ssa.Call, ssa.Value, ssa.Value) {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if ssaflow.CallName(call.Common()) != "acquire" {
			continue
		}
		var resource, errValue ssa.Value
		for _, extract := range ssaflow.InstructionsOf[*ssa.Extract](fn) {
			if extract.Tuple != call {
				continue
			}
			switch extract.Index {
			case 0:
				resource = extract
			case 1:
				errValue = extract
			}
		}
		if resource == nil || errValue == nil {
			t.Fatal("missing acquisition results")
		}
		return call, resource, errValue
	}
	t.Fatal("missing acquisition")
	return nil, nil, nil
}

func optionalAcquisitionFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "optionalacquisition", `package optionalacquisition
 type resource struct{n int}
 func (*resource) Close(){}
 func acquire()(*resource,error){return &resource{},nil}
 func exact(flag int){var p *resource;var e error;if flag==1{p,e=acquire()};if flag==1{if e!=nil{return};p.Close()}}
 func inverse(flag int){var p *resource;var e error;if flag!=0{p,e=acquire()};if flag==0{return};if e!=nil{return};p.Close()}
 func leak(flag int){var p *resource;var e error;if flag==1{p,e=acquire()};if flag==1{if e!=nil{return};p.n++}}
 func otherGuard(flag,other int){var p *resource;var e error;if flag==1{p,e=acquire()};if other==1{if e!=nil{return};p.Close()}}
 func otherResource(flag int,other *resource){p:=other;var e error;if flag==1{p,e=acquire()};if flag==1{if e!=nil{return};p.Close()}}
 func otherError(flag int,other error){var p *resource;e:=other;if flag==1{p,e=acquire()};if flag==1{if e!=nil{return};p.Close()}}
 type failure struct{}
 func (*failure) Error()string{return "failure"}
 func boxedError(flag int){var p *resource;var e error=(*failure)(nil);if flag==1{p,e=acquire()};if flag==1{if e!=nil{return};p.Close()}}
 func cyclic(flag int,done bool){for{var p *resource;var e error;if flag==1{p,e=acquire()};if flag==1{if e!=nil{return};p.Close()};if done{return}}}
 `)
}
