package cancellationownership

import (
	"reflect"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const cancellationOwnerFixture = `package ownerbudget
 type owner struct{cancel func();stop func();n int}
 func direct(cancel func())*owner{return &owner{cancel:cancel}}
 func closure(cancel func())*owner{return &owner{stop:func(){cancel()}}}
 func nested(cancel func())*owner{return &owner{stop:func(){func(){cancel()}()}}}
 func beforeStore(cancel func())*owner{var saved func();o:=&owner{stop:func(){saved()}};saved=cancel;return o}
 func rewritten(cancel func())*owner{saved:=cancel;o:=&owner{stop:func(){saved()}};saved=func(){};return o}
 func read(cancel func())*owner{saved:=cancel;o:=&owner{stop:func(){saved()}};saved();return o}
 func multi(cancel func())*owner{saved:=cancel;o:=&owner{stop:func(){saved()}};_ =func(){saved()};return o}
 func opaque(cancel func(),publish func(*owner))*owner{o:=&owner{cancel:cancel};publish(o);return o}
 func twoFields(cancel func())*owner{return &owner{cancel:cancel,stop:cancel}}
 func fieldRead(cancel func())*owner{o:=&owner{cancel:cancel};o.cancel();return o}
 func otherField(cancel func())*owner{o:=&owner{cancel:cancel};o.n=3;_ =o.n;return o}
 func wrong(cancel,other func())*owner{return &owner{stop:func(){other()}}}
`

func TestCancellationOwnerAllowanceAndContracts(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ownerbudget", cancellationOwnerFixture)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"direct", true},
		{"closure", true},
		{"nested", true},
		{"beforeStore", false},
		{"rewritten", false},
		{"read", false},
		{"multi", false},
		{"opaque", false},
		{"twoFields", false},
		{"fieldRead", false},
		{"otherField", true},
		{"wrong", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			want := proveCancellationOwnerWithin(fn.Params[0], nil)
			if !want.Known() || want.Proven() != test.want {
				t.Fatalf("owner=%+v want%v", want, test.want)
			}
			checkCancellationOwnerCutoffs(t, fn.Params[0], want)
		})
	}
}

func checkCancellationOwnerCutoffs(t *testing.T, cancel ssa.Value, want cancellationOwnerProof) {
	t.Helper()
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		pool := proofs.NewSearchBudget(4 * proofs.SummaryBudget)
		child := pool.Within(limit)
		got := proveCancellationOwnerWithin(cancel, child)
		if child.Exhausted() {
			if got.Known() || got.Owner != nil || got.Reason != proofs.EvidenceBudgetExhausted || pool.Exhausted() {
				t.Fatalf("cut%d retained%+v", limit, got)
			}
			fresh := proveCancellationOwnerWithin(cancel, pool.Within(proofs.SummaryBudget))
			if !reflect.DeepEqual(fresh, want) {
				t.Fatalf("fresh after cut%d=%+v want%+v", limit, fresh, want)
			}
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("complete%d=%+v want%+v", limit, got, want)
		}
		return
	}
	t.Fatal("owner query never completed")
}

func TestCancellationOwnerLargeCensusesShareAllowance(t *testing.T) {
	for _, test := range []struct{ name, body string }{
		{"capture", `o:=&owner{stop:func(){` + strings.Repeat("_ =cancel;", proofs.QueryBudget+1) + `cancel()}};return o`},
		{"ordering", `n:=0;` + strings.Repeat("n++;", proofs.QueryBudget+1) + `return &owner{n:n,stop:func(){cancel()}}`},
		{"fields", `o:=&owner{cancel:cancel};` + strings.Repeat("o.n++;", proofs.QueryBudget+1) + `return o`},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := `package ownerbudget;type owner struct{cancel func();stop func();n int};func padded(cancel func())*owner{` + test.body + `}`
			fn := ssaflowtest.BuildPackage(t, "ownerbudget", source).Func("padded")
			pool := proofs.NewSearchBudget(20 * proofs.SummaryBudget)
			child := pool.Within(proofs.QueryBudget)
			got := proveCancellationOwnerWithin(fn.Params[0], child)
			if got.Known() || got.Owner != nil || !child.Exhausted() || pool.Exhausted() {
				t.Fatalf("large census retained%+v", got)
			}
			fresh := proveCancellationOwnerWithin(fn.Params[0], pool.Within(10*proofs.SummaryBudget))
			if !fresh.Proven() || fresh.Owner == nil {
				t.Fatalf("fresh census=%+v", fresh)
			}
		})
	}
}

func TestCancellationOwnerCutoffKeepsExactCleanup(t *testing.T) {
	fn := ssaflowtest.BuildPackage(t, "ownerbudget", `package ownerbudget
 type owner struct{cancel func()}
 func root(cancel func())*owner{o:=&owner{cancel:cancel};cancel();return o}`).Func("root")
	query := &cancellationClassifier{cancel: fn.Params[0], pool: proofs.NewSearchBudget(0), actions: make(map[ssa.Instruction]cancellationAction)}
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	if label := query.classifyAction(call); label.action != cancellationActionRelease {
		t.Fatalf("exact cleanup=%+v", label)
	}
	returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
	if label := query.returnLabel(returned); label.action != cancellationActionUnknown || label.reason != reasonLabelOwnerUnavailable {
		t.Fatalf("cut owner return=%+v", label)
	}
	fresh := &cancellationClassifier{cancel: fn.Params[0]}
	if label := fresh.returnLabel(returned); label.action != cancellationActionTransfer {
		t.Fatalf("fresh owner return=%+v", label)
	}
}

func TestCancellationOwnerCutoffCannotBecomeLoss(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ownerbudget", `package ownerbudget
 type owner struct{cancel func()}
 func dropped(cancel func(),fail bool)*owner{o:=&owner{cancel:cancel};if fail{return nil};return o}
 func released(cancel func())*owner{o:=&owner{cancel:cancel};cancel();return o}`)
	for _, test := range []struct {
		name          string
		cut, complete ssaflow.ObligationOutcome
	}{
		{"dropped", ssaflow.ObligationUncertain, ssaflow.ObligationViolated},
		{"released", ssaflow.ObligationHonored, ssaflow.ObligationHonored},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			for _, limit := range []int{0, cancellationPoolBudget} {
				query := &cancellationClassifier{
					cancel: fn.Params[0], pool: proofs.NewSearchBudget(limit),
					actions: make(map[ssa.Instruction]cancellationAction),
				}
				got := ssaflow.EvaluateObligation(ssaflow.ObligationFlow{
					Start: fn.Blocks[0].Instrs[0], NonNil: fn.Params[0], Instruction: query.obligation, Edge: query.edgeObligation,
				})
				want := test.complete
				if limit == 0 {
					want = test.cut
				}
				if got != want {
					t.Fatalf("allowance%d outcome=%v want%v", limit, got, want)
				}
			}
		})
	}
}
