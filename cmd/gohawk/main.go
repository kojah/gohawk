// Command gohawk runs the GoHawk static analyzers.
package main

import (
	"github.com/kojah/gohawk"
	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	multichecker.Main(gohawk.Analyzers()...)
}
