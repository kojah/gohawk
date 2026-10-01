package heapmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// A copied address retains its destination's ownership; the allocation holding
// the copy is not that destination. Dynamic aggregate copies currently lose
// this evidence and must not be interpreted as either caller-owned or local.
func TestCopiedDestinationExclusivity(t *testing.T) {
	for _, test := range []struct {
		name, body   string
		known, local bool
	}{
		{"callerField", `owner.a = value`, true, false},
		{"copiedCallerAddress", `entry := struct{dest **int}{&owner.a}; *entry.dest = value`, true, false},
		{"copiedLocalAddress", `local := &box{}; entry := struct{dest **int}{&local.a}; *entry.dest = value`, true, true},
		{"replacedAddress", `local := &box{}; entry := struct{dest **int}{&owner.a}; ` +
			`entry.dest = &local.a; *entry.dest = value`, true, true},
		{"nilAddress", `entry := struct{dest **int}{}; *entry.dest = value`, false, false},
		{"opaqueAddress", `entry := struct{dest **int}{opaque()}; *entry.dest = value`, false, false},
		{"mixedAddress", `local := &box{}; dest := &owner.a; if pick { dest = &local.a }; *dest = value`, false, false},
		{"rangedCallerAddresses", `entries := []struct{dest **int}{{&owner.a},{&owner.b}}; ` +
			`for _, entry := range entries { *entry.dest = value }`, false, false},
		{"rangedLocalAddresses", `local := &box{}; entries := []struct{dest **int}{{&local.a},{&local.b}}; ` +
			`for _, entry := range entries { *entry.dest = value }`, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "destination", `package destination
 type box struct { a, b *int }
 func opaque() **int
 func probe(owner *box, value *int, pick bool) { `+test.body+` }
`)
			function := pkg.Func("probe")
			found := false
			for _, store := range ssaflow.InstructionsOf[*ssa.Store](function) {
				if store.Val != function.Params[1] {
					continue
				}
				found = true
				object, known := ExclusiveAt(store.Addr, store)
				if known != test.known || known && (object.Local != test.local || !object.Local && object.Parameter != 0) {
					t.Fatalf("destination ownership = %+v, known=%v; want known=%v, local=%v", object, known, test.known, test.local)
				}
			}
			if !found {
				t.Fatal("missing destination store")
			}
		})
	}
}
