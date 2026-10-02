package heapmodel

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// The projection names only what a caller can name: a returned fresh object
// with the parameter's field stored in it, a parameter stored into a global,
// a field the function read, and nothing about the locals in between.
func TestProjectHeap(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "heapprobe", `package heapprobe
import "io"
type File struct{ fd int }
func (f *File) Close() error { return nil }
type Pair struct{ First, Second *File }
type Journal struct{ file *File; log io.Writer }
var registry []*Pair
var keep *File
func adopt(p *Pair, w io.Writer) (*Journal, error) {
	j := &Journal{file: p.First, log: w}
	registry = append(registry, p)
	_ = p.Second.Close()
	return j, nil
}
func keepFirst(p *Pair) { keep = p.First }
func swap(p *Pair, other *File, pick bool) { if pick { p.First = other } }
func opaque(p *Pair, f func(*Pair)) { f(p) }
func readOnly(p *Pair) bool { return p.First != nil }
`)
	for name, want := range map[string][]string{
		"adopt": {
			"edge G:heapprobe.registry -> fresh(call#",
			"edge G:heapprobe.registry/index:* -> P0 may",
			"edge R0 -> fresh(new#",
			"edge R0/field:0 -> P0/field:0 must",
			"edge R0/field:1 -> P1 must",
			"effect P0 escaped field every",
			"read P0/field:0",
			"read P0/field:1",
		},
		"keepFirst": {"edge G:heapprobe.keep -> P0/field:0 must", "effect P0/field:0 escaped global every"},
		"swap":      {"edge P0/field:0 -> P1 may"},
		"opaque":    {"effect P0 escaped call every", "truncated P0"},
		"readOnly":  {"read P0/field:0"},
	} {
		t.Run(name, func(t *testing.T) {
			summary, ok := ProjectHeap(pkg.Func(name))
			if !ok {
				t.Fatal("projection unavailable")
			}
			rendered := summary.String()
			for _, line := range want {
				if !strings.Contains(rendered, line) {
					t.Errorf("projection lacks %q:\n%s", line, rendered)
				}
			}
			if name == "readOnly" && (len(summary.Edges) != 0 || len(summary.Effects) != 0 || len(summary.Truncated) != 0) {
				t.Errorf("read-only function should project nothing but reads:\n%s", rendered)
			}
		})
	}
}

// An instantiation wrapper calls its generic origin; that call is not
// recursion, so the wrapper returns what the origin returns.
func TestProjectHeapInstantiationWrapper(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "genericprobe", `package genericprobe
type File struct{ fd int }
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
func use(f *File) *File { return must(f, nil) }
`)
	var wrapper *ssa.Function
	for _, block := range pkg.Func("use").Blocks {
		for _, instruction := range block.Instrs {
			if call, ok := instruction.(*ssa.Call); ok {
				if callee := call.Call.StaticCallee(); callee != nil && callee.Origin() != nil {
					wrapper = callee
				}
			}
		}
	}
	if wrapper == nil {
		t.Fatal("no instantiation of must")
	}
	summary, ok := ProjectHeap(wrapper)
	if !ok || !strings.Contains(summary.String(), "edge R0 -> P0 must") {
		t.Fatalf("wrapper summary = %v (ok %t), want the result to be its first parameter", summary.String(), ok)
	}
}

// A callee another analyzer is projecting still yields its summary: the
// caller projects it privately rather than treating the contention as a call
// cycle and getting none.
func TestOnDemandProjectionDuringAnotherProjection(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "contended", `package contended
type File struct{ fd int }
func keep(f *File) *File { return f }
`)
	function := pkg.Func("keep")
	heapSummaries.Lock()
	heapSummaries.entries[function] = heapEntry{state: heapEntryProjecting}
	heapSummaries.Unlock()
	t.Cleanup(func() {
		heapSummaries.Lock()
		delete(heapSummaries.entries, function)
		heapSummaries.Unlock()
	})
	summary, ok := projectHeapOnDemand(function)
	if !ok || !strings.Contains(summary.String(), "edge R0 -> P0 must") {
		t.Fatalf("contended projection = %v (ok %t), want the callee's own summary", summary.String(), ok)
	}
}

func TestProjectHeapNamedSlotDepthBoundary(t *testing.T) {
	atLimit := strings.Repeat("Next.", SummaryPaths-1) + "Value"
	beyond := strings.Repeat("Next.", SummaryPaths) + "Value"
	pkg := ssaflowtest.BuildPackage(t, "slotdepth", `package slotdepth
 type File struct{fd int}
 type Node struct{Next *Node;Value *File}
 var keep *File
 func limit(p *Node,v *File){keep=p.`+atLimit+`;p.`+atLimit+`=v}
 func beyond(p *Node,v *File){keep=p.`+beyond+`;p.`+beyond+`=v}
 func history(p *Node,v *File){p.`+atLimit+`=v;p.`+atLimit+`=nil}
 `)

	for _, test := range []struct {
		name   string
		depth  int
		edge   string
		escape bool
	}{
		{"limit", SummaryPaths, "must", true}, {"beyond", SummaryPaths + 1, "", false}, {"history", SummaryPaths, "may", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			summary, ok := ProjectHeap(pkg.Func(test.name))
			if !ok {
				t.Fatal("projection unavailable")
			}
			rendered := summary.String()
			path := strings.Repeat("field:0/", test.depth-1) + "field:1"
			prefix := "edge P0/" + path + " -> P1 "
			if test.edge == "" {
				if strings.Contains(rendered, prefix) {
					t.Fatalf("over-depth source slot published:\n%s", rendered)
				}
			} else if !strings.Contains(rendered, prefix+test.edge) {
				t.Fatalf("missing state/history edge:\n%s", rendered)
			}
			effect := "effect P0/" + path + " escaped global every"
			if strings.Contains(rendered, effect) != test.escape {
				t.Fatalf("escape depth boundary changed:\n%s", rendered)
			}
			// Forwarded value targets have their own existing naming policy. The
			// shared source/escape slot bound must not silently truncate those targets.
			if test.name != "history" && !strings.Contains(rendered, "edge G:slotdepth.keep -> P0/"+path+" must") {
				t.Fatalf("forwarded target changed:\n%s", rendered)
			}
		})
	}
}
