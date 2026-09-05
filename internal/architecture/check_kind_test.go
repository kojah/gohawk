package architecture

import (
	"testing"

	publicanalyzers "github.com/kojah/gohawk/analyzers"
)

// A policy check is valid Go that violates an intentionally selected
// engineering convention. It is the one kind a reader cannot act on without
// first agreeing with the tool, which puts it at odds with the claim the whole
// project rests on: that a gohawk diagnostic is actionable. Every one of them
// was withdrawn from the catalog, and this keeps the catalog that way.
//
// The kind itself deliberately survives in internal/catalog. Removing the word
// would not stop a convention being shipped; it would only mean the next one
// arrives labelled hazard, where nothing distinguishes it from a real one.
// Keeping the vocabulary is what lets a proposal be named policy, and declined
// on that basis, at the point where someone is asking for it.
//
// Delisting is the escape hatch, not an exception here. A withdrawn check may
// carry any kind, because the catalog never exposes it; this test reads the
// catalog, so it sees exactly what a user can select.
func TestNoPublicCheckIsAConvention(t *testing.T) {
	t.Parallel()
	for name, info := range publicanalyzers.AnalyzerMetadata() {
		for _, check := range info.Checks {
			if check.Kind == publicanalyzers.CheckKindPolicy {
				t.Errorf("analyzer %q publishes check %q as kind policy; a listed check has to claim a defect or a "+
					"hazard, so either restate what goes wrong at runtime or delist it", name, check.ID)
			}
		}
	}
}
