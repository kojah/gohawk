package ssaflow

import "strings"

// JoinAccessPath renders a static field/index path for a summary or key.
func JoinAccessPath(path []string) string { return strings.Join(path, "/") }

// SplitAccessPath reverses JoinAccessPath.
func SplitAccessPath(joined string) []string {
	if joined == "" {
		return nil
	}
	return strings.Split(joined, "/")
}
