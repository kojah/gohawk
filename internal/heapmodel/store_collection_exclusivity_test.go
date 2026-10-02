package heapmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
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
