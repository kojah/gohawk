package cli

import (
	"bytes"
	"cmp"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/types"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis/checker"
)

// The trace view reads the evidence tracer as data and prints each proof
// together: for every function, the candidates the analyzers weighed, and
// under each the evidence, considered suppressions, and decision, in the
// order the proof produced them. It prints what the analyzers emitted and
// decides nothing; a proof step that emits nothing is a tracing gap to fill
// in the analyzer, not something this view can reconstruct.

func printTrace(arguments []string, output, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("trace", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	functionFilter := flags.String("func", "", "print only proofs for candidates in the function with this name")
	analyzerList := flags.String("analyzer", "", "comma-separated analyzers to run and trace (default all)")
	candidate := flags.String("candidate", "", "print only proofs of candidates whose position contains this path[:line]")
	decisions := flags.Bool("decisions", false, "print only candidates, labels, considered suppressions, and decisions, not evidence")
	includeTests := flags.Bool("tests", false, "also load the package's test variant")
	flags.Usage = func() {
		writeLine(errorsOutput, "usage: gohawk dump trace [-func NAME] [-analyzer NAMES] [-candidate PATH[:LINE]] [-decisions] [-tests] package...")
		flags.PrintDefaults()
	}
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	analyzers, err := catalogAnalyzers(*analyzerList)
	if err != nil {
		return err
	}
	selectors := []string{"all"}
	if *analyzerList != "" {
		selectors = selectors[:0]
		for _, analyzer := range analyzers {
			selectors = append(selectors, analyzer.Name)
		}
	}
	var records []analysisTrace.Record
	restore := analysisTrace.Capture(selectors, *candidate, func(record analysisTrace.Record) { records = append(records, record) })
	graph, err := analyzeForDump(flags.Args(), *includeTests, analyzers)
	restore()
	if err != nil {
		return err
	}
	proofs := groupProofs(records, newDeclarationIndex(graph), *functionFilter, *decisions)
	if len(proofs) == 0 {
		return errors.New("no traced proof matched")
	}
	var buffer bytes.Buffer
	writeProofs(&buffer, proofs)
	_, err = output.Write(buffer.Bytes())
	return err
}

// tracedFunction is one function's proofs, in the order their candidates
// were first traced.
type tracedFunction struct {
	name   string
	order  declaration
	proofs []*tracedProof
}

type tracedProof struct {
	check, candidate string
	steps            []analysisTrace.Record
}

func groupProofs(records []analysisTrace.Record, index declarationIndex, filter string, decisionsOnly bool) []*tracedFunction {
	functions := map[string]*tracedFunction{}
	proofs := map[[3]string]*tracedProof{}
	for _, record := range records {
		if decisionsOnly && record.Phase == "evidence" {
			continue
		}
		name, order := index.enclosing(cmp.Or(record.Candidate, record.Position))
		if name == "" {
			name = cmp.Or(record.Function, "(package-wide)")
		}
		if !functionMatches(name, filter) {
			continue
		}
		function := functions[name]
		if function == nil {
			function = &tracedFunction{name: name, order: order}
			functions[name] = function
		}
		check := cmp.Or(record.Check, record.Analyzer)
		key := [3]string{name, check, record.Candidate}
		proof := proofs[key]
		if proof == nil {
			proof = &tracedProof{check: check, candidate: record.Candidate}
			proofs[key] = proof
			function.proofs = append(function.proofs, proof)
		}
		proof.steps = append(proof.steps, record)
	}
	return slices.SortedFunc(maps.Values(functions), func(left, right *tracedFunction) int {
		return cmp.Or(cmp.Compare(left.order.file, right.order.file), cmp.Compare(left.order.start, right.order.start), cmp.Compare(left.name, right.name))
	})
}

// functionMatches accepts a function by its full name or by its last
// element, so -func Close selects (*T).Close.
func functionMatches(name, filter string) bool {
	if filter == "" || name == filter {
		return true
	}
	return strings.HasSuffix(name, "."+filter)
}

func writeProofs(buffer *bytes.Buffer, functions []*tracedFunction) {
	directory, _ := os.Getwd()
	for _, function := range functions {
		fmt.Fprintf(buffer, "// %s\n", function.name)
		for _, proof := range function.proofs {
			header := "//   " + proof.check
			if proof.candidate != "" {
				header += " at " + relativePosition(directory, proof.candidate)
			}
			buffer.WriteString(header + "\n")
			writeSteps(buffer, directory, proof)
		}
	}
}

// writeSteps prints a proof's steps, folding a run of identical steps into
// one line with a count, since a walk often gives the same answer for every
// instruction it passes.
func writeSteps(buffer *bytes.Buffer, directory string, proof *tracedProof) {
	var previous string
	repeats := 0
	flush := func() {
		if previous == "" {
			return
		}
		if repeats > 1 {
			previous += " ×" + strconv.Itoa(repeats)
		}
		buffer.WriteString(previous + "\n")
	}
	for _, step := range foldUnanswered(proof.steps) {
		var text strings.Builder
		fmt.Fprintf(&text, "//     %-10s %-8s %s", step.phase, step.outcome, step.title)
		if step.position != "" && step.position != proof.candidate {
			text.WriteString(" at " + relativePosition(directory, step.position))
		}
		for _, key := range slices.Sorted(maps.Keys(step.details)) {
			text.WriteString(" " + key + "=" + detailValue(step.details[key]))
		}
		line := text.String()
		if line == previous {
			repeats++
			continue
		}
		flush()
		previous, repeats = line, 1
	}
	flush()
}

// foldUnanswered folds the questions one instruction was asked and did not
// settle into one step. A classifier asks each instruction several questions,
// a release, a transfer, a summary, and the shared evidence names each, so an
// instruction the resource passes by untouched would otherwise take a line per
// question. The folded step lists every question with its reason; a proven
// answer, or a question asked alone, is kept as it was traced.
func foldUnanswered(steps []analysisTrace.Record) []traceRow {
	folded := make([]traceRow, 0, len(steps))
	for start := 0; start < len(steps); {
		end := start + 1
		for end < len(steps) && unanswered(steps[start]) && unanswered(steps[end]) && sameInstruction(steps[start], steps[end]) {
			end++
		}
		if end-start == 1 {
			step := steps[start]
			folded = append(folded, traceRow{step.Phase, step.Outcome, step.Reason, step.Position, step.Details})
		} else {
			folded = append(folded, foldedStep(steps[start:end]))
		}
		start = end
	}
	return folded
}

// unanswered reports a step from the shared evidence that did not prove its
// question.
func unanswered(step analysisTrace.Record) bool {
	return step.Phase == "evidence" && step.Details["question"] != "" &&
		(step.Outcome == analysisTrace.OutcomeRejected || step.Outcome == analysisTrace.OutcomeUnknown)
}

func sameInstruction(left, right analysisTrace.Record) bool {
	return left.Position == right.Position && left.Details["instruction"] == right.Details["instruction"] &&
		left.Details["target"] == right.Details["target"]
}

// traceRow is one printed line of a proof: a traced step, titled by its
// reason, or a fold of several, titled by what the fold means. A title is
// display text, not a reason code: the dump classifies nothing.
type traceRow struct {
	phase    string
	outcome  analysisTrace.Outcome
	title    string
	position string
	details  map[string]string
}

func foldedStep(steps []analysisTrace.Record) traceRow {
	answers := make([]string, 0, len(steps))
	outcome := analysisTrace.OutcomeRejected
	for _, step := range steps {
		answers = append(answers, step.Details["question"]+":"+strings.TrimPrefix(step.Reason, "evidence-"))
		if step.Outcome == analysisTrace.OutcomeUnknown {
			outcome = analysisTrace.OutcomeUnknown
		}
	}
	details := map[string]string{"answers": strings.Join(answers, ",")}
	for _, key := range []string{"instruction", "target", "target_type", "callee"} {
		if value, ok := steps[0].Details[key]; ok {
			details[key] = value
		}
	}
	return traceRow{phase: "evidence", outcome: outcome, title: "unanswered", position: steps[0].Position, details: details}
}

// detailValue quotes a detail only when it would not read as one word.
func detailValue(value string) string {
	if value == "" || strings.ContainsAny(value, " \t\"=") {
		return strconv.Quote(value)
	}
	return value
}

// relativePosition shortens a file:line:col position to the working
// directory when the file is under it.
func relativePosition(directory, position string) string {
	if relative, err := filepath.Rel(directory, position); err == nil && !strings.HasPrefix(relative, "..") {
		return relative
	}
	return position
}

// A declarationIndex finds the function declaration a traced position lies
// in, so every step of a proof is filed under the function it was about even
// when the analyzer did not name one.
type declarationIndex map[string][]declaration

type declaration struct {
	name        string
	file        string
	start, stop int
}

func newDeclarationIndex(graph *checker.Graph) declarationIndex {
	index := declarationIndex{}
	for _, action := range graph.Roots {
		pkg := action.Package
		for _, file := range pkg.Syntax {
			filename := pkg.Fset.Position(file.Pos()).Filename
			if _, seen := index[filename]; seen {
				continue
			}
			declarations := []declaration{}
			for _, declared := range file.Decls {
				function, ok := declared.(*ast.FuncDecl)
				if !ok {
					continue
				}
				declarations = append(declarations, declaration{
					name: pkg.Name + "." + declarationName(function), file: filename,
					start: pkg.Fset.Position(function.Pos()).Line, stop: pkg.Fset.Position(function.End()).Line,
				})
			}
			index[filename] = declarations
		}
	}
	return index
}

// declarationName writes a method as SSA does, (*T).Name or T.Name.
func declarationName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	receiver := types.ExprString(function.Recv.List[0].Type)
	if strings.HasPrefix(receiver, "*") {
		receiver = "(" + receiver + ")"
	}
	return receiver + "." + function.Name.Name
}

func (index declarationIndex) enclosing(position string) (string, declaration) {
	filename, line, ok := splitPosition(position)
	if !ok {
		return "", declaration{}
	}
	for _, declared := range index[filename] {
		if declared.start <= line && line <= declared.stop {
			return declared.name, declared
		}
	}
	return "", declaration{}
}

// splitPosition parses file:line:col, or file:line.
func splitPosition(position string) (string, int, bool) {
	rest, last, found := cutLast(position)
	if !found {
		return "", 0, false
	}
	if filename, line, found := cutLast(rest); found {
		if number, err := strconv.Atoi(line); err == nil {
			return filename, number, true
		}
	}
	number, err := strconv.Atoi(last)
	return rest, number, err == nil
}

func cutLast(text string) (string, string, bool) {
	at := strings.LastIndex(text, ":")
	if at < 0 {
		return text, "", false
	}
	return text[:at], text[at+1:], true
}
