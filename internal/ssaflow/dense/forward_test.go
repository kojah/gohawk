package dense

import (
	"maps"
	"testing"
)

func union(left, right uint8) uint8 { return left | right }

func TestForwardRevisitsJoinAfterLatePredecessor(t *testing.T) {
	// The short branch reaches join before the longer branch. Its extra fact
	// must propagate through join and exit even though both were already seen.
	transfer := func(point string, fact uint8) []State[string, uint8] {
		switch point {
		case "entry":
			return []State[string, uint8]{{"join", 1}, {"long", 2}}
		case "long":
			return []State[string, uint8]{{"later", fact}}
		case "later":
			return []State[string, uint8]{{"join", fact}}
		case "join":
			return []State[string, uint8]{{"exit", fact}}
		default:
			return nil
		}
	}
	result := Forward([]State[string, uint8]{{"entry", 0}}, union, transfer, 20)
	want := map[string]uint8{"entry": 0, "long": 2, "later": 2, "join": 3, "exit": 3}
	if !result.Complete || !maps.Equal(result.In, want) {
		t.Fatalf("result = %+v, want complete with %v", result, want)
	}
}

func TestForwardConvergesOnCycle(t *testing.T) {
	transfer := func(point string, fact uint8) []State[string, uint8] {
		if point == "loop" {
			return []State[string, uint8]{{"loop", fact | (fact << 1 & 7)}, {"exit", fact}}
		}
		return nil
	}
	result := Forward([]State[string, uint8]{{"loop", 1}}, union, transfer, 10)
	if !result.Complete || result.In["loop"] != 7 || result.In["exit"] != 7 {
		t.Fatalf("loop failed to converge: %+v", result)
	}
}

func TestForwardCoalescesPendingFacts(t *testing.T) {
	var calls int
	transfer := func(point string, fact uint8) []State[string, uint8] {
		calls++
		if point != "join" || fact != 3 {
			t.Errorf("transfer(%q, %d), want join with both incoming facts", point, fact)
		}
		return nil
	}
	initial := []State[string, uint8]{{"join", 1}, {"join", 2}, {"join", 1}}
	result := Forward(initial, union, transfer, 1)
	if !result.Complete || result.Steps != 1 || calls != 1 {
		t.Fatalf("pending facts were not coalesced: %+v, calls=%d", result, calls)
	}
}

func TestForwardDistinguishesZeroFromUnreachable(t *testing.T) {
	transfer := func(point string, fact uint8) []State[string, uint8] {
		if point == "entry" {
			return []State[string, uint8]{{"exit", fact}}
		}
		return nil
	}
	result := Forward([]State[string, uint8]{{"entry", 0}}, union, transfer, 2)
	if fact, reached := result.In["exit"]; !result.Complete || !reached || fact != 0 {
		t.Fatalf("reachable zero fact was lost: %+v", result)
	}
	if _, reached := result.In["unreachable"]; reached {
		t.Fatal("unreachable point acquired a fact")
	}
}

func TestForwardBudgetStopsNonConvergingTransfer(t *testing.T) {
	transfer := func(point string, fact int) []State[string, int] {
		return []State[string, int]{{point, fact + 1}}
	}
	for _, limit := range []int{-1, 0, 1, 5} {
		result := Forward([]State[string, int]{{"loop", 0}}, func(a, b int) int { return max(a, b) }, transfer, limit)
		if result.Complete || result.Steps != max(limit, 0) {
			t.Fatalf("limit %d: result = %+v", limit, result)
		}
	}
}

func TestForwardEmptyInput(t *testing.T) {
	result := Forward[string, uint8](nil, union, func(string, uint8) []State[string, uint8] {
		t.Fatal("transfer called without a reachable entry")
		return nil
	}, 0)
	if !result.Complete || result.Steps != 0 || len(result.In) != 0 {
		t.Fatalf("empty input = %+v", result)
	}
}
