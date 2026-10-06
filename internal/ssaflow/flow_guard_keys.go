package ssaflow

// Guard keys retain their existing byte grammar and equality, including any
// delimiter collisions. A walk reuses only the last bounded guard list's key;
// it still charges every guard before publishing that key. Stable is absent
// from both the encoded key and this representation.
type guardKeyPart struct {
	identity string
	value    bool
}

type guardKeyParts struct {
	entries [GuardLimit]guardKeyPart
	count   int
}

type guardKeys struct {
	parts guardKeyParts
	key   string
	ready bool
}

func (keys *guardKeys) keyWithin(guards PathGuards, budget *SearchBudget) string {
	if keys == nil || len(guards) > GuardLimit {
		return guards.KeyWithin(budget)
	}
	if len(guards) == 0 {
		return ""
	}
	parts := guardKeyParts{count: len(guards)}
	for index, guard := range guards {
		if !budget.Spend() {
			return ""
		}
		parts.entries[index] = guardKeyPart{identity: guard.Identity, value: guard.Value}
	}
	if keys.ready && keys.parts == parts {
		return keys.key
	}
	// Charges above preserve the original per-entry cutoff; rendering has
	// no independent charges and delegates to the authoritative byte format.
	key := guards.KeyWithin(nil)
	keys.parts, keys.key, keys.ready = parts, key, true
	return key
}
