package main

import (
	"encoding"
	"encoding/json"
	"testing"

	gohawk "github.com/kojah/gohawk/analyzers"
)

// Published manifest classifications are textual even though their owning Go
// domains are numeric. Explicit pointers also exercise the public decoder API.
func TestManifestClassificationDecoders(t *testing.T) {
	for _, test := range []struct {
		value encoding.TextUnmarshaler
		label string
	}{
		{new(gohawk.CheckKind), "defect"},
		{new(gohawk.CheckTier), "core"},
	} {
		t.Run(test.label, func(t *testing.T) {
			if err := test.value.UnmarshalText([]byte(test.label)); err != nil {
				t.Fatal(err)
			}
			want := `"` + test.label + `"`
			got, err := json.Marshal(test.value)
			if err != nil || string(got) != want {
				t.Fatalf("classification = %s, %v; want %s", got, err, want)
			}
			if err := test.value.UnmarshalText([]byte("invalid")); err == nil {
				t.Fatal("unknown manifest classification accepted")
			}
			got, err = json.Marshal(test.value)
			if err != nil || string(got) != want {
				t.Fatal("failed classification decode changed metadata")
			}
		})
	}
}
