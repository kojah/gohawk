package lifecyclefacts

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestLifecycleEvidenceImportedSummaryThroughImmutableCapture(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type closer struct{}

func helper(*closer) {}

func accepted(value *closer) {
	defer func() { helper(value) }()
}

func reassigned(value, other *closer) {
	defer func() { helper(value) }()
	value = other
}

func sibling(value, other *closer) {
	defer func() { helper(other) }()
}
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "accepted", want: true},
		{name: "reassigned"},
		{name: "sibling"},
	} {
		function := pkg.Func(test.name)
		deferred := findDefer(t, function)
		helper := findAnonymousCall(t, function, "helper")
		pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{
			Analyzer: Summaries{helper.Common().StaticCallee(): {Closed: parameterMaskFor(0)}},
		}}
		completion := ssaflow.CompletionRequest{Instruction: deferred, Target: function.Params[0], Methods: []string{"Close"}}
		proof := NewLifecycleEvidence(pass, "test", "test/check").Prove(EvidenceRequest{
			Instruction: deferred,
			Target:      function.Params[0],
			Completion:  &completion,
			SelectMask: func(fact Fact) ParameterMask {
				return fact.Closed
			},
		})
		if got := proof.Proven(); got != test.want {
			t.Errorf("%s imported capture proof = %#v, proven %t, want %t", test.name, proof, got, test.want)
		}
		if test.want && proof.Reason != reasonLifecycleSummaryCapturedArgument {
			t.Errorf("%s imported capture reason = %q, want %q", test.name, proof.Reason, reasonLifecycleSummaryCapturedArgument)
		}
	}
}

func TestTypeCanReleaseUsesLifecycleVocabulary(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type committer struct{}
func (*committer) Commit() error { return nil }

type rollbacker struct{}
func (*rollbacker) Rollback() error { return nil }

type waiter struct{}
func (*waiter) Wait() {}

type observer struct{}
func (*observer) Observe() {}
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "committer", want: true},
		{name: "rollbacker", want: true},
		{name: "waiter", want: true},
		{name: "observer"},
	} {
		got := typeCanRelease(types.NewPointer(pkg.Type(test.name).Type()))
		if got != test.want {
			t.Errorf("typeCanRelease(*%s) = %t, want %t", test.name, got, test.want)
		}
	}
}

func findDefer(t *testing.T, function *ssa.Function) *ssa.Defer {
	t.Helper()
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if deferred, ok := instruction.(*ssa.Defer); ok {
				return deferred
			}
		}
	}
	t.Fatal("defer not found")
	return nil
}

func findAnonymousCall(t *testing.T, function *ssa.Function, name string) *ssa.Call {
	t.Helper()
	for _, member := range function.AnonFuncs {
		for _, block := range member.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if ok && ssaflow.CallName(call.Common()) == name {
					return call
				}
			}
		}
	}
	t.Fatalf("anonymous call %s not found", name)
	return nil
}
