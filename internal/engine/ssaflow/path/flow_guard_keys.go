package path

import (
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	// Guard keys retain their existing byte grammar and equality, including any
	// delimiter collisions. A walk reuses a bounded window of rendered guard lists;
	// it still charges every guard before publishing that key. Stable is absent
	// from both the encoded key and this representation.
)

type guardKeyPart struct {
	identity string
	value    bool
}

type guardKeyParts struct {
	entries [GuardLimit]guardKeyPart
	count   int
}

// A four-entry window covers small alternating branch frontiers without retaining
// every list visited. A one-list walk never allocates the additional window.
const guardKeyMemoLimit = 4

type guardKeyEntry struct {
	parts guardKeyParts
	key   string
	ready bool
}

type guardKeys struct {
	guardKeyEntry
	history *[guardKeyMemoLimit - 1]guardKeyEntry
	next    int
}

func (keys *guardKeys) keyWithin(guards PathGuards, budget *proofs.SearchBudget) string {
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
	if keys.history != nil {
		for index := range keys.history {
			entry := &keys.history[index]
			if entry.ready && entry.parts.count == parts.count && entry.parts == parts {
				keys.guardKeyEntry, keys.history[index] = *entry, keys.guardKeyEntry
				return keys.key
			}
		}
	}
	// Charges above preserve the original per-entry cutoff; rendering has
	// no independent charges and delegates to the authoritative byte format.
	key := guards.KeyWithin(nil)
	keys.remember(guardKeyEntry{parts: parts, key: key, ready: true})
	return key
}

func (keys *guardKeys) remember(entry guardKeyEntry) {
	if keys.ready {
		if keys.history == nil {
			keys.history = new([guardKeyMemoLimit - 1]guardKeyEntry)
		}
		keys.history[keys.next] = keys.guardKeyEntry
		keys.next = (keys.next + 1) % len(keys.history)
	}
	keys.guardKeyEntry = entry
}
