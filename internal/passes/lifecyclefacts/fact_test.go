package lifecyclefacts

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
	ssaBuild "golang.org/x/tools/go/ssa/ssautil"
)

func TestParameterMask(t *testing.T) {
	mask := parameterMaskFor(1) | parameterMaskFor(63)
	for _, test := range []struct {
		index int
		want  bool
	}{
		{index: -1},
		{index: 0},
		{index: 1, want: true},
		{index: 63, want: true},
		{index: 64},
	} {
		if got := mask.contains(test.index); got != test.want {
			t.Errorf("mask.contains(%d) = %t, want %t", test.index, got, test.want)
		}
	}
}

func TestLifecycleEvidenceImportedProvenanceAndUncertainty(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type closer struct{}

func (*closer) Close() {}
func helper(value *closer) {}
func caller(value *closer) { helper(value) }
`)
	caller := pkg.Func("caller")
	instruction := findLifecycleCall(t, caller, "helper")
	callee := instruction.Common().StaticCallee()
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{
		Analyzer: summarySet{callee: {Closed: parameterMaskFor(0)}},
	}}
	request := EvidenceRequest{
		Instruction: instruction,
		Target:      caller.Params[0],
		SelectMask: func(fact Fact) ParameterMask {
			return fact.Closed
		},
	}
	proof := NewLifecycleEvidence(pass, "test", "test/check").Prove(request)
	if !proof.Proven() || proof.Provenance != ssaflow.EvidenceFromImportedFact || proof.Reason != reasonLifecycleSummary {
		t.Fatalf("imported proof = %#v, want lifecycle-summary provenance", proof)
	}

	pass.ResultOf[Analyzer] = summarySet{callee: {}}
	rejected := NewLifecycleEvidence(pass, "test", "test/check").Prove(request)
	if rejected.State != ssaflow.EvidenceDisproven || rejected.Provenance != ssaflow.EvidenceFromImportedFact {
		t.Fatalf("empty summary proof = %#v, want imported disproof", rejected)
	}

	pass.ResultOf[Analyzer] = summarySet{}
	unknown := NewLifecycleEvidence(pass, "test", "test/check").Prove(request)
	if unknown.State != ssaflow.EvidenceUnknown || unknown.Reason != ssaflow.EvidenceUnavailable {
		t.Fatalf("missing summary proof = %#v, want unknown", unknown)
	}
}

func TestLifecycleEvidenceStrictImportedProjectionIsOptIn(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type closer struct{}
type owner struct { body *closer }

func acquire() *owner { return nil }
func helper(*closer) {}
func accepted() {
	value := acquire()
	helper(value.body)
}
func reassigned() {
	value := acquire()
	value.body = &closer{}
	helper(value.body)
}
`)
	prove := func(t *testing.T, functionName string, enabled bool) ssaflow.Proof {
		t.Helper()
		function := pkg.Func(functionName)
		acquisition := findLifecycleCall(t, function, "acquire")
		instruction := findLifecycleCall(t, function, "helper")
		pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{
			Analyzer: summarySet{instruction.Common().StaticCallee(): {Closed: parameterMaskFor(0)}},
		}}
		return NewLifecycleEvidence(pass, "test", "test/check").Prove(EvidenceRequest{
			Instruction:              instruction,
			Target:                   acquisition,
			StrictImportedProjection: enabled,
			SelectMask: func(fact Fact) ParameterMask {
				return fact.Closed
			},
		})
	}

	withoutOptIn := prove(t, "accepted", false)
	if withoutOptIn.Proven() {
		t.Fatalf("ordinary imported proof = %#v, want projection rejected", withoutOptIn)
	}
	projected := prove(t, "accepted", true)
	if !projected.Proven() || projected.Reason != reasonLifecycleSummaryProjectedArgument ||
		projected.Provenance != ssaflow.EvidenceFromImportedFact {
		t.Fatalf("projected imported proof = %#v, want strict projected lifecycle summary", projected)
	}
	reassigned := prove(t, "reassigned", true)
	if reassigned.Proven() {
		t.Fatalf("reassigned projection proof = %#v, want rejected", reassigned)
	}
}

func TestLifecycleSummaryDeferredCallbackBoundaries(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type closer struct{}
func (*closer) Close() {}

type owner struct { body *closer }

func Exported(value *owner) {
	defer func() { value.body.Close() }()
}

func Conditional(value *owner, enabled bool) {
	defer func() {
		if enabled {
			value.body.Close()
		}
	}()
}

func Sibling(value, other *owner) {
	defer func() { other.body.Close() }()
}
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "Exported", want: true},
		{name: "Conditional"},
		{name: "Sibling"},
	} {
		function := pkg.Func(test.name)
		fact := summarize(&analysis.Pass{}, function)
		if got := fact.Closed.contains(0); got != test.want {
			t.Errorf("%s Closed parameter = %t, want %t", test.name, got, test.want)
		}
	}
}

// Commit and Rollback are the cleanup pair of a transaction; each is
// summarized independently, so a helper that settles the transaction one way
// or the other on different paths exports neither mask.
func TestLifecycleSummaryTransactionMasks(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type transaction struct{}
func (*transaction) Commit() error { return nil }
func (*transaction) Rollback() error { return nil }

func Finish(tx *transaction) { _ = tx.Rollback() }

func Save(tx *transaction, ok bool) error {
	if ok {
		return tx.Commit()
	}
	return tx.Rollback()
}
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	finish := summarize(pass, pkg.Func("Finish"))
	if !finish.RolledBack.contains(0) || finish.Committed.contains(0) {
		t.Errorf("Finish masks = committed %t rolled back %t, want rolled back only", finish.Committed.contains(0), finish.RolledBack.contains(0))
	}
	save := summarize(pass, pkg.Func("Save"))
	if save.RolledBack.contains(0) || save.Committed.contains(0) {
		t.Errorf("Save masks = committed %t rolled back %t, want neither", save.Committed.contains(0), save.RolledBack.contains(0))
	}
}

func buildLifecycleTestSSA(t *testing.T, source string) *ssa.Package {
	t.Helper()
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, "lifecyclefacts.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	pkg, _, err := ssaBuild.BuildPackage(
		&types.Config{Importer: importer.Default()},
		files,
		types.NewPackage("example.com/lifecyclefactstest", "lifecyclefactstest"),
		[]*ast.File{file},
		ssa.SanityCheckFunctions,
	)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func findLifecycleCall(t *testing.T, function *ssa.Function, name string) *ssa.Call {
	t.Helper()
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if ok && ssaflow.CallName(call.Common()) == name {
				return call
			}
		}
	}
	t.Fatalf("call %s not found", name)
	return nil
}
