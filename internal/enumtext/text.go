// Package enumtext preserves textual output for domain-owned numeric enums.
package enumtext

import (
	"errors"
	"fmt"
)

// Name presents a valid label, including the unset zero label. Invalid numeric
// values remain visible rather than impersonating an unset value.
func Name[N ~uint8](value N, labels []string) string {
	if int(value) < len(labels) {
		return labels[value]
	}
	return fmt.Sprintf("invalid(%d)", value)
}

// Encode returns a stable wire label and rejects values outside the domain.
func Encode[N ~uint8](value N, labels []string) ([]byte, error) {
	if int(value) >= len(labels) {
		return nil, fmt.Errorf("invalid enum value %d", value)
	}
	return []byte(labels[value]), nil
}

// Decode resolves a wire label into destination, including the unset zero
// label. Unknown labels leave the previous value intact; a nil destination
// returns an error.
func Decode[N ~uint8](destination *N, text []byte, labels []string) error {
	if destination == nil {
		return errors.New("nil enum destination")
	}
	for index, label := range labels {
		if string(text) == label {
			*destination = N(index)
			return nil
		}
	}
	return fmt.Errorf("unknown enum label %q", text)
}
