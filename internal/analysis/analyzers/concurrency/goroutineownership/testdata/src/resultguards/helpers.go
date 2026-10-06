package resultguards

// NeverFails is an exported helper whose error result is always nil.
func NeverFails() error { return nil }
