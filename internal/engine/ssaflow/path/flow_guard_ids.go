package path

import (
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	// Guard IDs are local to one walk and intern the authoritative encoded bytes,
	// not the structural guard list. Legacy delimiter collisions must still name
	// the same visited state. Rendering retains every original budget charge.
)

type guardKeyID int

type guardIDs struct {
	keys   guardKeys
	known  map[string]guardKeyID
	last   string
	lastID guardKeyID
}

func (ids *guardIDs) within(guards PathGuards, budget *proofs.SearchBudget) guardKeyID {
	key := ids.keys.keyWithin(guards, budget)
	if budget.Exhausted() || key == "" {
		return 0
	}
	if key == ids.last {
		return ids.lastID
	}
	if id, ok := ids.known[key]; ok {
		ids.last, ids.lastID = key, id
		return id
	}
	if ids.known == nil {
		ids.known = make(map[string]guardKeyID)
	}
	id := guardKeyID(len(ids.known) + 1)
	ids.known[key] = id
	ids.last, ids.lastID = key, id
	return id
}
