package heapmodel

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

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
