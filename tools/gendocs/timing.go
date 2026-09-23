package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/kojah/gohawk/internal/docexamples"
)

// docsTimings separates fixture analysis from page work so a slow generated
// documentation check points to the expensive phase without a CPU profile.
type docsTimings struct {
	started   time.Time
	examples  bool
	manifest  time.Duration
	render    time.Duration
	write     time.Duration
	analyzers int
	pages     int
	collector docexamples.Metrics
}

func (timings *docsTimings) exampleMetrics() *docexamples.Metrics {
	if timings == nil || !timings.examples {
		return nil
	}
	return &timings.collector
}

func (timings *docsTimings) String() string {
	if timings == nil {
		return ""
	}
	mode := "fast"
	if timings.examples {
		mode = "examples"
	}
	var output strings.Builder
	// A strings.Builder never returns a write error.
	_, _ = fmt.Fprintf(&output, "gendocs timing: total=%.3fs mode=%s analyzers=%d files=%d\n",
		time.Since(timings.started).Seconds(), mode, timings.analyzers, timings.pages)
	_, _ = fmt.Fprintf(&output, "  manifest: %.3fs\n", timings.manifest.Seconds())
	if timings.examples {
		metrics := timings.collector
		_, _ = fmt.Fprintf(&output, "  fixture scan: %.3fs targets=%d regions=%d\n",
			metrics.RegionScan.Seconds(), metrics.Targets, metrics.Regions)
		_, _ = fmt.Fprintf(&output, "  package load: %.3fs packages=%d\n",
			metrics.PackageLoad.Seconds(), metrics.LoadedPackages)
		_, _ = fmt.Fprintf(&output, "  analyzer run: %.3fs roots=%d\n",
			metrics.AnalyzerRun.Seconds(), metrics.AnalyzerRoots)
		_, _ = fmt.Fprintf(&output, "  diagnostics: %.3fs\n", metrics.DiagnosticExtraction.Seconds())
	}
	_, _ = fmt.Fprintf(&output, "  page render: %.3fs\n  file sync: %.3fs\n", timings.render.Seconds(), timings.write.Seconds())
	return output.String()
}
