//go:build linux

package lsp

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
)

// processTreeMemory sums the resident memory of every process in the group
// that go vet leads. The second result is false when nothing could be read,
// so an unknown measurement is never mistaken for zero.
func processTreeMemory(group int) (int64, bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, false
	}
	var pages int64
	measured := false
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			continue
		}
		processGroup, resident, ok := parseProcStat(stat)
		if ok && processGroup == group {
			pages += resident
			measured = true
		}
	}
	return pages * int64(os.Getpagesize()), measured
}

// parseProcStat reads the process group and resident page count from a
// /proc/<pid>/stat line. The command name may contain spaces and parentheses,
// so fields are counted from the last closing parenthesis.
func parseProcStat(stat []byte) (group int, residentPages int64, ok bool) {
	end := bytes.LastIndexByte(stat, ')')
	if end < 0 {
		return 0, 0, false
	}
	fields := bytes.Fields(stat[end+1:])
	const groupField, residentField = 2, 21
	if len(fields) <= residentField {
		return 0, 0, false
	}
	group, groupErr := strconv.Atoi(string(fields[groupField]))
	residentPages, residentErr := strconv.ParseInt(string(fields[residentField]), 10, 64)
	return group, residentPages, groupErr == nil && residentErr == nil
}
