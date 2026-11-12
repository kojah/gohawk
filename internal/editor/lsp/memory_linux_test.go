//go:build linux

package lsp

import (
	"os"
	"syscall"
	"testing"
)

func TestParseProcStat(t *testing.T) {
	t.Parallel()
	// A command name with spaces and parentheses must not shift the fields.
	stat := []byte("4242 (go vet (tool)) S 1 777 777 0 -1 4194304 100 0 0 0 1 2 0 0 20 0 5 0 12345 1000000 256 18446744073709551615 0 0")
	group, resident, ok := parseProcStat(stat)
	if !ok || group != 777 || resident != 256 {
		t.Errorf("parseProcStat = %d, %d, %t; want 777, 256, true", group, resident, ok)
	}
	if _, _, ok := parseProcStat([]byte("garbage")); ok {
		t.Error("a malformed stat line was accepted")
	}
}

func TestProcessTreeMemoryMeasuresThisProcessGroup(t *testing.T) {
	t.Parallel()
	used, measured := processTreeMemory(syscall.Getpgrp())
	if !measured || used <= 0 {
		t.Errorf("own process group: %d bytes, measured=%t", used, measured)
	}
	if _, measured := processTreeMemory(os.Getpid() + 1<<22); measured {
		t.Error("a group with no processes was reported as measured")
	}
}
