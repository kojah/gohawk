package lifecyclefacts

import (
	"go/types"
	"slices"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// A struct parameter is passed by value and spilled into a local cell before
// its fields are selected. The summary must still see a cleanup call on the
// field, and claim it at the field's own path rather than for the whole
// parameter, for the parameter itself, a value receiver, a local copy, an
// array, and an array inside a struct alike. A branch without the call, or a
// call on a field of some other local, proves nothing about the parameter.
func TestLifecycleSummaryClosesFieldOfByValueParameter(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

import "os"

type job struct{ out *os.File }

func Direct(j job) error { return j.out.Close() }
func Copied(j job) error { k := j; return k.out.Close() }
func (j job) Finish() error { return j.out.Close() }
func OneBranch(j job, flag bool) error {
	if flag {
		return nil
	}
	return j.out.Close()
}
func Sibling(j job, other *os.File) error {
	k := job{out: other}
	return k.out.Close()
}
func Indexed(files [2]*os.File) error { return files[0].Close() }
type batch struct{ files [2]*os.File }
func Nested(b batch) error { return b.files[0].Close() }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	cases := map[string]struct {
		function *ssa.Function
		path     string
		want     bool
	}{
		"Direct":    {pkg.Func("Direct"), "field:0", true},
		"Copied":    {pkg.Func("Copied"), "field:0", true},
		"Finish":    {pkg.Prog.LookupMethod(pkg.Type("job").Type(), pkg.Pkg, "Finish"), "field:0", true},
		"OneBranch": {pkg.Func("OneBranch"), "field:0", false},
		"Sibling":   {pkg.Func("Sibling"), "field:0", false},
		"Indexed":   {pkg.Func("Indexed"), "index:0", true},
		"Nested":    {pkg.Func("Nested"), "field:0/index:0", true},
	}
	for name, test := range cases {
		fact := summarize(pass, test.function)
		got := slices.Contains(fact.Discharges, Discharge{Parameter: 0, Method: "Close", Path: test.path})
		if got != test.want {
			t.Errorf("%s: discharge of Close at %q = %t, want %t (fact %+v)", name, test.path, got, test.want, fact.Discharges)
		}
		if fact.Closed != 0 {
			t.Errorf("%s: a field cleanup must not claim the whole parameter", name)
		}
	}
}
