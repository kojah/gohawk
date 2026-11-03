package lifecyclefacts

import (
	"bytes"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/factcodec"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// Exported facts are action inputs for every importer in build systems that
// key work on fact bytes, so the same function must project to the same
// bytes on every run. Go randomizes map iteration per range loop, so two
// independent builds of one package in one process expose any projection
// whose numbering or order follows a map walk.
// https://github.com/kojah/gohawk/issues/69
func TestHeapProjectionIsByteDeterministic(t *testing.T) {
	for _, path := range []string{"bufio", "strconv"} {
		first := encodedHeapProjections(t, path)
		for range 3 {
			for name, encoded := range encodedHeapProjections(t, path) {
				if !bytes.Equal(first[name], encoded) {
					t.Errorf("%s: heap projection bytes differ between identical builds", name)
				}
			}
		}
	}
}

// encodedHeapProjections builds path's SSA from scratch and encodes the heap
// projection of each of its functions, keyed by function name.
func encodedHeapProjections(t *testing.T, path string) map[string][]byte {
	t.Helper()
	loaded, err := packages.Load(&packages.Config{Mode: packages.LoadSyntax}, path)
	if err != nil || packages.PrintErrors(loaded) != 0 || len(loaded) != 1 {
		t.Fatalf("load %s: %v", path, err)
	}
	program, built := ssautil.Packages(loaded, ssa.InstantiateGenerics)
	program.Build()
	encoded := map[string][]byte{}
	for function := range ssautil.AllFunctions(program) {
		if function.Pkg != built[0] || function.Blocks == nil {
			continue
		}
		data, err := factcodec.Encode(projectHeap(function))
		if err != nil {
			t.Fatalf("encode %s: %v", function, err)
		}
		encoded[function.String()] = data
	}
	if len(encoded) == 0 {
		t.Fatalf("%s: no functions projected", path)
	}
	return encoded
}
