package heapmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCollectionDestinationExclusivity(t *testing.T) {
	for _, test := range []struct {
		name, body   string
		known, local bool
	}{
		{"freshMap", `m:=make(map[int]int);m[0]=7`, true, true},
		{"publishedMap", `m:=make(map[int]int);globalMap=m;m[0]=7`, false, false},
		{"opaqueMap", `m:=opaqueMap();m[0]=7`, false, false},
		{"callerMap", `suppliedMap[0]=7`, true, false},
		{"mixedMap", `m:=suppliedMap;if flag{m=make(map[int]int)};m[0]=7`, false, false},
		{"freshSlice", `s:=make([]int,n);s[n/2]=7`, true, true},
		{"publishedSlice", `s:=make([]int,n);globalSlice=s;s[0]=7`, false, false},
		{"opaqueSlice", `s:=opaqueSlice();s[0]=7`, false, false},
		{"callerSlice", `suppliedSlice[n/2]=7`, true, false},
		{"mixedSlice", `s:=suppliedSlice;if flag{s=make([]int,n)};s[0]=7`, false, false},
		{"escapedCallerElement", `retain(&suppliedSlice[0]);suppliedSlice[1]=7`, false, false},
		{"escapedElement", `s:=make([]int,n);retain(&s[0]);s[1]=7`, false, false},
		{"asyncSlice", `s:=make([]int,n);go consume(s);s[0]=7`, false, false},
		{"loopMap", `for i:=0;i<n;i++{m:=make(map[int]int);m[0]=7}`, true, true},
		{"publishedLoopMap", `for i:=0;i<n;i++{m:=make(map[int]int);globalMap=m;m[0]=7}`, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "collectionexclusive", `package collectionexclusive
 var globalMap map[int]int
 var globalSlice []int
 func opaqueMap()map[int]int
 func opaqueSlice()[]int
 func retain(*int)
 func consume([]int)
 func probe(n int,suppliedMap map[int]int,suppliedSlice []int,flag bool){`+test.body+`}
 `)
			fn := pkg.Func("probe")
			found := false
			for instruction := range ssaflow.InstructionsWithin(fn, nil) {
				var destination ssa.Value
				switch mutation := instruction.(type) {
				case *ssa.MapUpdate:
					destination = mutation.Map
				case *ssa.Store:
					destination = mutation.Addr
				}
				if destination == nil {
					continue
				}
				found = true
				object, known := ExclusiveAt(destination, instruction)
				if known != test.known || known && object.Local != test.local {
					t.Fatalf("known=%v object=%+v; want known=%v local=%v", known, object, test.known, test.local)
				}
			}
			if !found {
				t.Fatal("missing compiled mutation")
			}
		})
	}
}

func TestFreshChannelExclusivity(t *testing.T) {
	for _, test := range []struct {
		name, body   string
		known, local bool
	}{
		{"private", `ch:=make(chan int);println(len(ch))`, true, true},
		{"published", `ch:=make(chan int);global=ch;println(len(ch))`, false, false},
		{"async", `ch:=make(chan int);go consume(ch);println(len(ch))`, false, false},
		{"borrowed", `println(len(supplied))`, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "chanexclusive", `package chanexclusive
 var global chan int
 func consume(chan int)
 func probe(supplied chan int){`+test.body+`}
 `)
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](pkg.Func("probe")) {
				if ssaflow.CallName(call.Common()) != "len" {
					continue
				}
				object, known := ExclusiveAt(call.Common().Args[0], call)
				if known != test.known || known && object.Local != test.local {
					t.Fatalf("known=%v object=%+v; want known=%v local=%v", known, object, test.known, test.local)
				}
				return
			}
			t.Fatal("missing compiled observation")
		})
	}
}

// A copied address retains its destination's ownership; the allocation holding
// the copy is not that destination. A dynamic aggregate copy must retain
// every possible destination rather than treating its local holder as the owner.
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
			`for _, entry := range entries { *entry.dest = value }`, true, false},
		{"rangedMixedAddresses", `local := &box{}; entries := []struct{dest **int}{{&owner.a},{&local.b}}; ` +
			`for _, entry := range entries { *entry.dest = value }`, false, false},
		{"rangedNilAddress", `entries := []struct{dest **int}{{&owner.a},{nil}}; ` +
			`for _, entry := range entries { *entry.dest = value }`, false, false},
		{"rangedOpaqueAddress", `entries := []struct{dest **int}{{&owner.a},{opaque()}}; ` +
			`for _, entry := range entries { *entry.dest = value }`, false, false},
		{"rangedDynamicReplacement", `local := &box{}; entries := []struct{dest **int}{{&owner.a},{&owner.b}}; ` +
			`entries[position].dest = &local.a; for _, entry := range entries { *entry.dest = value }`, false, false},
		{"rangedFixedReplacement", `local := &box{}; entries := []struct{dest **int}{{&owner.a},{&owner.b}}; ` +
			`entries[0].dest = &local.a; for _, entry := range entries { *entry.dest = value }`, false, false},
		{"rangedUnknownWindow", `for _, entry := range supplied { *entry.dest = value }`, false, false},
		{"rangedOversizeWindow", `var entries [17]struct{dest **int}; for index := range entries { entries[index].dest = &owner.a }; ` +
			`for _, entry := range entries { *entry.dest = value }`, false, false},
		{"rangedOffsetWindow", `entries := []struct{dest **int}{{nil},{&owner.a},{&owner.b}}; ` +
			`for _, entry := range entries[1:] { *entry.dest = value }`, true, false},
		{"rangedReplacedCopy", `local := &box{}; entries := []struct{dest **int}{{&owner.a},{&owner.b}}; ` +
			`for _, entry := range entries { entry.dest = &local.a; *entry.dest = value }`, true, true},
		{"rangedNestedAddresses", `entries := []struct{nested struct{dest **int}}{{struct{dest **int}{&owner.a}},{struct{dest **int}{&owner.b}}}; ` +
			`for _, entry := range entries { *entry.nested.dest = value }`, true, false},
		{"rangedLocalAddresses", `local := &box{}; entries := []struct{dest **int}{{&local.a},{&local.b}}; ` +
			`for _, entry := range entries { *entry.dest = value }`, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "destination", `package destination
 type box struct { a, b *int }
 func opaque() **int
 func probe(owner *box, value *int, pick bool, position int, supplied []struct{dest **int}) { `+test.body+` }
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
