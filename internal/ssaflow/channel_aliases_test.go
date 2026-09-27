package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestChannelValues(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "channels", `package channels

var kept chan int

func send(out chan<- int) { out <- 1 }

func captured() int {
	results := make(chan int)
	go func() { results <- 1 }()
	return <-results
}

func passed() int {
	results := make(chan int)
	go send(results)
	return <-results
}

func escapes() {
	results := make(chan int)
	kept = results
}
`)
	cases := map[string][]string{
		"captured": {"send", "receive"},
		"passed":   {"send", "receive"},
		"escapes":  {"other"},
	}
	for name, want := range cases {
		made := ssaflow.InstructionsOf[*ssa.MakeChan](pkg.Func(name))[0]
		_, uses := ssaflow.ChannelValues(made)
		var got []string
		for _, use := range uses {
			switch typed := use.Instruction.(type) {
			case *ssa.Send:
				got = append(got, "send")
			case *ssa.UnOp:
				if typed.Op.String() == "<-" {
					got = append(got, "receive")
				}
			default:
				got = append(got, "other")
			}
		}
		if len(got) != len(want) {
			t.Errorf("%s: uses = %v, want %v", name, got, want)
			continue
		}
		for _, kind := range want {
			found := false
			for _, use := range got {
				found = found || use == kind
			}
			if !found {
				t.Errorf("%s: uses = %v, want %v", name, got, want)
			}
		}
	}
}
