package heapmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestAggregateWriteDoesNotKeepStaleFields(t *testing.T) {
	for _, test := range []struct {
		name, setup, write, selection string
		exact, keepSibling            bool
	}{
		{"possibleWhole", `dst:=&a;if flag{dst=&b}`, `*dst=holder{inner:part{value:replacement},sibling:old}`, `inner.value`, false, false},
		{"possibleNested", `dst:=&a.inner;if flag{dst=&b.inner}`, `*dst=part{value:replacement}`, `inner.value`, false, true},
		{"definiteNested", `dst:=&a.inner`, `*dst=part{value:replacement}`, `inner.value`, true, true},
		{"mixedValue", `dst:=&a.inner;v:=part{value:replacement};if flag{v=part{value:old}}`, `*dst=v`, `inner.value`, false, true},
		{"definiteWhole", `dst:=&a`, `*dst=holder{inner:part{value:replacement},sibling:old}`, `inner.value`, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "aggregatewrites", `package aggregatewrites
type part struct{value *int}
type holder struct{inner part;sibling *int}
func probe(flag bool)(**int,**int,*int,*int,**int){
 old:=new(int);replacement:=new(int)
 a:=holder{inner:part{value:old},sibling:old};b:=a;_=b
 before:=a;`+test.setup+`;`+test.write+`
 return &a.`+test.selection+`,&before.inner.value,old,replacement,&a.sibling
}`)
			fn := pkg.Func("probe")
			t.Log(RenderRegions(fn))
			returns := ssaflow.InstructionsOf[*ssa.Return](fn)
			if len(returns) != 1 {
				t.Fatalf("return count=%d, want one", len(returns))
			}
			at := returns[0]
			v := at.Results
			contents, exact := ContentValue(v[0], at)
			if exact != test.exact || exact && !DefinitelySame(contents, v[3]) {
				t.Fatalf("aggregate contents=%v exact=%v want exact replacement=%v", contents, exact, test.exact)
			}
			contents, exact = ContentValue(v[1], at)
			if !exact || !DefinitelySame(contents, v[2]) {
				t.Fatal("aggregate write changed an earlier snapshot")
			}
			if test.keepSibling {
				contents, exact = ContentValue(v[4], at)
				if !exact || !DefinitelySame(contents, v[2]) {
					t.Fatal("nested aggregate write changed an untouched sibling")
				}
			}
		})
	}
}

func TestDefiniteAggregateZeroClearsStoredFields(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "aggregatezero", `package aggregatezero
type part struct{value *int}
func probe()*int{a:=part{value:new(int)};a=part{};return a.value}
`)
	fn := pkg.Func("probe")
	returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
	if !regionsOfFunction(fn).contentIsNil(returned.Results[0], nil, returned) {
		t.Fatal("definite aggregate zero did not clear its field")
	}
}

func TestPossibleAggregateWriteKeepsFormerPointeeStorage(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "aggregatepointee", `package aggregatepointee
type item struct{data *int}
type part struct{value *item}
func probe(flag bool)(**int,*int){
 data:=new(int);old:=&item{data:data};a:=part{value:old};b:=a
 dst:=&a;if flag{dst=&b};*dst=part{value:&item{data:new(int)}}
 return &old.data,data
}`)
	fn := pkg.Func("probe")
	returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
	contents, exact := ContentValue(returned.Results[0], returned)
	if !exact || !DefinitelySame(contents, returned.Results[1]) {
		t.Fatal("aggregate replacement modified its former pointee")
	}
}
