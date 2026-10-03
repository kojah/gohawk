package lockorder

import (
	"encoding/json"
	"testing"
)

func TestLockModeWireLabels(t *testing.T) {
	encoded, err := json.Marshal([]LockMode{ModeRead, ModeExclusive})
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	if err := json.Unmarshal(encoded, &labels); err != nil {
		t.Fatal(err)
	}
	if len(labels) != 2 || labels[0] != "RLock" || labels[1] != "Lock" {
		t.Fatalf("graph acquisition labels changed: %s", encoded)
	}
	var restored []LockMode
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored) != 2 || restored[0] != ModeRead || restored[1] != ModeExclusive {
		t.Fatalf("restored modes: %+v", restored)
	}
	if err := restored[0].UnmarshalText([]byte("Unlock")); err == nil || restored[0] != ModeRead {
		t.Fatalf("invalid acquisition replaced read mode: %v, %v", restored[0], err)
	}
}
