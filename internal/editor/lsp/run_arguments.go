package lsp

import (
	"path/filepath"
	"sort"
	"strings"
)

// relativePatterns names package directories the way the go command expects
// them from the module root: relative when inside it, absolute otherwise.
func relativePatterns(root string, directories []string) []string {
	patterns := make([]string, 0, len(directories))
	for _, directory := range directories {
		relative, err := filepath.Rel(root, directory)
		if err != nil || !filepath.IsLocal(relative) {
			// A package of a sibling workspace module is named by its
			// absolute directory.
			patterns = append(patterns, directory)
			continue
		}
		patterns = append(patterns, "./"+filepath.ToSlash(relative))
	}
	sort.Strings(patterns)
	return patterns
}

// shellJoin renders arguments for the log. It is for reading, not for
// execution: the command itself never passes through a shell.
func shellJoin(arguments []string) string {
	quoted := make([]string, len(arguments))
	for index, argument := range arguments {
		if strings.ContainsAny(argument, " \t\"'") {
			argument = "'" + strings.ReplaceAll(argument, "'", `'\''`) + "'"
		}
		quoted[index] = argument
	}
	return strings.Join(quoted, " ")
}
