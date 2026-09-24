package heapmodel

import (
	"go/types"
	"strconv"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestContentIsNilAcrossPointerField(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "nilpathprobe", `package nilpathprobe
type inner struct{ value *int }
type outer struct{ next *inner }
type inline struct{ next inner }
func external() *inner
func observe(any) {}
func opaque() { o := &outer{next: external()}; observe(o) }
func knownNil() { o := &outer{next: &inner{}}; observe(o) }
func nilParent() { o := &outer{}; observe(o) }
func inlineNil() { o := &inline{}; observe(o) }
`)
	for _, test := range []struct {
		name string
		path []string
		want bool
	}{
		{"opaque", []string{"field:0", "field:0"}, false},
		{"knownNil", []string{"field:0", "field:0"}, true},
		{"nilParent", []string{"field:0"}, true},
		{"nilParent", []string{"field:0", "field:0"}, false},
		{"inlineNil", []string{"field:0", "field:0"}, true},
	} {
		t.Run(test.name+"/"+test.path[len(test.path)-1], func(t *testing.T) {
			call := heapObservation(t, pkg.Func(test.name))
			root := unwrapInterface(call.Common().Args[0])
			got := regionsOfFunction(call.Parent()).contentIsNil(root, test.path, call)
			if got != test.want {
				t.Errorf("contentIsNil(%v) = %t, want %t\n%s", test.path, got, test.want, RenderRegions(call.Parent()))
			}
		})
	}
}

// The nil query only supports these tests since its analyzer was removed.
// contentIsNil reports whether the slot at path beneath the root's object
// certainly holds nil when the instruction runs: one non-stale entry, and
// it is nil. An empty path asks about the root itself. Intermediate pointer
// fields must be followed to their pointee; a local outer object's zero
// slots say nothing about the pointed-to object's fields.
func (graph *regionGraph) contentIsNil(root ssa.Value, path []string, at ssa.Instruction) bool {
	defer graph.lock()()
	set, ok := graph.pointsToUnlocked(root)
	if !ok {
		return false
	}
	base, ok := singleSlot(set)
	if !ok {
		return false
	}
	if len(path) == 0 {
		return base.region.kind == regionNil
	}
	state := graph.stateAt(at)
	if state == nil {
		return false
	}
	target := base
	if len(path) > 1 {
		// A flattened path through a pointer field would inspect the local
		// outer object's unwritten slot, not the pointee's field. Follow only
		// exact pointer contents; an opaque pointee leaves the answer unknown.
		// https://github.com/timescale/timescaledb-tune/blob/c7a642bd4e16d48a51c060a43dc6dff864cb42d7/pkg/tstune/config_file_test.go#L25-L26
		currentType := root.Type()
		if pointer, ok := currentType.Underlying().(*types.Pointer); ok {
			currentType = pointer.Elem()
		}
		for _, step := range path[:len(path)-1] {
			target.path = joinSlotPath(target.path, step)
			fieldType, ok := selectedFieldType(currentType, step)
			if !ok {
				return false
			}
			switch typed := fieldType.Underlying().(type) {
			case *types.Pointer:
				held, exact := singleSlot(graph.content(state, target))
				if !exact || held.region.kind == regionNil {
					return false
				}
				target = held
				currentType = typed.Elem()
			case *types.Struct:
				currentType = fieldType
			default:
				return false
			}
		}
		target.path = joinSlotPath(target.path, path[len(path)-1])
	} else {
		target.path = joinSlotPath(target.path, ssaflow.JoinAccessPath(path))
	}
	held, ok := singleSlot(graph.content(state, target))
	return ok && held.region.kind == regionNil
}

func selectedFieldType(parent types.Type, step string) (types.Type, bool) {
	indexText, hasField := strings.CutPrefix(step, "field:")
	if !hasField {
		return nil, false
	}
	structure, ok := parent.Underlying().(*types.Struct)
	if !ok {
		return nil, false
	}
	index, err := strconv.Atoi(indexText)
	if err != nil || index < 0 || index >= structure.NumFields() {
		return nil, false
	}
	return structure.Field(index).Type(), true
}
