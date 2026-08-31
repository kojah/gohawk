package blockingtest

import "testing"

//gohawk:example flagged
func waitForEvent(t *testing.T, events <-chan Event) Event {
	return <-events // want "blocking channel receive in test code requires cancellation-aware select"
}

//gohawk:example end

//gohawk:example ok
func waitForEventSafely(t *testing.T, events <-chan Event) Event {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-t.Context().Done():
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}

//gohawk:example end
