package lifecyclefacts

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// A struct parameter is passed by value and spilled into a local cell before
// its fields are selected. The summary must still see a cleanup call on the
// field as a cleanup of the parameter, for the parameter itself, a value
// receiver, a local copy, an array, and an array inside a struct alike. A
// branch without the call, or a call on a field of some other local, proves
// nothing about the parameter.
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
		want     bool
	}{
		"Direct":    {pkg.Func("Direct"), true},
		"Copied":    {pkg.Func("Copied"), true},
		"Finish":    {pkg.Prog.LookupMethod(pkg.Type("job").Type(), pkg.Pkg, "Finish"), true},
		"OneBranch": {pkg.Func("OneBranch"), false},
		"Sibling":   {pkg.Func("Sibling"), false},
		"Indexed":   {pkg.Func("Indexed"), true},
		"Nested":    {pkg.Func("Nested"), true},
	}
	for name, test := range cases {
		fact := summarize(pass, newRetentionCache(), test.function)
		if got := fact.Closed.contains(0); got != test.want {
			t.Errorf("%s: Closed parameter 0 = %t, want %t (fact %+v)", name, got, test.want, fact)
		}
	}
}
