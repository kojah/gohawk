package lockorder

import (
	"math/rand/v2"
	"sync"
)

// Read-lock writes into results the function builds itself. A reader that
// copies guarded data into a slice of its own and then reorders or fills in
// that slice writes only its own memory; other readers each build their own.
// The slice starts nil, so the element addresses must not be widened to
// "anything" just because indexing the nil start would fault.
//
// Known gap: the write walk does not follow a phi, so a slice that is nil on
// one path and the owner's own storage on another is not reported when it is
// written at a constant index. Before selections through nil stopped being
// widened to unknown, that case was reported only because the unknown
// element aliased everything, including the owner.

type resultRecord struct{ peer int }

type resultStore struct {
	mu        sync.RWMutex
	providers map[string][]resultRecord
	order     []string
}

// Accepted: a shuffle of a slice appended from nil under the read lock.
func (s *resultStore) shuffled(key string) []int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []int
	for _, record := range s.providers[key] {
		out = append(out, record.peer)
	}
	for i := range out {
		j := rand.IntN(i + 1)
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Accepted: reversing a locally built order in place.
func (s *resultStore) reversed() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var order []string
	order = append(order, s.order...)
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order
}

type resultTask struct {
	slot int
	args []int
}

// Accepted: filling in fields of tasks the function allocated.
func (s *resultStore) grouped(key string) []*resultTask {
	tasks := make([]*resultTask, 0)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.providers[key] {
		j := 0
		for j = 0; j < len(tasks); j++ {
			if tasks[j].slot == record.peer {
				tasks[j].args = append(tasks[j].args, record.peer)
				break
			}
		}
		if j == len(tasks) {
			tasks = append(tasks, &resultTask{slot: record.peer})
		}
	}
	return tasks
}

type resultHop struct{ pending bool }

type resultView struct{ hops []resultHop }

// Accepted: updating the last element of a nested local result through a
// pointer to it.
func (s *resultStore) views(key string) []resultView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []resultView
	for range s.providers[key] {
		out = append(out, resultView{hops: []resultHop{{pending: true}}})
		hop := &out[len(out)-1].hops[0]
		hop.pending = false
	}
	return out
}
