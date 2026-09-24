package codecbench

import (
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/tools/codecbench/heappb"
	"google.golang.org/protobuf/proto"
)

// These adapters are benchmark code, not a validated publication boundary.
// Production would need checked narrowing and domain-version validation.
func toSlot(slot heapmodel.HeapSlot) *heappb.HeapSlot {
	return &heappb.HeapSlot{Root: &heappb.HeapRoot{Kind: uint32(slot.Root.Kind), Index: int32(slot.Root.Index), Package: slot.Root.Package, Name: slot.Root.Name}, Path: slot.Path}
}

func fromSlot(slot *heappb.HeapSlot) heapmodel.HeapSlot {
	root := slot.GetRoot()
	return heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapRootKind(root.GetKind()), Index: int(root.GetIndex()), Package: root.GetPackage(), Name: root.GetName()}, Path: slot.GetPath()}
}

func encodeHeapProto(value heapmodel.HeapSummary) ([]byte, error) {
	wire := &heappb.HeapSummary{Version: int32(value.Version)}
	wire.Edges = slices.Grow(wire.Edges, len(value.Edges))
	wire.Effects = slices.Grow(wire.Effects, len(value.Effects))
	wire.Holds = slices.Grow(wire.Holds, len(value.Holds))
	wire.Reads = slices.Grow(wire.Reads, len(value.Reads))
	wire.Requires = slices.Grow(wire.Requires, len(value.Requires))
	wire.Truncated = slices.Grow(wire.Truncated, len(value.Truncated))
	for _, edge := range value.Edges {
		wire.Edges = append(wire.Edges, &heappb.HeapEdge{From: toSlot(edge.From), To: &heappb.HeapTarget{Kind: uint32(edge.To.Kind), Slot: toSlot(edge.To.Slot), Origin: edge.To.Origin, Object: int32(edge.To.Object)}, Must: edge.Must})
	}
	for _, effect := range value.Effects {
		wire.Effects = append(wire.Effects, &heappb.HeapEffect{Slot: toSlot(effect.Slot), Escape: uint32(effect.Escape), Release: effect.Release, Every: effect.Every})
	}
	for _, hold := range value.Holds {
		wire.Holds = append(wire.Holds, &heappb.HeapHold{Result: int32(hold.Result), Parameter: int32(hold.Parameter), Must: hold.Must})
	}
	for _, slot := range value.Reads {
		wire.Reads = append(wire.Reads, toSlot(slot))
	}
	for _, requirement := range value.Requires {
		wire.Requires = append(wire.Requires, &heappb.HeapRequirement{Slot: toSlot(requirement.Slot), Kind: uint32(requirement.Kind), Method: requirement.Method})
	}
	for _, slot := range value.Truncated {
		wire.Truncated = append(wire.Truncated, toSlot(slot))
	}
	return (proto.MarshalOptions{Deterministic: true}).Marshal(wire)
}

func decodeHeapProto(data []byte) (heapmodel.HeapSummary, error) {
	var wire heappb.HeapSummary
	if err := (proto.UnmarshalOptions{RecursionLimit: 64}).Unmarshal(data, &wire); err != nil {
		return heapmodel.HeapSummary{}, err
	}
	value := heapmodel.HeapSummary{Version: int(wire.Version)}
	value.Edges = slices.Grow(value.Edges, len(wire.Edges))
	value.Effects = slices.Grow(value.Effects, len(wire.Effects))
	value.Holds = slices.Grow(value.Holds, len(wire.Holds))
	value.Reads = slices.Grow(value.Reads, len(wire.Reads))
	value.Requires = slices.Grow(value.Requires, len(wire.Requires))
	value.Truncated = slices.Grow(value.Truncated, len(wire.Truncated))
	for _, edge := range wire.Edges {
		value.Edges = append(value.Edges, heapmodel.HeapEdge{From: fromSlot(edge.From), To: heapmodel.HeapTarget{Kind: heapmodel.HeapTargetKind(edge.To.GetKind()), Slot: fromSlot(edge.To.GetSlot()), Origin: edge.To.GetOrigin(), Object: int(edge.To.GetObject())}, Must: edge.Must})
	}
	for _, effect := range wire.Effects {
		value.Effects = append(value.Effects, heapmodel.HeapEffect{Slot: fromSlot(effect.Slot), Escape: heapmodel.HeapEscape(effect.Escape), Release: effect.Release, Every: effect.Every})
	}
	for _, hold := range wire.Holds {
		value.Holds = append(value.Holds, heapmodel.HeapHold{Result: int(hold.Result), Parameter: int(hold.Parameter), Must: hold.Must})
	}
	for _, slot := range wire.Reads {
		value.Reads = append(value.Reads, fromSlot(slot))
	}
	for _, requirement := range wire.Requires {
		value.Requires = append(value.Requires, heapmodel.HeapRequirement{Slot: fromSlot(requirement.Slot), Kind: heapmodel.HeapRequirementKind(requirement.Kind), Method: requirement.Method})
	}
	for _, slot := range wire.Truncated {
		value.Truncated = append(value.Truncated, fromSlot(slot))
	}
	return value, nil
}
