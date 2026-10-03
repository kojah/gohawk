package ssaflow

import "github.com/kojah/gohawk/internal/enumtext"

// EvidenceProvenance identifies the analysis boundary that supplied a proof.
// Zero means no boundary supplied evidence; it retains the empty wire label.
type EvidenceProvenance uint8

const (
	_ EvidenceProvenance = iota
	EvidenceFromLocalSSA
	EvidenceFromImportedFact
)

var evidenceProvenanceLabels = [...]string{0: "", EvidenceFromLocalSSA: "local-ssa", EvidenceFromImportedFact: "imported-fact"}

// String returns the stable presentation label.
func (value EvidenceProvenance) String() string {
	return enumtext.Name(value, evidenceProvenanceLabels[:])
}

// MarshalText preserves string labels in text and JSON output.
func (value EvidenceProvenance) MarshalText() ([]byte, error) {
	return enumtext.Encode(value, evidenceProvenanceLabels[:])
}

// UnmarshalText accepts only domain labels and leaves value unchanged on error.
func (value *EvidenceProvenance) UnmarshalText(text []byte) error {
	return enumtext.Decode(value, text, evidenceProvenanceLabels[:])
}
