package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

// The dump command groups the debugging views of what the analyzers derived:
// the SSA they read, the facts they publish, and the models they build. Each
// view is a reading aid for one package or function at a time; none of them
// analyzes anything the ordinary run does not.

type dumpView struct {
	name    string
	summary string
	print   func(arguments []string, output, errorsOutput io.Writer) error
}

func dumpViews() []dumpView {
	return []dumpView{
		{"ssa", "the SSA form the analyzers see", printSSA},
		{"facts", "the facts the analyzers import", printFacts},
		{"heap", "how the heap model was derived", printHeap},
		{"budget", "where the analysis spent its time and which searches ran out of budget", printBudget},
		{"locks", "the lock order graph and the cycles lockorder reported", printLocks},
		{"trace", "each proof the analyzers traced, grouped by function and candidate", printTrace},
	}
}

func runDump(arguments []string, output, errorsOutput io.Writer) error {
	if len(arguments) == 0 || arguments[0] == "-h" || arguments[0] == "-help" || arguments[0] == "--help" {
		printDumpUsage(errorsOutput)
		if len(arguments) == 0 {
			return errors.New("a view is required")
		}
		return flag.ErrHelp
	}
	for _, view := range dumpViews() {
		if view.name == arguments[0] {
			return view.print(arguments[1:], output, errorsOutput)
		}
	}
	printDumpUsage(errorsOutput)
	return fmt.Errorf("unknown view %q", arguments[0])
}

func printDumpUsage(output io.Writer) {
	writeLine(output, "usage: gohawk dump VIEW [flags] package...")
	writeLine(output, "\nViews:")
	for _, view := range dumpViews() {
		writeFormattedf(output, "  %-12s %s\n", view.name, view.summary)
	}
	writeLine(output, "\nRun 'gohawk dump VIEW -h' for a view's flags.")
}
