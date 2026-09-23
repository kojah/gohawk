package ssaflow

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
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
			"edge R1 -> nil must",
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
