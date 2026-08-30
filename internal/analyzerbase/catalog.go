package analyzerbase

import (
	"fmt"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// AnalyzerID identifies an analyzer in the catalog.
type AnalyzerID string

// GroupID identifies a related set of analyzers.
type GroupID string

// CheckInfo describes one independently configurable diagnostic rule.
type CheckInfo struct {
	ID    Check
	Doc   string
	OptIn bool
}

// EnabledByDefault reports whether the check runs when its analyzer is selected.
func (info CheckInfo) EnabledByDefault() bool {
	return !info.OptIn
}

// AnalyzerSpec is the complete declaration of one analyzer.
type AnalyzerSpec struct {
	Analyzer     *analysis.Analyzer
	OptIn        bool
	Checks       []CheckInfo
	SuggestedFix bool
}

// EnabledByDefault reports whether the analyzer runs without explicit selection.
func (spec AnalyzerSpec) EnabledByDefault() bool {
	return !spec.OptIn
}

// GroupSpec declares a catalog group and its analyzers.
type GroupSpec struct {
	ID        GroupID
	Doc       string
	DocPath   string
	Analyzers []AnalyzerSpec
}

// Catalog is a validated analyzer registry in presentation and execution order.
type Catalog struct {
	groups         []GroupSpec
	executionOrder []AnalyzerID
	byAnalyzer     map[AnalyzerID]AnalyzerSpec
	checkOwner     map[Check]AnalyzerID
}

// NewCatalog validates and constructs an analyzer catalog.
func NewCatalog(groups []GroupSpec, executionOrder []AnalyzerID) (*Catalog, error) {
	catalog := &Catalog{
		groups:         cloneGroups(groups),
		executionOrder: slices.Clone(executionOrder),
		byAnalyzer:     make(map[AnalyzerID]AnalyzerSpec),
		checkOwner:     make(map[Check]AnalyzerID),
	}
	seenGroups := make(map[GroupID]bool)
	seenPaths := make(map[string]bool)
	for groupIndex := range catalog.groups {
		group := &catalog.groups[groupIndex]
		if group.ID == "" || strings.TrimSpace(group.Doc) == "" || strings.TrimSpace(group.DocPath) == "" {
			return nil, fmt.Errorf("catalog group %d has incomplete identity or documentation", groupIndex)
		}
		if seenGroups[group.ID] {
			return nil, fmt.Errorf("catalog group %q is declared more than once", group.ID)
		}
		if seenPaths[group.DocPath] {
			return nil, fmt.Errorf("catalog documentation path %q is used more than once", group.DocPath)
		}
		seenGroups[group.ID], seenPaths[group.DocPath] = true, true
		for analyzerIndex := range group.Analyzers {
			spec := &group.Analyzers[analyzerIndex]
			if spec.Analyzer == nil || spec.Analyzer.Name == "" {
				return nil, fmt.Errorf("catalog group %q contains an analyzer without an identity", group.ID)
			}
			id := AnalyzerID(spec.Analyzer.Name)
			if _, exists := catalog.byAnalyzer[id]; exists {
				return nil, fmt.Errorf("analyzer %q is declared more than once", id)
			}
			if len(spec.Checks) == 0 {
				return nil, fmt.Errorf("analyzer %q declares no checks", id)
			}
			for checkIndex := range spec.Checks {
				check := &spec.Checks[checkIndex]
				if check.ID == "" || !strings.HasPrefix(string(check.ID), string(id)+"/") {
					return nil, fmt.Errorf("analyzer %q has invalid check identity %q", id, check.ID)
				}
				if strings.TrimSpace(check.Doc) == "" {
					return nil, fmt.Errorf("check %q has no description", check.ID)
				}
				if owner, exists := catalog.checkOwner[check.ID]; exists {
					return nil, fmt.Errorf("check %q belongs to both %q and %q", check.ID, owner, id)
				}
				catalog.checkOwner[check.ID] = id
			}
			catalog.byAnalyzer[id] = cloneAnalyzerSpec(*spec)
		}
	}
	seenOrder := make(map[AnalyzerID]bool)
	for _, id := range catalog.executionOrder {
		if _, exists := catalog.byAnalyzer[id]; !exists {
			return nil, fmt.Errorf("execution order contains unknown analyzer %q", id)
		}
		if seenOrder[id] {
			return nil, fmt.Errorf("execution order repeats analyzer %q", id)
		}
		seenOrder[id] = true
	}
	if len(seenOrder) != len(catalog.byAnalyzer) {
		return nil, fmt.Errorf("execution order contains %d analyzers; catalog declares %d", len(seenOrder), len(catalog.byAnalyzer))
	}
	return catalog, nil
}

// Groups returns catalog groups in stable presentation order.
func (catalog *Catalog) Groups() []GroupSpec {
	return cloneGroups(catalog.groups)
}

// Analyzers returns analyzer specs in stable execution order.
func (catalog *Catalog) Analyzers() []AnalyzerSpec {
	result := make([]AnalyzerSpec, 0, len(catalog.executionOrder))
	for _, id := range catalog.executionOrder {
		result = append(result, cloneAnalyzerSpec(catalog.byAnalyzer[id]))
	}
	return result
}

// Analyzer returns the declaration for id.
func (catalog *Catalog) Analyzer(id AnalyzerID) (AnalyzerSpec, bool) {
	spec, ok := catalog.byAnalyzer[id]
	return cloneAnalyzerSpec(spec), ok
}

// CheckOwner returns the analyzer that owns check.
func (catalog *Catalog) CheckOwner(check Check) (AnalyzerID, bool) {
	owner, ok := catalog.checkOwner[check]
	return owner, ok
}

func cloneGroups(groups []GroupSpec) []GroupSpec {
	cloned := make([]GroupSpec, len(groups))
	for index, group := range groups {
		cloned[index] = GroupSpec{ID: group.ID, Doc: group.Doc, DocPath: group.DocPath, Analyzers: make([]AnalyzerSpec, len(group.Analyzers))}
		for analyzerIndex, analyzer := range group.Analyzers {
			cloned[index].Analyzers[analyzerIndex] = cloneAnalyzerSpec(analyzer)
		}
	}
	return cloned
}

func cloneAnalyzerSpec(spec AnalyzerSpec) AnalyzerSpec {
	checks := make([]CheckInfo, len(spec.Checks))
	for index, check := range spec.Checks {
		checks[index] = check
	}
	spec.Checks = checks
	return spec
}
