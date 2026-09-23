package lifecyclefacts

import (
	"go/types"
	"slices"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// The kept-contents claim is loose about whether and exact about where: a
// field stored, sent, captured, started, or returned is claimed at its own
// path, through a visible helper as well as directly; a basic-typed load, a
// read, or a call into a visible body that keeps nothing claims no path; and
// a non-aggregate parameter carries no claim at all. A field selected from a
// local copy of the pointee is still that field, and a parameter Retained
// outright, including one whose whole value is copied out, needs no claim
// of its own.
func TestLifecycleSummaryKeptContents(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type closer struct{ id int }

func (c *closer) Close() error { return nil }

type pair struct {
	first, second *closer
	name          string
}
type nested struct{ inner pair }
type sink interface{ Add(any) }

var (
	saved    *closer
	savedAll pair
	registry sink
	names    []string
	handles  chan *closer
)

func KeepFirst(p *pair)              { saved = p.first }
func KeepCopy(p *pair)               { c := *p; saved = c.second }
func KeepWhole(p *pair)              { savedAll = *p }
func Publish(p *pair)                { registry.Add(p.second) }
func Send(p *pair)                   { handles <- p.first }
func Capture(p *pair)                { defer func() { _ = p.second }() }
func Start(p *pair)                  { go use(p.first) }
func use(c *closer)                  { _ = c }
func ViaHelper(p *pair)              { KeepFirst(p) }
func ViaFieldHelper(p *pair)         { keep(p.second) }
func keep(c *closer)                 { saved = c }
func ByValue(p pair)                 { saved = p.first }
func First(p *pair) *closer          { return p.first }
func Nested(n *nested)               { saved = n.inner.second }
func KeepName(p *pair)               { names = append(names, p.name) }
func Inspect(p *pair) bool           { return p.first != nil }
func CloseSecond(p *pair) error      { return p.second.Close() }
func ViaReader(p *pair)              { read(p) }
func read(p *pair)                   { _ = p.first }
func NotAggregate(c *closer)         { saved = c }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string][]string{
		"KeepFirst":      {"field:0"},
		"KeepCopy":       {"field:1"},
		"KeepWhole":      nil,
		"Publish":        {"field:1"},
		"Send":           {"field:0"},
		"Capture":        nil,
		"Start":          {"field:0"},
		"ViaHelper":      {"field:0"},
		"ViaFieldHelper": {"field:1"},
		"ByValue":        {"field:0"},
		"First":          {"field:0"},
		"Nested":         {"field:0/field:1"},
		"KeepName":       nil,
		"Inspect":        nil,
		"CloseSecond":    nil,
		"ViaReader":      nil,
		"NotAggregate":   nil,
	} {
		fact := summarize(pass, pkg.Func(name))
		var got []string
		for _, kept := range fact.Kept {
			if kept.Parameter == 0 {
				got = append(got, kept.Path)
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s Kept paths = %q, want %q (retained %t)", name, got, want, fact.Retained.contains(0))
		}
	}
}
