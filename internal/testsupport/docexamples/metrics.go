package docexamples

import (
	"time"

	"golang.org/x/tools/go/packages"
)

// Metrics records the work performed by CollectAll. Counts include
// imported packages; durations are populated only when requested.
type Metrics struct {
	Targets              int
	Regions              int
	LoadedPackages       int
	AnalyzerRoots        int
	AnalyzerTimings      []AnalyzerTiming
	RegionScan           time.Duration
	PackageLoad          time.Duration
	AnalyzerRun          time.Duration
	DiagnosticExtraction time.Duration
}

// AnalyzerTiming is the wall time spent in one analyzer's checker run, including
// its prerequisite passes over the selected fixture roots.
type AnalyzerTiming struct {
	Name     string
	Duration time.Duration
	Roots    int
}

func (metrics *Metrics) reset(targets int) {
	if metrics != nil {
		*metrics = Metrics{Targets: targets}
	}
}

func (metrics *Metrics) start() time.Time {
	if metrics == nil {
		return time.Time{}
	}
	return time.Now()
}

func (metrics *Metrics) addRegions(count int) {
	if metrics != nil {
		metrics.Regions += count
	}
}

func (metrics *Metrics) finishRegionScan(started time.Time) {
	if metrics != nil {
		metrics.RegionScan = time.Since(started)
	}
}

func (metrics *Metrics) finishPackageLoad(started time.Time, roots []*packages.Package) {
	if metrics != nil {
		metrics.PackageLoad = time.Since(started)
		metrics.LoadedPackages = countLoadedPackages(roots)
	}
}

func (metrics *Metrics) finishAnalyzerRun(started time.Time, name string, roots int) {
	if metrics != nil {
		duration := time.Since(started)
		metrics.AnalyzerRun += duration
		metrics.AnalyzerRoots += roots
		metrics.AnalyzerTimings = append(metrics.AnalyzerTimings, AnalyzerTiming{Name: name, Duration: duration, Roots: roots})
	}
}

func (metrics *Metrics) finishDiagnostics(started time.Time) {
	if metrics != nil {
		metrics.DiagnosticExtraction += time.Since(started)
	}
}

func countLoadedPackages(roots []*packages.Package) int {
	seen := make(map[*packages.Package]bool)
	stack := append([]*packages.Package(nil), roots...)
	for len(stack) > 0 {
		last := len(stack) - 1
		pkg := stack[last]
		stack = stack[:last]
		if pkg == nil || seen[pkg] {
			continue
		}
		seen[pkg] = true
		for _, imported := range pkg.Imports {
			stack = append(stack, imported)
		}
	}
	return len(seen)
}
