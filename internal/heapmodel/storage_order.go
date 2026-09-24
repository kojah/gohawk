package heapmodel

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// StoreMayFollow reports whether the store can run after the observation on
// the same cell. A local declared inside a loop is a fresh allocation each
// iteration, so a store reached only by re-executing the allocation writes a
// different cell and does not reassign the observed one. cb-spider retries a
// request in a loop and defers the body close inside each iteration:
// https://github.com/cloud-barista/cb-spider/blob/5aa6bd8a8a09003dc168ac78f6ea987617de9d31/cloud-control-manager/cloud-driver/drivers/ibm/resources/PriceInfoHandler.go#L153-L169
func StoreMayFollow(address ssa.Value, observation ssa.Instruction, store *ssa.Store) bool {
	allocation, ok := address.(*ssa.Alloc)
	if !ok {
		return ssaflow.InstructionMayFollow(observation, store)
	}
	if observation.Block() == store.Block() {
		return ssaflow.InstructionIndex(observation) <= ssaflow.InstructionIndex(store)
	}
	seen := map[*ssa.BasicBlock]bool{allocation.Block(): true}
	queue := append([]*ssa.BasicBlock(nil), observation.Block().Succs...)
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if seen[block] {
			// The allocation's block is never entered: reaching a store through
			// it means the allocation ran again and the store writes a new cell.
			continue
		}
		if block == store.Block() {
			return true
		}
		seen[block] = true
		queue = append(queue, block.Succs...)
	}
	return false
}
