package analyzers

import "github.com/kojah/gohawk/internal/catalog"

// AnalyzerCheck identifies one independently configurable diagnostic rule.
type AnalyzerCheck string

// CheckKind describes the semantic claim made by a diagnostic rule.
type CheckKind string

const (
	// CheckKindDefect identifies behavior that the available evidence establishes as broken or ineffective.
	CheckKindDefect CheckKind = "defect"
	// CheckKindHazard identifies risky behavior whose harm depends on a wider runtime contract.
	CheckKindHazard CheckKind = "hazard"
	// CheckKindPolicy identifies valid Go that violates an intentionally selected engineering convention.
	CheckKindPolicy CheckKind = "policy"
)

// CheckTier records how much trust a check has earned and whether it runs
// without being asked for: core runs by default, extended must be selected,
// and experimental must be selected under an explicit experimental ceiling or
// by check ID.
type CheckTier string

const (
	// CheckTierCore identifies checks whose precision is demonstrated on the repository audit; they run by default.
	CheckTierCore CheckTier = "core"
	// CheckTierExtended identifies stable checks that encode a house rule a team may reasonably decline.
	CheckTierExtended CheckTier = "extended"
	// CheckTierExperimental identifies heuristic audits that may change or be retired.
	CheckTierExperimental CheckTier = "experimental"
)

// ParseCheckTier returns the tier named by value.
func ParseCheckTier(value string) (CheckTier, error) {
	tier, err := catalog.ParseTier(value)
	return CheckTier(tier), err
}

// Within reports whether tier is at or above the trust of ceiling.
func (tier CheckTier) Within(ceiling CheckTier) bool {
	return catalog.CheckTier(tier).Within(catalog.CheckTier(ceiling))
}

// AnalyzerCheckInfo describes a specific diagnostic rule.
type AnalyzerCheckInfo struct {
	ID   AnalyzerCheck
	Doc  string
	Kind CheckKind
	Tier CheckTier
}

// EnabledAt reports whether the check runs when its analyzer is selected under ceiling.
func (info AnalyzerCheckInfo) EnabledAt(ceiling CheckTier) bool {
	return info.Tier.Within(ceiling)
}

// EnabledByDefault reports whether the check runs when its analyzer is selected.
func (info AnalyzerCheckInfo) EnabledByDefault() bool {
	return info.EnabledAt(CheckTierCore)
}
