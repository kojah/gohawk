// Command gohawk runs the GoHawk static analyzers.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/kojah/gohawk"
	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	normalizeVersionFlag(os.Args)
	if generalHelpRequested(os.Args) {
		printAnalyzerGroups()
	}
	multichecker.Main(gohawk.Analyzers()...)
}

func normalizeVersionFlag(arguments []string) {
	for index, argument := range arguments {
		if argument == "-V" {
			arguments[index] = "-V=full"
		}
	}
}

func generalHelpRequested(arguments []string) bool {
	if len(arguments) < 2 {
		return false
	}
	switch arguments[1] {
	case "help", "-h", "--help", "-help":
		return len(arguments) == 2
	default:
		return false
	}
}

func printAnalyzerGroups() {
	fmt.Fprintln(os.Stderr, "GoHawk analyzer groups (select checks with individual -NAME flags):")
	for _, group := range gohawk.AnalyzerGroups() {
		names := make([]string, 0, len(group.Analyzers))
		for _, analyzer := range group.Analyzers {
			names = append(names, analyzer.Name)
		}
		fmt.Fprintf(os.Stderr, "  %s (%s): %s\n", group.Name, group.Doc, strings.Join(names, ", "))
	}
	fmt.Fprintln(os.Stderr)
}
