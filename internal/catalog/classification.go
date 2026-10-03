package catalog

import (
	"errors"
	"fmt"
	"slices"

	"github.com/kojah/gohawk/internal/enumtext"
)

// CheckKind describes the semantic claim made by a diagnostic rule.
type CheckKind uint8

const (
	_ CheckKind = iota
	// KindDefect identifies behavior that the available evidence establishes as broken or ineffective.
	KindDefect
	// KindHazard identifies risky behavior whose harm depends on a wider runtime contract.
	KindHazard
	// KindPolicy identifies valid Go that violates an intentionally selected engineering convention.
	KindPolicy
)

// CheckTier records how much trust a check has earned and therefore whether
// it runs without being asked for. Tiers are ordered: core is enabled by
// default, and experimental must be selected under an explicit experimental
// ceiling or by check ID.
type CheckTier uint8

const (
	_ CheckTier = iota
	// TierCore identifies checks whose precision is demonstrated on the
	// repository audit and guarded by the precision replay; they run by default.
	TierCore
	// TierExperimental identifies heuristic audits that may change or be
	// retired; they run only under an experimental ceiling or by check ID.
	TierExperimental
)

// Tiers lists the tiers from most to least trusted.
func Tiers() []CheckTier {
	return []CheckTier{TierCore, TierExperimental}
}

// ParseTier returns the tier named by value.
func ParseTier(value string) (CheckTier, error) {
	for _, tier := range Tiers() {
		if tier.String() == value {
			return tier, nil
		}
	}
	if value == "extended" {
		// The extended tier was removed on 2026-09-25 while it held no checks.
		return 0, errors.New("the extended tier was removed; use core or experimental")
	}
	return 0, fmt.Errorf("unknown tier %q (expected core or experimental)", value)
}

// Within reports whether tier is at or above the trust of ceiling, so a
// ceiling of experimental admits core and experimental checks.
func (tier CheckTier) Within(ceiling CheckTier) bool {
	return tierRank(tier) <= tierRank(ceiling)
}

func tierRank(tier CheckTier) int {
	return slices.Index(Tiers(), tier)
}

var checkKindLabels = [...]string{0: "", KindDefect: "defect", KindHazard: "hazard", KindPolicy: "policy"}

// String returns the stable presentation label.
func (value CheckKind) String() string { return enumtext.Name(value, checkKindLabels[:]) }

// MarshalText preserves string labels in text and JSON output.
func (value CheckKind) MarshalText() ([]byte, error) {
	return enumtext.Encode(value, checkKindLabels[:])
}

// UnmarshalText accepts only domain labels and leaves value unchanged on error.
func (value *CheckKind) UnmarshalText(text []byte) error {
	parsed, err := enumtext.Decode[CheckKind](text, checkKindLabels[:])
	if err == nil {
		*value = parsed
	}
	return err
}

var checkTierLabels = [...]string{0: "", TierCore: "core", TierExperimental: "experimental"}

// String returns the stable presentation label.
func (value CheckTier) String() string { return enumtext.Name(value, checkTierLabels[:]) }

// MarshalText preserves string labels in text and JSON output.
func (value CheckTier) MarshalText() ([]byte, error) {
	return enumtext.Encode(value, checkTierLabels[:])
}

// UnmarshalText accepts only domain labels and leaves value unchanged on error.
func (value *CheckTier) UnmarshalText(text []byte) error {
	parsed, err := enumtext.Decode[CheckTier](text, checkTierLabels[:])
	if err == nil {
		*value = parsed
	}
	return err
}
