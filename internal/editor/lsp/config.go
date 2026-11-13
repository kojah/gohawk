package lsp

import (
	"math"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Editor settings choose when checks run and how much a run may use. None of
// them changes what a check reports: analyzers have no options.

const (
	runOnPause = "onPause"
	runOnSave  = "onSave"
	runManual  = "manual"

	scopePackage          = "package"
	scopeThenImporters    = "packageThenImporters"
	scopeWorkspaceOnSave  = "workspaceOnSave"
	defaultDebounce       = 750 * time.Millisecond
	defaultTimeout        = 60 * time.Second
	maxDebounce           = time.Minute
	maxTimeoutSecs        = 3600
	defaultMemoryLimit    = "2GiB"
	defaultMaxParallel    = 2
	defaultImporterLimit  = 50
	stepPathClientCommand = "gohawk.stepPath"
)

type settings struct {
	Run          string `json:"run"`
	DebounceMS   *int   `json:"debounce"`
	Scope        string `json:"scope"`
	Experimental bool   `json:"experimental"`
	MemoryLimit  string `json:"memoryLimit"`
	MaxParallel  int    `json:"maxParallel"`
	TimeoutSecs  int    `json:"timeout"`
	// ImporterLimit is a pointer because zero is a meaningful value: no
	// limit.
	ImporterLimit *int `json:"importerLimit"`
}

// initializationOptions carries the settings and the commands the client
// implements itself, so the server offers only actions the editor can run.
type initializationOptions struct {
	Settings       settings `json:"settings"`
	ClientCommands []string `json:"clientCommands"`
}

type configurationParams struct {
	Settings struct {
		Gohawk *settings `json:"gohawk"`
	} `json:"settings"`
}

func defaultSettings() settings {
	return settings{Run: runOnPause, Scope: scopeThenImporters, MemoryLimit: defaultMemoryLimit, MaxParallel: defaultMaxParallel}
}

// normalized fills unset or unknown values with defaults so the rest of the
// server never branches on an invalid setting.
func (s settings) normalized() settings {
	switch s.Run {
	case runOnPause, runOnSave, runManual:
	default:
		s.Run = runOnPause
	}
	switch s.Scope {
	case scopePackage, scopeThenImporters, scopeWorkspaceOnSave:
	default:
		s.Scope = scopeThenImporters
	}
	if _, ok := parseByteSize(s.MemoryLimit); !ok {
		s.MemoryLimit = defaultMemoryLimit
	}
	if s.MaxParallel < 1 {
		s.MaxParallel = defaultMaxParallel
	}
	// A value far beyond the machine is a mistake, and the go command takes
	// it literally.
	s.MaxParallel = min(s.MaxParallel, max(runtime.NumCPU(), defaultMaxParallel))
	return s
}

func (s settings) debounce() time.Duration {
	if s.DebounceMS == nil || *s.DebounceMS < 0 {
		return defaultDebounce
	}
	return min(time.Duration(*s.DebounceMS)*time.Millisecond, maxDebounce)
}

// importerLimit is the most importers a background run follows, or zero for
// no limit.
func (s settings) importerLimit() int {
	if s.ImporterLimit == nil || *s.ImporterLimit < 0 {
		return defaultImporterLimit
	}
	return *s.ImporterLimit
}

func (s settings) timeout() time.Duration {
	if s.TimeoutSecs < 1 {
		return defaultTimeout
	}
	return time.Duration(min(s.TimeoutSecs, maxTimeoutSecs)) * time.Second
}

func (s settings) memoryBytes() int64 {
	size, _ := parseByteSize(s.MemoryLimit)
	return size
}

// parseByteSize reads a size in the form GOMEMLIMIT accepts: a number with an
// optional B, KiB, MiB, GiB, or TiB suffix.
func parseByteSize(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	units := []struct {
		suffix string
		factor int64
	}{{"TiB", 1 << 40}, {"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}, {"B", 1}, {"", 1}}
	for _, unit := range units {
		digits, found := strings.CutSuffix(value, unit.suffix)
		if !found {
			continue
		}
		number, err := strconv.ParseInt(digits, 10, 64)
		if err != nil || number <= 0 || number > math.MaxInt64/unit.factor {
			return 0, false
		}
		return number * unit.factor, true
	}
	return 0, false
}
