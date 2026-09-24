package ssaflow

import "github.com/kojah/gohawk/internal/heapmodel"

// These aliases keep the existing SSA-facing API source-compatible while
// consumers move to heapmodel. The summary contract itself lives there.
type (
	HeapRootKind        = heapmodel.HeapRootKind
	HeapRoot            = heapmodel.HeapRoot
	HeapSlot            = heapmodel.HeapSlot
	HeapTargetKind      = heapmodel.HeapTargetKind
	HeapTarget          = heapmodel.HeapTarget
	HeapEdge            = heapmodel.HeapEdge
	HeapEscape          = heapmodel.HeapEscape
	HeapEffect          = heapmodel.HeapEffect
	HeapSummary         = heapmodel.HeapSummary
	HeapHold            = heapmodel.HeapHold
	HeapRequirementKind = heapmodel.HeapRequirementKind
	HeapRequirement     = heapmodel.HeapRequirement
)

const (
	HeapParameter      = heapmodel.HeapParameter
	HeapResult         = heapmodel.HeapResult
	HeapGlobal         = heapmodel.HeapGlobal
	HeapFreeVar        = heapmodel.HeapFreeVar
	HeapTargetSlot     = heapmodel.HeapTargetSlot
	HeapTargetFresh    = heapmodel.HeapTargetFresh
	HeapTargetNil      = heapmodel.HeapTargetNil
	HeapTargetUnknown  = heapmodel.HeapTargetUnknown
	HeapEscapedGlobal  = heapmodel.HeapEscapedGlobal
	HeapEscapedField   = heapmodel.HeapEscapedField
	HeapEscapedCall    = heapmodel.HeapEscapedCall
	HeapEscapedAsync   = heapmodel.HeapEscapedAsync
	HeapEscapedSend    = heapmodel.HeapEscapedSend
	HeapRequiresMethod = heapmodel.HeapRequiresMethod
	HeapRequiresNonNil = heapmodel.HeapRequiresNonNil
)

// heapPathDepth bounds the paths a summary names beneath a root, and
// heapSlotLimit the slots per root before the root is truncated instead.
const (
	heapPathDepth = heapmodel.SummaryPaths
	heapSlotLimit = heapmodel.SummarySlots
)

func sortedSlots(set map[HeapSlot]bool) []HeapSlot {
	return heapmodel.SortedSlots(set)
}

func heapSlotLess(left, right HeapSlot) bool {
	return heapmodel.SlotLess(left, right)
}

func heapEdgeLess(left, right HeapEdge) bool {
	return heapmodel.EdgeLess(left, right)
}

func heapEffectLess(left, right HeapEffect) bool {
	return heapmodel.EffectLess(left, right)
}
