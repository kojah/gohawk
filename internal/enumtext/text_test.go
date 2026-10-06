package enumtext_test

import (
	"encoding"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/kojah/gohawk/analyzers"
	"github.com/kojah/gohawk/internal/enumtext"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/trace"
)

// Exercise the published enum types, including their unset values, rather than
// just the codec. JSON must keep its previous labels and reject unknown input
// without overwriting evidence already held by a receiver.
func TestDomainWireLabels(t *testing.T) {
	for _, test := range []struct {
		value encoding.TextMarshaler
		label string
	}{
		{analyzers.CheckKind(0), ""},
		{analyzers.CheckKindDefect, "defect"},
		{analyzers.CheckKindHazard, "hazard"},
		{analyzers.CheckKindPolicy, "policy"},
		{analyzers.CheckTier(0), ""},
		{analyzers.CheckTierCore, "core"},
		{analyzers.CheckTierExperimental, "experimental"},
		{proofs.EvidenceProvenance(0), ""},
		{proofs.EvidenceFromLocalSSA, "local-ssa"},
		{proofs.EvidenceFromImportedFact, "imported-fact"},
		{trace.Outcome(0), ""},
		{trace.OutcomeObserved, "observed"},
		{trace.OutcomeAccepted, "accepted"},
		{trace.OutcomeRejected, "rejected"},
		{trace.OutcomeUnknown, "unknown"},
	} {
		t.Run(reflect.TypeOf(test.value).String()+"/"+test.label, func(t *testing.T) {
			got, err := json.Marshal(test.value)
			want, _ := json.Marshal(test.label)
			if err != nil || string(got) != string(want) {
				t.Fatalf("marshal = %s, %v; want %s", got, err, want)
			}
			target := reflect.New(reflect.TypeOf(test.value))
			if err := json.Unmarshal(got, target.Interface()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(target.Elem().Interface(), test.value) {
				t.Fatalf("round trip = %v", target.Elem())
			}
			if err := json.Unmarshal([]byte(`"unrecognized"`), target.Interface()); err == nil {
				t.Fatal("unknown label accepted")
			}
			if !reflect.DeepEqual(target.Elem().Interface(), test.value) {
				t.Fatal("failed decode changed receiver")
			}
			if err := json.Unmarshal([]byte(`1`), target.Interface()); err == nil {
				t.Fatal("numeric wire value accepted")
			}
		})
	}
}

func TestInvalidDomainValues(t *testing.T) {
	for _, value := range []encoding.TextMarshaler{
		analyzers.CheckKind(255), analyzers.CheckTier(255), proofs.EvidenceProvenance(255), trace.Outcome(255),
	} {
		if _, err := json.Marshal(value); err == nil {
			t.Errorf("%T accepted invalid numeric value", value)
		}
	}
}

func TestDecodeNilDestination(t *testing.T) {
	if err := enumtext.Decode[uint8](nil, []byte(""), []string{""}); err == nil {
		t.Fatal("nil destination accepted")
	}
}
