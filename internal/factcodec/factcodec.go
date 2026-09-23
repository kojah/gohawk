// Package factcodec encodes analysis facts for go/analysis. gob compiles a
// decoding engine per type for every stream it opens, and the analysis test
// harness round-trips every inherited fact through a fresh stream, so a
// summary with many fields costs far more to compile than to decode. A fact
// that encodes itself as JSON inside the gob stream leaves gob a single byte
// slice to handle, and JSON builds its encoders once per type for the
// process. The wire format is gohawk's own; nothing outside the analyzers
// reads it.
package factcodec

import "encoding/json"

// Encode serializes a fact.
func Encode(fact any) ([]byte, error) {
	return json.Marshal(fact)
}

// Decode fills a fact from its encoding.
func Decode(data []byte, fact any) error {
	return json.Unmarshal(data, fact)
}
