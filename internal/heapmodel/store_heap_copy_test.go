package heapmodel

import (
	"strconv"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestProjectHeapAggregateCopies(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "copies", `package copies
type File struct{ fd int }
type View struct { Out *File; Level int }
type Context struct { Value View }
func changeLevel(v View) View { v.Level = 1; return v }
func extract(c Context) View { return c.Value }
func replace(v View, f *File) View { v.Out = f; return v }
func snapshot(v View, f *File) View { old := v; v.Out = f; old.Level = 1; return old }
func opaque(v View, mutate func(*View)) View { mutate(&v); return v }
func priorSnapshot(v View, mutate func(*View)) View { old := v; mutate(&v); old.Level = 1; return old }
func zero() View { v := View{}; v.Level = 1; return v }
`)
	for _, test := range []struct {
		name   string
		want   string
		absent string
	}{
		{"changeLevel", "edge R0/field:0 -> P0/field:0 must", ""},
		{"extract", "edge R0/field:0 -> P0/field:0/field:0 must", ""},
		{"replace", "edge R0/field:0 -> P1 must", "edge R0/field:0 -> P0/field:0"},
		{"snapshot", "edge R0/field:0 -> P0/field:0 must", "edge R0/field:0 -> P1"},
		{"opaque", "", "edge R0/field:0 -> P0/field:0 must"},
		// The existing graph conservatively clobbers the shared external
		// origin even for this earlier value copy. Export preserves that gap.
		{"priorSnapshot", "", "edge R0/field:0 -> P0/field:0 must"},
		{"zero", "", "edge R0/field:0 -> P0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			summary, ok := ProjectHeap(pkg.Func(test.name))
			if !ok {
				t.Fatal("projection unavailable")
			}
			rendered := summary.String()
			if test.want != "" && !strings.Contains(rendered, test.want) {
				t.Errorf("projection lacks %q:\n%s", test.want, rendered)
			}
			if test.absent != "" && strings.Contains(rendered, test.absent) {
				t.Errorf("projection incorrectly contains %q:\n%s", test.absent, rendered)
			}
		})
	}
}

func TestProjectHeapAggregateCopyBounds(t *testing.T) {
	var wideFields strings.Builder
	// Give each field a distinct name without making the fixture depend on a
	// manually maintained list that could stop exercising the slot budget.
	for index := range SummarySlots + 1 {
		wideFields.WriteString("F" + strconv.Itoa(index) + " *File;")
	}
	pkg := ssaflowtest.BuildPackage(t, "copybounds", `package copybounds
type File struct{ fd int }
type Wide struct { Level int; `+wideFields.String()+` }
type Deep struct { Nested struct { Nested struct { Nested struct { Out *File } } }; Level int }
type Array struct { Values [2]*File; Level int }
func wide(v Wide) Wide { v.Level = 1; return v }
func deep(v Deep) Deep { v.Level = 1; return v }
func array(v Array) Array { v.Level = 1; return v }
`)
	for _, name := range []string{"wide", "deep", "array"} {
		t.Run(name, func(t *testing.T) {
			summary, ok := ProjectHeap(pkg.Func(name))
			if !ok || len(summary.Truncated) == 0 {
				t.Fatalf("bounded projection must mark omitted content: ok %t\n%s", ok, summary.String())
			}
			if len(summary.Edges) > SummarySlots+1 {
				t.Fatalf("projection exceeds result root plus slot budget:\n%s", summary.String())
			}
		})
	}
}

func TestProjectHeapConditionalAggregateCopies(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "conditionalcopies", `package conditionalcopies
type File struct{ fd int }
type View struct { Out *File; Level int }
func conditional(v View, change bool) View { if !change { return v }; v.Level = 1; return v }
func replacement(v View, f *File, change bool) View { if !change { return v }; v.Out = f; return v }
func observe(a, b *File) {}
func preserved(f *File, change bool) { v := conditional(View{Out: f}, change); observe(v.Out, f) }
func replaced(f, other *File, change bool) { v := replacement(View{Out: f}, other, change); observe(v.Out, f) }
`)
	for _, name := range []string{"conditional", "replacement"} {
		summary, ok := ProjectHeap(pkg.Func(name))
		if !ok {
			t.Fatalf("%s projection unavailable", name)
		}
		RegisterHeapSummary(pkg.Func(name), summary)
		preserved := strings.Contains(summary.String(), "edge R0/field:0 -> P0/field:0 must")
		if preserved != (name == "conditional") {
			t.Fatalf("%s preserved writer = %t:\n%s", name, preserved, summary.String())
		}
	}
	for name, want := range map[string]bool{"preserved": true, "replaced": false} {
		call := heapObservation(t, pkg.Func(name))
		graph := regionsOfFunction(call.Parent())
		if got := graph.mustSame(call.Common().Args[0], call.Common().Args[1]); got != want {
			t.Errorf("%s writer identity = %t, want %t:\n%s", name, got, want, RenderRegions(call.Parent()))
		}
	}
}
