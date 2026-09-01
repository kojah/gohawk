// Package flagvalue provides shared validated analyzer flag values.
package flagvalue

import (
	"fmt"
	"strings"
)

// Choice validates a flag whose value belongs to a closed set.
type Choice struct {
	value   *string
	allowed map[string]bool
}

// NewChoice creates a validated single-choice flag value.
func NewChoice(value *string, allowed ...string) *Choice {
	choices := make(map[string]bool, len(allowed))
	for _, choice := range allowed {
		choices[choice] = true
	}
	return &Choice{value: value, allowed: choices}
}

func (choice *Choice) String() string {
	if choice == nil || choice.value == nil {
		return ""
	}
	return *choice.value
}

func (choice *Choice) Set(value string) error {
	if !choice.allowed[value] {
		return fmt.Errorf("unknown value %q", value)
	}
	*choice.value = value
	return nil
}

// CommaSeparatedChoice validates each member of a comma-separated flag.
type CommaSeparatedChoice struct {
	value   *string
	allowed map[string]bool
}

// NewCommaSeparatedChoice creates a validated comma-separated flag value.
func NewCommaSeparatedChoice(value *string, allowed ...string) *CommaSeparatedChoice {
	choices := make(map[string]bool, len(allowed))
	for _, choice := range allowed {
		choices[choice] = true
	}
	return &CommaSeparatedChoice{value: value, allowed: choices}
}

func (choice *CommaSeparatedChoice) String() string {
	if choice == nil || choice.value == nil {
		return ""
	}
	return *choice.value
}

func (choice *CommaSeparatedChoice) Set(value string) error {
	for item := range CommaSeparatedSet(value) {
		if !choice.allowed[item] {
			return fmt.Errorf("unknown value %q", item)
		}
	}
	*choice.value = value
	return nil
}

// CommaSeparatedSet parses non-empty comma-separated items.
func CommaSeparatedSet(value string) map[string]bool {
	result := make(map[string]bool)
	for item := range strings.SplitSeq(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result[item] = true
		}
	}
	return result
}
