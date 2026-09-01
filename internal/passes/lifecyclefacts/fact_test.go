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
