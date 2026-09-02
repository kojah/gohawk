// Package catalog provides the validated internal analyzer registry.
package catalog

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/check"

	"golang.org/x/tools/go/analysis"
)

// AnalyzerID identifies an analyzer in the catalog.
type AnalyzerID string

// GroupID identifies a related set of analyzers.
type GroupID string

// CheckKind describes the semantic claim made by a diagnostic rule.
type CheckKind string

const (
	// KindDefect identifies behavior that the available evidence establishes as broken or ineffective.
	KindDefect CheckKind = "defect"
	// KindHazard identifies risky behavior whose harm depends on a wider runtime contract.
	KindHazard CheckKind = "hazard"
	// KindPolicy identifies valid Go that violates an intentionally selected engineering convention.
	KindPolicy CheckKind = "policy"
)

// CheckTier records how much trust a check has earned and therefore whether
// it runs without being asked for. Tiers are ordered: core is enabled by
// default, extended must be selected, and experimental must be selected under
// an explicit experimental ceiling or by check ID.
type CheckTier string

const (
	// TierCore identifies checks whose precision is demonstrated on the
	// repository audit and guarded by the precision replay; they run by default.
	TierCore CheckTier = "core"
	// TierExtended identifies stable checks that encode a house rule a team
	// may reasonably decline; they run only when selected.
	TierExtended CheckTier = "extended"
	// TierExperimental identifies heuristic audits that may change or be
	// retired; they run only under an experimental ceiling or by check ID.
	TierExperimental CheckTier = "experimental"
)

// Tiers lists the tiers from most to least trusted.
func Tiers() []CheckTier {
	return []CheckTier{TierCore, TierExtended, TierExperimental}
}

// ParseTier returns the tier named by value.
func ParseTier(value string) (CheckTier, error) {
	for _, tier := range Tiers() {
		if string(tier) == value {
			return tier, nil
		}
	}
	return "", fmt.Errorf("unknown tier %q (expected core, extended, or experimental)", value)
}

// Within reports whether tier is at or above the trust of ceiling, so a
// ceiling of extended admits core and extended checks.
func (tier CheckTier) Within(ceiling CheckTier) bool {
	return tierRank(tier) <= tierRank(ceiling)
}

func tierRank(tier CheckTier) int {
	return slices.Index(Tiers(), tier)
}

// CheckInfo describes one independently configurable diagnostic rule.
type CheckInfo struct {
	ID   check.ID
	Doc  string
	Kind CheckKind
	Tier CheckTier
}

// EnabledAt reports whether the check runs when its analyzer is selected
// under ceiling.
func (info CheckInfo) EnabledAt(ceiling CheckTier) bool {
	return info.Tier.Within(ceiling)
}

// EnabledByDefault reports whether the check runs when its analyzer is selected.
func (info CheckInfo) EnabledByDefault() bool {
	return info.EnabledAt(TierCore)
}

// AnalyzerSpec is the complete declaration of one analyzer.
type AnalyzerSpec struct {
	Analyzer     *analysis.Analyzer
	Checks       []CheckInfo
	SuggestedFix bool
	// Group is the catalog group the analyzer was declared in.
	Group GroupID
}

// Tier is the most trusted tier among the analyzer's checks: the analyzer
// runs whenever a check at that tier is selected.
func (spec AnalyzerSpec) Tier() CheckTier {
	tier := TierExperimental
	for _, info := range spec.Checks {
		if info.Tier.Within(tier) {
			tier = info.Tier
		}
	}
	return tier
}

// EnabledAt reports whether the analyzer has a check that runs under ceiling.
func (spec AnalyzerSpec) EnabledAt(ceiling CheckTier) bool {
	return spec.Tier().Within(ceiling)
}

// EnabledByDefault reports whether the analyzer runs without explicit selection.
func (spec AnalyzerSpec) EnabledByDefault() bool {
	return spec.EnabledAt(TierCore)
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
	checkOwner     map[check.ID]AnalyzerID
}

// NewCatalog validates and constructs an analyzer catalog.
func NewCatalog(groups []GroupSpec, executionOrder []AnalyzerID) (*Catalog, error) {
	catalog := &Catalog{
		groups:         cloneGroups(groups),
		executionOrder: slices.Clone(executionOrder),
		byAnalyzer:     make(map[AnalyzerID]AnalyzerSpec),
		checkOwner:     make(map[check.ID]AnalyzerID),
	}
	seenGroups := make(map[GroupID]bool)
	seenPaths := make(map[string]bool)
	for groupIndex := range catalog.groups {
		if err := catalog.addGroup(groupIndex, seenGroups, seenPaths); err != nil {
			return nil, err
		}
	}
	if err := catalog.validateExecutionOrder(); err != nil {
		return nil, err
	}
	return catalog, nil
}

func (catalog *Catalog) addGroup(index int, seenGroups map[GroupID]bool, seenPaths map[string]bool) error {
	group := &catalog.groups[index]
	if group.ID == "" || strings.TrimSpace(group.Doc) == "" || strings.TrimSpace(group.DocPath) == "" {
		return fmt.Errorf("catalog group %d has incomplete identity or documentation", index)
	}
	if seenGroups[group.ID] {
		return fmt.Errorf("catalog group %q is declared more than once", group.ID)
	}
	if seenPaths[group.DocPath] {
		return fmt.Errorf("catalog documentation path %q is used more than once", group.DocPath)
	}
	seenGroups[group.ID], seenPaths[group.DocPath] = true, true
	for analyzerIndex := range group.Analyzers {
		if err := catalog.addAnalyzer(group.ID, &group.Analyzers[analyzerIndex]); err != nil {
			return err
		}
	}
	return nil
}

func (catalog *Catalog) addAnalyzer(groupID GroupID, spec *AnalyzerSpec) error {
	spec.Group = groupID
	if spec.Analyzer == nil || spec.Analyzer.Name == "" {
		return fmt.Errorf("catalog group %q contains an analyzer without an identity", groupID)
	}
	id := AnalyzerID(spec.Analyzer.Name)
	if _, exists := catalog.byAnalyzer[id]; exists {
		return fmt.Errorf("analyzer %q is declared more than once", id)
	}
	if len(spec.Checks) == 0 {
		return fmt.Errorf("analyzer %q declares no checks", id)
	}
	for checkIndex := range spec.Checks {
		if err := catalog.addCheck(id, &spec.Checks[checkIndex]); err != nil {
			return err
		}
	}
	catalog.byAnalyzer[id] = cloneAnalyzerSpec(*spec)
	return nil
}

func (catalog *Catalog) addCheck(analyzerID AnalyzerID, info *CheckInfo) error {
	if info.ID == "" || !strings.HasPrefix(string(info.ID), string(analyzerID)+"/") {
		return fmt.Errorf("analyzer %q has invalid check identity %q", analyzerID, info.ID)
	}
	if strings.TrimSpace(info.Doc) == "" {
		return fmt.Errorf("check %q has no description", info.ID)
	}
	if !validCheckKind(info.Kind) {
		return fmt.Errorf("check %q has invalid kind %q", info.ID, info.Kind)
	}
	if tierRank(info.Tier) < 0 {
		return fmt.Errorf("check %q has invalid tier %q", info.ID, info.Tier)
	}
	if owner, exists := catalog.checkOwner[info.ID]; exists {
		return fmt.Errorf("check %q belongs to both %q and %q", info.ID, owner, analyzerID)
	}
	catalog.checkOwner[info.ID] = analyzerID
	return nil
}

func validCheckKind(kind CheckKind) bool {
	return kind == KindDefect || kind == KindHazard || kind == KindPolicy
}

func (catalog *Catalog) validateExecutionOrder() error {
	seenOrder := make(map[AnalyzerID]bool)
	for _, id := range catalog.executionOrder {
		if _, exists := catalog.byAnalyzer[id]; !exists {
			return fmt.Errorf("execution order contains unknown analyzer %q", id)
		}
		if seenOrder[id] {
			return fmt.Errorf("execution order repeats analyzer %q", id)
		}
		seenOrder[id] = true
	}
	if len(seenOrder) != len(catalog.byAnalyzer) {
		return fmt.Errorf("execution order contains %d analyzers; catalog declares %d", len(seenOrder), len(catalog.byAnalyzer))
	}
	return nil
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
func (catalog *Catalog) CheckOwner(check check.ID) (AnalyzerID, bool) {
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
	copy(checks, spec.Checks)
	spec.Checks = checks
	return spec
}
