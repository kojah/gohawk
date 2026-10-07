package heapmodel

import (
	"go/types"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
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

func TestSummaryWriteInvalidatesAggregateAndKeepsEarlierCopy(t *testing.T) {
	for _, test := range []struct {
		name, path  string
		must, exact bool
	}{
		{"possibleField", "field:1", false, false},
		{"definiteField", "field:1", true, true},
		{"possibleElement", "field:0/index:0", false, false},
		{"possibleWildcard", "field:0/index:*", false, false},
		{"definiteWildcard", "field:0/index:*", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "summarywrite", `package summarywrite
type holder struct{values [1]*int;field *int;sibling *int}
func possibleWrite(*holder,*int)
func probe()(**int,**int,**int,*int,*int){
 old:=new(int);replacement:=new(int)
 source:=holder{values:[1]*int{old},field:old,sibling:old};owner:=new(holder);*owner=source
 before:=*owner;possibleWrite(owner,replacement);after:=*owner
 `+summaryWriteReturn(test.path)+`
}`)
			RegisterHeapSummary(pkg.Func("possibleWrite"), HeapSummary{Edges: []HeapEdge{{
				From: HeapSlot{Root: HeapRoot{Kind: HeapParameter, Index: 0}, Path: test.path},
				To:   HeapTarget{Kind: HeapTargetSlot, Slot: HeapSlot{Root: HeapRoot{Kind: HeapParameter, Index: 1}}},
				Must: test.must,
			}}})
			fn := pkg.Func("probe")
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			returns := ssaflow.InstructionsOf[*ssa.Return](fn)
			if len(returns) != 1 {
				t.Fatalf("return count=%d, want one", len(returns))
			}
			returned := returns[0]
			values := returned.Results
			checkSummarySlotContents(t, returned, test.exact)
			for _, address := range values[1:3] {
				contents, exact := ContentValue(address, returned)
				if !exact || !DefinitelySame(contents, values[3]) {
					t.Fatal("summary write changed earlier snapshot or sibling")
				}
			}
		})
	}
}

func summaryWriteReturn(path string) string {
	if strings.Contains(path, "index:") {
		return `return &after.values[0],&before.values[0],&after.sibling,old,replacement`
	}
	return `return &after.field,&before.field,&after.sibling,old,replacement`
}

// Possible containment may retain historical aggregate objects even after a
// definite replacement. Exact content and positive possible contents are the
// guarantees this test needs; it does not require negative may evidence.
func checkSummarySlotContents(t *testing.T, returned *ssa.Return, must bool) {
	t.Helper()
	v := returned.Results
	contents, exact := ContentValue(v[0], returned)
	if must {
		if !exact || !DefinitelySame(contents, v[4]) {
			t.Fatal("definite summary write lost exact replacement")
		}
	} else {
		if exact {
			t.Fatalf("possible summary write established exact contents %v", contents)
		}
		retained, known := ContainsAt(v[0], v[3], returned)
		if !known || !retained {
			t.Fatal("possible summary write lost prior contents")
		}
	}
	retained, known := ContainsAt(v[0], v[4], returned)
	if !known || !retained {
		t.Fatal("summary write lost possible replacement")
	}
}

func TestWildcardWriteKeepsUnwrittenElementPossibilities(t *testing.T) {
	for _, test := range []struct {
		body  string
		exact bool
	}{
		{`var a [2]*int;a[i]=p`, false},
		{`a:=supplied;a[i]=p`, false},
		{`var a [2]*int;a[0]=p`, true},
		{`a:=supplied;a[0]=p`, true},
	} {
		t.Run(test.body, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "wildcardwrite", `package wildcardwrite
func probe(i int,p *int,supplied [2]*int)(*int,*int){`+test.body+`;return a[0],p}
`)
			fn := pkg.Func("probe")
			t.Log(RenderRegions(fn))
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
			if exact := DefinitelySame(returned.Results[0], returned.Results[1]); exact != test.exact {
				t.Fatalf("element zero exact=%v, want %v", exact, test.exact)
			}
		})
	}
}

// A field is write-once only when every write initializes a fresh object
// before anything else sees it, and no code, here or in another package, can
// overwrite it later. Each rejected field shows one way that can fail.
func TestWriteOnceFields(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "writeonce", `package writeonce
import "sync"
type conn struct{ mu sync.Mutex }
type server struct{ fixed, reset, late, escaped, loadedFirst, neverSet *conn }
type replaced struct{ whole *conn }
func newServer() *server {
	s := &server{fixed: &conn{}, reset: &conn{}, escaped: &conn{}}
	return s
}
func (s *server) use() (*conn, *conn) { return s.fixed, s.neverSet }
func (s *server) resetIt() { s.reset = &conn{} }
func later(publish func(*server)) {
	s := &server{}
	publish(s)
	s.late = &conn{}
}
func escape(s *server) **conn { return &s.escaped }
func overwrite(r *replaced) { *r = replaced{} }
func loadFirst() *conn {
	s := new(server)
	c := s.loadedFirst
	next := &conn{}
	s.loadedFirst = next
	return c
}

type Exported struct{ c *conn }
func (e *Exported) get() *conn { return e.c }
type Locked struct{ mu sync.Mutex; c *conn }
func newLocked() *Locked { return &Locked{c: &conn{}} }
type Public struct{ C *conn; mu sync.Mutex }
type inner struct{ c *conn }
type Outer struct{ in inner }
type slot struct{ c *conn }
func copies(dst, src []slot) { copy(dst, src) }
`)
	fields := NewWriteOnceFields(pkg.Pkg, ssaflow.DeclaredFunctions(pkg))
	field := func(typeName, name string) *types.Var {
		object, _, _ := types.LookupFieldOrMethod(pkg.Type(typeName).Type(), true, pkg.Pkg, name)
		return object.(*types.Var)
	}
	for _, test := range []struct {
		typeName, field string
		want            bool
	}{
		{"server", "fixed", true},
		{"server", "reset", false},
		{"server", "late", false},
		{"server", "escaped", false},
		{"replaced", "whole", false},
		{"server", "loadedFirst", false},
		{"server", "neverSet", false},
		{"Exported", "c", false},
		{"Locked", "c", true},
		{"Public", "C", false},
		{"inner", "c", false},
		{"slot", "c", false},
	} {
		if got := fields.Fixed(field(test.typeName, test.field)); got != test.want {
			t.Errorf("%s.%s: Fixed = %t, want %t", test.typeName, test.field, got, test.want)
		}
	}
}

func TestContentFromWritesDoesNotBuildGraph(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "writequery", `package writequery
type box struct { flag bool; next *box }
func opaque(*box)
func stable() bool { b := &box{flag: true}; return b.flag }
func escaped() bool { b := &box{flag: true}; opaque(b); return b.flag }
func nested() bool { b := &box{next: &box{flag: true}}; opaque(b); return b.next.flag }
func mixed(pick bool) bool { b := &box{flag: true}; if pick { b.flag = false }; return b.flag }
func pointerMixed(p,q *box,pick bool) *box { b := &box{next:p}; if pick { b.next=q }; return b.next }
`)
	for _, name := range []string{"stable", "escaped", "nested", "mixed", "pointerMixed"} {
		function := pkg.Func(name)
		loads := ssaflow.InstructionsOf[*ssa.UnOp](function)
		load := loads[len(loads)-1]
		proof := NewStorage(proofs.NewSearchBudget(proofs.QueryBudget)).ContentFromWrites(load.X, load)
		if proof.Proven() != (name == "stable") {
			t.Errorf("%s: unexpected proof %+v", name, proof)
		}
		regionGraphs.Lock()
		_, built := regionGraphs.entries[function]
		regionGraphs.Unlock()
		if built {
			t.Errorf("%s: writes-only query requested a points-to graph", name)
		}
	}
}

func TestNestedBackingCopyIdentity(t *testing.T) {
	for _, test := range []struct {
		name, body string
		want       bool
	}{
		{"root", `copied:=*source;observe(copied.value,source.value)`, true},
		{"nested", `var copied outer;copied.nested=*source;observe(copied.nested.value,source.value)`, true},
		{"sibling", `var copied outer;copied.nested=*source;observe(copied.nested.value,source.other)`, false},
		{"copyOfCopy", `var copied outer;copied.nested=*source;var next outer;next.nested=copied.nested;observe(next.nested.value,source.value)`, true},
		{"deeper", `var copied struct{padding int;nested outer};copied.nested.nested=*source;observe(copied.nested.nested.value,source.value)`, true},
		{"copyChanged", `var copied outer;copied.nested=*source;copied.nested.value=other;observe(copied.nested.value,source.value)`, false},
		{"sourceChanged", `var copied outer;copied.nested=*source;source.value=other;observe(copied.nested.value,source.value)`, false},
		{"snapshotBeforeChange", `old:=source.value;var copied outer;copied.nested=*source;source.value=other;observe(copied.nested.value,old)`, true},
		{"opaqueCopy", `var copied outer;copied.nested=*source;opaque(&copied);observe(copied.nested.value,source.value)`, false},
		{"opaqueSource", `var copied outer;copied.nested=*source;opaque(source);observe(copied.nested.value,source.value)`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "backingpaths", `package backingpaths
type inner struct{value,other *int}
type outer struct{padding int;nested inner}
func observe(a,b *int){}
func opaque(any)
func probe(source *inner,other *int){`+test.body+`}
`)
			function := pkg.Func("probe")
			var dump strings.Builder
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			call := heapObservation(t, function)
			graph := regionsOfFunction(function)
			if !graph.available {
				t.Fatalf("graph unavailable: %s", graph.buildReason)
			}
			arguments := call.Common().Args
			if got := graph.mustSame(arguments[0], arguments[1]); got != test.want {
				t.Fatalf("nested copy identity = %v, want %v", got, test.want)
			}
			if test.want && !graph.aliasProof(arguments[0], arguments[1]).Aliases {
				t.Fatal("exact copied identity did not support possible aliasing")
			}
		})
	}
}
