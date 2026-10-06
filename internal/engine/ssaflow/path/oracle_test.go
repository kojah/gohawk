package path_test

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// The counted-loop and stable-guard proofs are claims about what a program
// does when it runs, so these tests run the fixture and compare: a proven
// loop count must equal the iterations executed, and an obligation the walk
// calls honored because a stable guard prunes a path must be met on every
// execution.

const oracleSource = `package main

import "fmt"

var ticks = map[string]int{}

func tick(name string) { ticks[name]++ }

func below()     { for i := 0; i < 3; i++ { tick("below") } }
func upTo()      { for i := 0; i <= 3; i++ { tick("upTo") } }
func offset()    { for i := 2; i < 7; i++ { tick("offset") } }
func stepped()   { for i := 0; i < 7; i += 2 { tick("stepped") } }
func notEqual()  { for i := 0; i != 5; i++ { tick("notEqual") } }
func ranged()    { for range 4 { tick("ranged") } }
func downward()  { for i := 10; i > 0; i-- { tick("downward") } }

var owned int

func start() {}
func own()   { owned++ }

func stable(b bool)   { start(); if b { own() }; if !b { own() } }
func violated(b bool) { start(); if b { own() } }

func main() {
	below(); upTo(); offset(); stepped(); notEqual(); ranged(); downward()
	for name, count := range ticks {
		fmt.Println("loop", name, count)
	}
	for _, b := range []bool{false, true} {
		owned = 0; stable(b); fmt.Println("owned stable", b, owned)
		owned = 0; violated(b); fmt.Println("owned violated", b, owned)
	}
}
`

type oracleRun struct {
	loops map[string]int
	// unowned records the functions some execution returned from without
	// calling own.
	unowned map[string]bool
}

func runOracle(t *testing.T) oracleRun {
	t.Helper()
	directory := t.TempDir()
	for name, content := range map[string]string{"go.mod": "module oracle\n\ngo 1.24\n", "main.go": oracleSource} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.CommandContext(t.Context(), "go", "run", ".")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("run oracle fixture: %v", err)
	}
	run := oracleRun{loops: map[string]int{}, unowned: map[string]bool{}}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		switch {
		case len(fields) == 3 && fields[0] == "loop":
			run.loops[fields[1]], _ = strconv.Atoi(fields[2])
		case len(fields) == 4 && fields[0] == "owned" && fields[3] == "0":
			run.unowned[fields[1]] = true
		}
	}
	return run
}

// loopHeader returns the block a back edge returns to.
func loopHeader(t *testing.T, function *ssa.Function) *ssa.BasicBlock {
	t.Helper()
	for _, block := range function.Blocks {
		if slices.ContainsFunc(block.Preds, block.Dominates) {
			return block
		}
	}
	t.Fatalf("%s: no loop", function.Name())
	return nil
}

func TestCountedLoopMatchesExecution(t *testing.T) {
	run := runOracle(t)
	pkg := ssaflowtest.BuildPackage(t, "main", oracleSource)
	proven := 0
	for name, executed := range run.loops {
		header := loopHeader(t, pkg.Func(name))
		for proof, loop := range map[string]ssapath.CountedLoop{
			"loop":   ssapath.ProveCountedLoop(header, 100, proofs.NewSearchBudget(proofs.QueryBudget)),
			"region": ssapath.ProveCountedRegion(header, 100, proofs.NewSearchBudget(proofs.QueryBudget)),
		} {
			if !loop.Proven() {
				continue
			}
			proven++
			t.Logf("%s %s: proven %d, executed %d", name, proof, loop.Count, executed)
			if loop.Count != executed {
				t.Errorf("%s %s: proven count %d, executed %d", name, proof, loop.Count, executed)
			}
		}
	}
	// The plain ascending loop is the shape the proof is built for; if it
	// stops being proven, the comparison above checks nothing.
	if proven == 0 {
		t.Fatal("no loop was proven, so the oracle compared nothing")
	}
}

func TestStableGuardPruningMatchesExecution(t *testing.T) {
	run := runOracle(t)
	pkg := ssaflowtest.BuildPackage(t, "main", oracleSource)
	for _, name := range []string{"stable", "violated"} {
		function := pkg.Func(name)
		outcome := ssapath.EvaluateObligation(ssapath.ObligationFlow{
			Start: callNamed(t, function, "start"), Instruction: callsTo("own"),
		})
		if honored := outcome == ssapath.ObligationHonored; honored == run.unowned[name] {
			t.Errorf("%s: walk outcome %d, but an execution returned unowned = %t", name, outcome, run.unowned[name])
		}
	}
}
