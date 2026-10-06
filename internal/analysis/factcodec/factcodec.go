// Package factcodec publishes immutable summaries as opaque, deterministic
// binary facts. Envelope hides nested schemas from gob's per-stream descriptor
// work; CBOR encodes the payload without another set of type descriptors.
// Domain passes still own validation and unknown/proven semantics. This private
// wire format is versioned independently of domain schemas; changing the tool
// also invalidates the Go analysis cache. There is no legacy JSON fallback.
package factcodec

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

const (
	wireHeader      = "GHF\x01"
	maxPayloadBytes = 16 << 20
)

// Modes are immutable and safe to share. Preserve configuration errors rather
// than panicking at initialization if a future options change is invalid.
var (
	encodeMode, encodeModeError = cbor.CoreDetEncOptions().EncMode()
	decodeMode, decodeModeError = (cbor.DecOptions{
		DupMapKey:         cbor.DupMapKeyEnforcedAPF,
		MaxNestedLevels:   64,
		MaxArrayElements:  1 << 20,
		MaxMapPairs:       1 << 16,
		IndefLength:       cbor.IndefLengthForbidden,
		TagsMd:            cbor.TagsForbidden,
		ExtraReturnErrors: cbor.ExtraDecErrorUnknownField,
	}).DecMode()
)

// Encode serializes a fact deterministically, rejecting oversized payloads.
func Encode(fact any) ([]byte, error) {
	if encodeModeError != nil {
		return nil, encodeModeError
	}
	payload, err := encodeMode.Marshal(fact)
	if err != nil {
		return nil, err
	}
	if len(payload) == 1 && payload[0] == 0xf6 {
		return nil, errors.New("nil fact payload")
	}
	if len(payload) > maxPayloadBytes {
		return nil, fmt.Errorf("fact payload exceeds %d bytes", maxPayloadBytes)
	}
	return append([]byte(wireHeader), payload...), nil
}

// Decode fills a fact from its versioned encoding. Size, nesting, collection,
// duplicate-key and unknown-field checks reject corrupt or incompatible facts.
// Callers wanting atomic replacement should use Envelope.GobDecode.
func Decode(data []byte, fact any) error {
	if decodeModeError != nil {
		return decodeModeError
	}
	if len(data) > maxPayloadBytes+len(wireHeader) {
		return fmt.Errorf("fact payload exceeds %d bytes", maxPayloadBytes)
	}
	if !bytes.HasPrefix(data, []byte(wireHeader)) {
		return errors.New("unsupported fact encoding")
	}
	if len(data) == len(wireHeader)+1 && (data[len(wireHeader)] == 0xf6 || data[len(wireHeader)] == 0xf7) {
		// CBOR treats null and undefined as absent. Neither may manufacture
		// a zero summary, especially a package marker claiming completeness.
		return errors.New("absent fact payload")
	}
	return decodeMode.Unmarshal(data[len(wireHeader):], fact)
}
