// Package enumtext preserves textual output for domain-owned numeric enums.
package enumtext

import "fmt"

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

// Decode resolves a wire label, including the unset zero label. Callers should
// assign the returned value only on success to preserve their previous state.
func Decode[N ~uint8](text []byte, labels []string) (N, error) {
	for index, label := range labels {
		if string(text) == label {
			return N(index), nil
		}
	}
	return 0, fmt.Errorf("unknown enum label %q", text)
}
