package lifecyclefacts

import (
	"go/types"
	"slices"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// A cleanup of a replacement aggregate must never settle the original
// parameter's field. A saved field value keeps its own earlier snapshot.
func TestSpillReplacementCleanupContracts(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
 type closer struct{}
 func(*closer)Close()error{return nil}
 type job struct {file *closer}
 func Replacement(j,other job){j=other;j.file.Close()}
 func Earlier(j,other job){saved:=j.file;j=other;saved.Close()}
 func WrappedEarlier(j,other job){saved:=j.file;j=other;var c interface{Close()error}=saved;c.Close()}
 func RestoredAfterRead(j,other job){original:=j;j=other;saved:=j.file;j=original;saved.Close()}
 func Ambiguous(j,other job,flag bool){if flag{j=other};j.file.Close()}
 func Agreeing(j job,flag bool){original:=j;if flag{j=original};j.file.Close()}
 `)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for _, test := range []struct {
		name       string
		parameters []int
	}{
		{"Replacement", []int{1}},
		{"Earlier", []int{0}},
		{"WrappedEarlier", []int{0}},
		{"RestoredAfterRead", []int{1}},
		{"Ambiguous", nil},
		{"Agreeing", []int{0}},
	} {
		fact := summarize(pass, pkg.Func(test.name))
		var got []int
		for _, discharge := range fact.Discharges {
			if discharge.Method == "Close" && discharge.Path == "field:0" {
				got = append(got, discharge.Parameter)
			}
		}
		if !slices.Equal(got, test.parameters) || fact.MethodMask("Close") != 0 {
			t.Errorf("%s fields=%v, whole mask=%v, want%v", test.name, got, fact.MethodMask("Close"), test.parameters)
		}
	}
}
