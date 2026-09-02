package lifecyclefacts

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
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
		Analyzer: Summaries{callee: {Closed: parameterMaskFor(0)}},
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

	pass.ResultOf[Analyzer] = Summaries{callee: {}}
	rejected := NewLifecycleEvidence(pass, "test", "test/check").Prove(request)
	if rejected.State != ssaflow.EvidenceDisproven || rejected.Provenance != ssaflow.EvidenceFromImportedFact {
		t.Fatalf("empty summary proof = %#v, want imported disproof", rejected)
	}

	pass.ResultOf[Analyzer] = Summaries{}
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
			Analyzer: Summaries{instruction.Common().StaticCallee(): {Closed: parameterMaskFor(0)}},
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

func Guarded(value *owner) {
	if value != nil && value.body != nil {
		value.body.Close()
	}
}
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "Exported", want: true},
		{name: "Conditional"},
		{name: "Sibling"},
		{name: "Guarded", want: true},
	} {
		function := pkg.Func(test.name)
		fact := summarize(&analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}, function)
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

// Retention is over-approximate: storing, capturing, returning, or handing
// the parameter to an opaque callee marks it, while invoking it does not.
func TestLifecycleSummaryRetention(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

var handlers []func()

func Register(handler func()) { handlers = append(handlers, handler) }
func Invoke(handler func()) { handler() }
func Return(handler func()) func() { return handler }
func keep(handler func()) { handlers = append(handlers, handler) }
func read(handler func()) { _ = handler == nil }
func ViaKeeper(handler func()) { keep(handler) }
func ViaReader(handler func()) { read(handler) }
func DeferCapture(handler func()) { defer func() { handler() }() }

type holder struct{ run func() }

func IntoLocalStruct(handler func()) { h := holder{run: handler}; h.run() }

type sink interface{ Add(func()) }

var registry sink

func ViaInterface(handler func()) { registry.Add(handler) }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string]bool{"Register": true, "Invoke": false, "Return": true, "ViaKeeper": true, "ViaReader": false} {
		if got := summarize(pass, pkg.Func(name)).Retained.contains(0); got != want {
			t.Errorf("%s Retained parameter = %t, want %t", name, got, want)
		}
		if got := summarize(pass, pkg.Func(name)).Stored.contains(0); got != (want && name != "Return") {
			t.Errorf("%s Stored parameter = %t, want %t", name, got, want && name != "Return")
		}
	}
	// Deferring a literal that captured the parameter, or handing it to an
	// interface, retains loosely but does not store.
	for name, want := range map[string][2]bool{"DeferCapture": {true, false}, "ViaInterface": {true, false}, "IntoLocalStruct": {true, false}} {
		retained, stored := want[0], want[1]
		fact := summarize(pass, pkg.Func(name))
		if got := fact.Retained.contains(0); got != retained {
			t.Errorf("%s Retained parameter = %t, want %t", name, got, retained)
		}
		if got := fact.Stored.contains(0); got != stored {
			t.Errorf("%s Stored parameter = %t, want %t", name, got, stored)
		}
	}
}

// A constructor owns the fields it fills with resources it acquired, not
// with resources it received; a method releases the fields it closes on every
// return.
func TestLifecycleSummaryFieldMasks(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

import "os"

type Journal struct {
	file *os.File
	name string
}

func Open(path string) (*Journal, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &Journal{file: file, name: path}, nil
}

func Wrap(file *os.File) *Journal { return &Journal{file: file} }

func (j *Journal) Close() error { return j.file.Close() }

func (j *Journal) MaybeClose(ok bool) error {
	if ok {
		return j.file.Close()
	}
	return nil
}
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	if got := summarize(pass, pkg.Func("Open")).OwnedFields; !got.contains(0) || got.contains(1) {
		t.Errorf("Open OwnedFields = %#x, want field 0 only", uint64(got))
	}
	if got := summarize(pass, pkg.Func("Wrap")).OwnedFields; got != 0 {
		t.Errorf("Wrap OwnedFields = %#x, want none", uint64(got))
	}
	journal := pkg.Type("Journal").Type()
	closeMethod := pkg.Prog.LookupMethod(types.NewPointer(journal), pkg.Pkg, "Close")
	if got := summarize(pass, closeMethod).ReleasedFields; !got.contains(0) {
		t.Errorf("Close ReleasedFields = %#x, want field 0", uint64(got))
	}
	maybe := pkg.Prog.LookupMethod(types.NewPointer(journal), pkg.Pkg, "MaybeClose")
	if got := summarize(pass, maybe).ReleasedFields; got != 0 {
		t.Errorf("MaybeClose ReleasedFields = %#x, want none", uint64(got))
	}
}

// A parameter stored in a returned struct is a view when no method of that
// type releases the field, and an owner otherwise.
func TestLifecycleSummaryReturnedViews(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

import "os"

type Reader struct{ file *os.File }

func NewReader(file *os.File) *Reader { return &Reader{file: file} }

type Owner struct{ file *os.File }

func Adopt(file *os.File) *Owner { return &Owner{file: file} }

func (o *Owner) Close() error { return o.file.Close() }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	summaries := Summaries{}
	for _, name := range []string{"NewReader", "Adopt"} {
		summaries[pkg.Func(name)] = summarize(pass, pkg.Func(name))
	}
	owner := pkg.Type("Owner").Type()
	closeMethod := pkg.Prog.LookupMethod(types.NewPointer(owner), pkg.Pkg, "Close")
	summaries[closeMethod] = summarize(pass, closeMethod)
	if got := returnedViews(pass, pkg.Func("NewReader"), summaries[pkg.Func("NewReader")], summaries); !got.contains(0) {
		t.Errorf("NewReader ReturnedView = %#x, want parameter 0", uint64(got))
	}
	if got := returnedViews(pass, pkg.Func("Adopt"), summaries[pkg.Func("Adopt")], summaries); got != 0 {
		t.Errorf("Adopt ReturnedView = %#x, want none", uint64(got))
	}
}

func buildLifecycleTestSSA(t *testing.T, source string) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "example.com/lifecyclefactstest", source)
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
