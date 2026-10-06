package lifecyclefacts

import (
	"go/types"
	"slices"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestForwardedDischargePaths(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
 type closer struct{}
 func(*closer)Close()error{return nil}
 type job struct{file,other *closer}
 func closeOne(p *closer){p.Close()}
 func Earlier(j,replacement job){saved:=j.file;j=replacement;defer closeOne(saved)}
 func Replacement(j,replacement job){j=replacement;defer closeOne(j.file)}
 func Sibling(j job){defer closeOne(j.other)}
 func Ambiguous(j,replacement job,flag bool){if flag{j=replacement};defer closeOne(j.file)}
 `)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for _, test := range []struct {
		name       string
		discharges []Discharge
	}{
		{"Earlier", []Discharge{{Parameter: 0, Method: "Close", Path: "field:0"}}},
		{"Replacement", []Discharge{{Parameter: 1, Method: "Close", Path: "field:0"}}},
		{"Sibling", []Discharge{{Parameter: 0, Method: "Close", Path: "field:1"}}},
		{"Ambiguous", nil},
	} {
		fact := summarize(pass, pkg.Func(test.name))
		if !slices.Equal(fact.Discharges, test.discharges) {
			t.Errorf("%s discharges=%+v, want %+v", test.name, fact.Discharges, test.discharges)
		}
	}
}
