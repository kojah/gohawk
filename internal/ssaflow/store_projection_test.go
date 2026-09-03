package ssaflow

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestUnmodifiedNonEmptyAccessPathAtBoundaries(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

type closer struct{}
func (*closer) Close() {}
type owner struct { body *closer }

func acquire() *owner { return nil }
func cleanup(*closer) {}
func mutateOwner(*owner) {}
func mutateSlot(**closer) {}

func accepted() {
	value := acquire()
	cleanup(value.body)
}
func escapedLater() {
	value := acquire()
	cleanup(value.body)
	mutateOwner(value)
}
func reassigned() {
	value := acquire()
	value.body = &closer{}
	cleanup(value.body)
}
func escapedRoot() {
	value := acquire()
	mutateOwner(value)
	cleanup(value.body)
}
func escapedAddress() {
	value := acquire()
	mutateSlot(&value.body)
	cleanup(value.body)
}
func selectedOwner(choose bool) {
	value := acquire()
	other := acquire()
	selected := other
	if choose { selected = value }
	cleanup(selected.body)
}
func sibling() {
	value := acquire()
	other := acquire()
	_ = value
	cleanup(other.body)
}
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "accepted", want: true},
		{name: "escapedLater", want: true},
		{name: "reassigned"},
		{name: "escapedRoot"},
		{name: "escapedAddress"},
		{name: "selectedOwner"},
		{name: "sibling"},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			var root ssa.Value
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					common := InstructionCall(instruction)
					if common != nil && CallName(common) == "acquire" && root == nil {
						root, _ = instruction.(ssa.Value)
					}
				}
			}
			cleanupCall := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
				return CallName(InstructionCall(instruction)) == "cleanup"
			})
			argument := InstructionCall(cleanupCall).Args[0]
			if got := UnmodifiedNonEmptyAccessPathAt(argument, root, cleanupCall); got != test.want {
				t.Fatalf("UnmodifiedNonEmptyAccessPathAt() = %t, want %t", got, test.want)
			}
		})
	}
}
