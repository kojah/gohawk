package analyzers

import (
	"slices"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/catalog"
	"golang.org/x/tools/go/analysis"
)

func expectedAnalyzerNames() []string {
	return []string{
		"apishape",
		"contextpolicy",
		"testlifecycle",
		"globalstate",
		"wirepolicy",
		"testpolicy",
		"goroutineownership",
		"producerlifecycle",
		"errorownership",
		"errorclassification",
		"inlineerror",
		"channelcapacity",
		"channelownership",
		"channelsafety",
		"processownership",
		"closedomain",
		"taintpolicy",
		"lockorder",
		"resourcelifetime",
		"deferinloop",
		"exitpolicy",
		"determinism",
		"concurrentcapture",
		"evalorder",
		"oncepolicy",
		"syncmapatomicity",
		"cancellationownership",
		"borrowedstorage",
	}
}

func TestCatalogDeclaration(t *testing.T) {
	if _, err := newCatalog(); err != nil {
		t.Fatalf("invalid analyzer catalog: %v", err)
	}
}

func TestUnknownDiagnosticReturnsError(t *testing.T) {
	analyzer := withSuppressions(&analysis.Analyzer{
		Name: "example",
		Run: func(pass *analysis.Pass) (any, error) {
			pass.Report(analysis.Diagnostic{Category: "example/unknown"})
			return nil, nil
		},
	}, []catalog.CheckInfo{{ID: "example/known", Kind: catalog.KindDefect}})

	_, err := analyzer.Run(&analysis.Pass{Report: func(analysis.Diagnostic) {}})
	if err == nil || !strings.Contains(err.Error(), `analyzer "example" reported unknown check "example/unknown"`) {
		t.Fatalf("Run() error = %v, want unknown-check error", err)
	}
}

func TestAnalyzerGroups(t *testing.T) {
	want := []struct {
		name      string
		doc       string
		docPath   string
		analyzers []string
	}{
		{
			name:      "contracts",
			doc:       "API and data contracts",
			docPath:   "api-and-data-contracts",
			analyzers: []string{"apishape", "contextpolicy", "closedomain", "wirepolicy"},
		},
		{
			name:    "ownership",
			doc:     "ownership and lifecycle",
			docPath: "ownership-and-lifecycle",
			analyzers: []string{
				"borrowedstorage",
				"cancellationownership",
				"channelcapacity",
				"channelownership",
				"channelsafety",
				"deferinloop",
				"exitpolicy",
				"goroutineownership",
				"producerlifecycle",
				"processownership",
				"resourcelifetime",
			},
		},
		{
			name:    "reliability",
			doc:     "reliability and safety",
			docPath: "reliability-and-safety",
			analyzers: []string{
				"concurrentcapture",
				"determinism",
				"errorownership",
				"errorclassification",
				"inlineerror",
				"evalorder",
				"globalstate",
				"lockorder",
				"oncepolicy",
				"syncmapatomicity",
				"taintpolicy",
			},
		},
		{name: "testing", doc: "testing", docPath: "testing", analyzers: []string{"testlifecycle", "testpolicy"}},
	}
	groups := AnalyzerGroups()
	if len(groups) != len(want) {
		t.Fatalf("group count = %d, want %d", len(groups), len(want))
	}
	seen := make(map[string]bool)
	for index, group := range groups {
		names := make([]string, 0, len(group.Analyzers))
		for _, analyzer := range group.Analyzers {
			if seen[analyzer.Name] {
				t.Fatalf("analyzer %q appears in more than one group", analyzer.Name)
			}
			seen[analyzer.Name] = true
			names = append(names, analyzer.Name)
		}
		if group.Name != want[index].name || group.Doc != want[index].doc || group.DocPath != want[index].docPath ||
			!slices.Equal(names, want[index].analyzers) {
			t.Fatalf(
				"group %d = %q (%q, %q) %v, want %q (%q, %q) %v",
				index,
				group.Name,
				group.Doc,
				group.DocPath,
				names,
				want[index].name,
				want[index].doc,
				want[index].docPath,
				want[index].analyzers,
			)
		}
	}
	if len(seen) != len(expectedAnalyzerNames()) {
		t.Fatalf("grouped analyzer count = %d, want %d", len(seen), len(expectedAnalyzerNames()))
	}
}

func TestAnalyzerMetadata(t *testing.T) {
	metadata := AnalyzerMetadata()
	if len(metadata) != len(expectedAnalyzerNames()) {
		t.Fatalf("metadata count = %d, want %d", len(metadata), len(expectedAnalyzerNames()))
	}
	optIn := map[string]bool{
		"apishape": true, "channelcapacity": true, "closedomain": true, "errorownership": true,
		"determinism": true, "globalstate": true, "taintpolicy": true,
		"testlifecycle": true, "testpolicy": true, "wirepolicy": true, "borrowedstorage": true,
	}
	seenChecks := make(map[AnalyzerCheck]string)
	optInChecks := map[AnalyzerCheck]bool{
		"goroutineownership/detached": true,
	}
	kinds := map[AnalyzerCheck]CheckKind{
		"apishape/parameter-count":           CheckKindPolicy,
		"apishape/mixed-receivers":           CheckKindPolicy,
		"apishape/adjacent-same-type":        CheckKindPolicy,
		"apishape/adjacent-optional-scalars": CheckKindPolicy,
		"contextpolicy/context-first":        CheckKindPolicy,
		"contextpolicy/context-storage":      CheckKindPolicy,
		"contextpolicy/nil-context":          CheckKindDefect,
		"closedomain/closed-string-domain":   CheckKindPolicy,
		"wirepolicy/keyed-literal":           CheckKindPolicy,
		"wirepolicy/serialization-tag":       CheckKindPolicy,
		"cancellationownership/release":      CheckKindDefect,
		"borrowedstorage/overlapping-owner":  CheckKindHazard,
		"channelcapacity/rationale":          CheckKindPolicy,
		"channelownership/caller-close":      CheckKindPolicy,
		"channelsafety/send-after-close":     CheckKindDefect,
		"deferinloop/cleanup-lifetime":       CheckKindHazard,
		"exitpolicy/skipped-defer":           CheckKindDefect,
		"goroutineownership/unjoined":        CheckKindHazard,
		"goroutineownership/detached":        CheckKindHazard,
		"producerlifecycle/abandoned-send":   CheckKindHazard,
		"processownership/missing-wait":      CheckKindDefect,
		"resourcelifetime/missing-release":   CheckKindDefect,
		"concurrentcapture/shared-capture":   CheckKindHazard,
		"determinism/map-output-order":       CheckKindHazard,
		"errorownership/log-and-return":      CheckKindPolicy,
		"errorclassification/text-match":     CheckKindHazard,
		"inlineerror/mismatched-condition":   CheckKindDefect,
		"evalorder/operand-mutation":         CheckKindHazard,
		"globalstate/mutable-package-state":  CheckKindPolicy,
		"lockorder/missing-release":          CheckKindDefect,
		"lockorder/recursive-acquire":        CheckKindDefect,
		"lockorder/contradictory-order":      CheckKindHazard,
		"oncepolicy/discarded-wrapper":       CheckKindDefect,
		"syncmapatomicity/non-atomic-claim":  CheckKindHazard,
		"taintpolicy/untrusted-sink":         CheckKindHazard,
		"testlifecycle/context-root":         CheckKindHazard,
		"testpolicy/helper-marker":           CheckKindPolicy,
	}
	for _, name := range expectedAnalyzerNames() {
		info, ok := metadata[name]
		if !ok {
			t.Errorf("metadata missing analyzer %q", name)
		} else if info.OptIn != optIn[name] {
			t.Errorf("analyzer %q opt-in = %t, want %t", name, info.OptIn, optIn[name])
		}
		if len(info.Checks) == 0 {
			t.Errorf("analyzer %q has no checks", name)
		}
		for _, check := range info.Checks {
			if check.ID == "" || !strings.HasPrefix(string(check.ID), name+"/") {
				t.Errorf("analyzer %q has invalid check identity %q", name, check.ID)
			}
			if strings.TrimSpace(check.Doc) == "" {
				t.Errorf("check %q has no description", check.ID)
			}
			if check.OptIn != optInChecks[check.ID] {
				t.Errorf("check %q opt-in = %t, want %t", check.ID, check.OptIn, optInChecks[check.ID])
			}
			if check.Kind != kinds[check.ID] {
				t.Errorf("check %q kind = %q, want %q", check.ID, check.Kind, kinds[check.ID])
			}
			if owner, exists := seenChecks[check.ID]; exists {
				t.Errorf("check %q belongs to both %q and %q", check.ID, owner, name)
			}
			seenChecks[check.ID] = name
		}
	}
	if len(seenChecks) != len(kinds) {
		t.Fatalf("classified check count = %d, want %d", len(seenChecks), len(kinds))
	}
}

func TestDefaultAnalyzers(t *testing.T) {
	want := []string{
		"contextpolicy", "goroutineownership", "producerlifecycle",
		"errorclassification", "inlineerror", "channelownership", "channelsafety",
		"processownership", "lockorder", "resourcelifetime",
		"deferinloop", "exitpolicy", "concurrentcapture",
		"evalorder", "oncepolicy", "syncmapatomicity", "cancellationownership",
	}
	var names []string
	for _, analyzer := range DefaultAnalyzers() {
		names = append(names, analyzer.Name)
	}
	if !slices.Equal(names, want) {
		t.Fatalf("default analyzers = %v, want %v", names, want)
	}
}

func TestAnalyzerRegistry(t *testing.T) {
	registered := Analyzers()
	names := make([]string, 0, len(registered))
	for _, analyzer := range registered {
		names = append(names, analyzer.Name)
	}
	want := expectedAnalyzerNames()
	if !slices.Equal(names, want) {
		t.Fatalf("registered analyzers = %v, want %v", names, want)
	}
}
