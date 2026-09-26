package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/token"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/kojah/gohawk/internal/analyzers/concurrency/lockorder"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
)

// The locks view prints the order graph lockorder recorded for each package:
// every pair of lock classes held together, with one witness of the order,
// and the cycles the analyzer reported over those edges. The cycles come from
// the analyzer's result, not a search here, so an edge on a cycle the
// analyzer declined, such as one serialized by a common guard, is not marked.

func printLocks(arguments []string, output, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("locks", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	dot := flags.Bool("dot", false, "print a Graphviz digraph with reported cycles in red")
	includeTests := flags.Bool("tests", false, "also load the package's test variant")
	flags.Usage = func() {
		writeLine(errorsOutput, "usage: gohawk dump locks [-dot] [-tests] package...")
		flags.PrintDefaults()
	}
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	analyzer := lockorder.Analyzer()
	graph, err := analyzeForDump(flags.Args(), *includeTests, []*analysis.Analyzer{analyzer})
	if err != nil {
		return err
	}
	var buffer bytes.Buffer
	for _, action := range graph.Roots {
		if action.Err != nil {
			return action.Err
		}
		orders, ok := action.Result.(*lockorder.Graph)
		if !ok || len(orders.Edges) == 0 {
			continue
		}
		if *dot {
			writeLockDot(&buffer, action, orders)
		} else {
			writeLockOrders(&buffer, action, orders)
		}
	}
	if buffer.Len() == 0 {
		return errors.New("no package holds two locks at once")
	}
	_, err = output.Write(buffer.Bytes())
	return err
}

func writeLockOrders(buffer *bytes.Buffer, action *checker.Action, orders *lockorder.Graph) {
	directory, _ := os.Getwd()
	position := func(pos token.Pos) string {
		return relativePosition(directory, action.Package.Fset.Position(pos).String())
	}
	fmt.Fprintf(buffer, "// %s\n", action.Package.PkgPath)
	buffer.WriteString("//   order edges (held -> acquired):\n")
	for _, edge := range orders.Edges {
		fmt.Fprintf(buffer, "//     %s -> %s: held at %s, acquired at %s", modeName(edge.Held, edge.HeldMode), modeName(edge.Acquired, edge.AcquiredMode),
			position(edge.HeldAt), position(edge.AcquiredAt))
		for _, call := range edge.Via {
			fmt.Fprintf(buffer, ", via %s at %s", strings.TrimPrefix(call.Message, "calls "), position(call.Pos))
		}
		if len(edge.Guards) != 0 {
			buffer.WriteString(" (while holding " + strings.Join(edge.Guards, ", ") + ")")
		}
		if edge.Variant {
			buffer.WriteString(" (loop-variant lock)")
		}
		buffer.WriteString("\n")
	}
	if orders.Full {
		buffer.WriteString("//   edge cap reached: later orders were not recorded\n")
	}
	if len(orders.Cycles) == 0 {
		buffer.WriteString("//   no cycle reported\n")
		return
	}
	buffer.WriteString("//   cycles reported:\n")
	for _, cycle := range orders.Cycles {
		names := []string{orders.Edges[cycle[0]].Held}
		for _, index := range cycle {
			names = append(names, orders.Edges[index].Acquired)
		}
		buffer.WriteString("//     " + strings.Join(names, " -> ") + "\n")
	}
}

func modeName(class, mode string) string {
	if mode == "RLock" {
		return class + " (read)"
	}
	return class
}

func writeLockDot(buffer *bytes.Buffer, action *checker.Action, orders *lockorder.Graph) {
	onCycle := map[int]bool{}
	for _, cycle := range orders.Cycles {
		for _, index := range cycle {
			onCycle[index] = true
		}
	}
	fmt.Fprintf(buffer, "digraph %s {\n", strconv.Quote(action.Package.PkgPath))
	for index, edge := range orders.Edges {
		at := action.Package.Fset.Position(edge.AcquiredAt)
		attributes := "label=" + strconv.Quote(shortFile(at.Filename)+":"+strconv.Itoa(at.Line))
		if onCycle[index] {
			attributes += ", color=red"
		}
		if edge.HeldMode == "RLock" || edge.AcquiredMode == "RLock" {
			attributes += ", style=dashed"
		}
		fmt.Fprintf(buffer, "  %s -> %s [%s];\n", strconv.Quote(edge.Held), strconv.Quote(edge.Acquired), attributes)
	}
	buffer.WriteString("}\n")
}

func shortFile(filename string) string {
	if at := strings.LastIndex(filename, "/"); at >= 0 {
		return filename[at+1:]
	}
	return filename
}
