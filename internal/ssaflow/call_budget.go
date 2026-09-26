package ssaflow

import (
	"go/token"
	"runtime"
	"strings"
	"sync/atomic"
)

// An interprocedural question can be asked of a call graph too large to walk.
// Mutual recursion is the usual cause: the cycle guard keeps the walk finite,
// but a memo cannot retain an answer the guard cut short, so a densely
// recursive package is re-walked once per route rather than once per function.
// A budget bounds that work by the instructions one question may examine.
//
// The budget deliberately does not decide what exhaustion means. Whether an
// undecided question suppresses a diagnostic or merely fails to prove one
// depends on the polarity of the proof being sought, and that is the caller's
// policy: a walk that claims an obligation was met must not claim it on a
// guess, while a walk that claims an obligation remains open must not invent
// one. Callers ask Spend before each step and choose their own answer when it
// reports false.

// The two shared bounds name how much one question may cost. They are the
// defaults a caller reaches for when it has no reason of its own; a caller
// with one, such as a whole-package caller-set walk, declares a named
// constant beside the proof that explains it. A bare number at a
// construction site is not a decision, so the architecture tests reject it.
const (
	// QueryBudget bounds one local question: a storage identity, a
	// projection, a value's uses, or one callee walked for a completion. It
	// is also what NewStorage and NewCallEffects assume for a nil budget.
	QueryBudget = 1000
	// SummaryBudget bounds a question that consults or computes a function
	// summary, or decides feasibility from one: twice a local question,
	// because it walks the callee as well as the caller.
	SummaryBudget = 2000
)

// SearchBudget bounds one interprocedural question by the number of
// instructions it may examine.
type SearchBudget struct {
	remaining int
	limit     int
	exhausted bool
	// site names the code that asked the question, recorded only while an
	// exhaustion recorder listens.
	site     string
	observer Observer
	// parent, when set, is the candidate-wide pool this budget also charges;
	// see Within.
	parent *SearchBudget
}

// NewSearchBudget returns a budget allowing limit instructions.
func NewSearchBudget(limit int) *SearchBudget {
	budget := &SearchBudget{remaining: limit, limit: limit}
	if exhaustionRecording.Load() != nil {
		budget.site = budgetSite()
	}
	return budget
}

// Spend charges one instruction and reports whether the walk may continue. A
// nil budget is unbounded, so a caller that does not need one passes nothing.
func (budget *SearchBudget) Spend() bool {
	if budget == nil {
		return true
	}
	if budget.remaining <= 0 {
		budget.exhaust(false)
		return false
	}
	if budget.parent != nil && !budget.parent.Spend() {
		budget.exhaust(true)
		return false
	}
	budget.remaining--
	return true
}

// Observed attaches an observer that hears each give-up of a proof spending
// this budget, and returns the budget so a query can be built inline. A nil
// observer leaves the budget silent; a nil budget stays unbounded and silent.
func (budget *SearchBudget) Observed(observer Observer) *SearchBudget {
	if budget != nil {
		budget.observer = observer
	}
	return budget
}

// Observe reports one give-up. Details are built only when someone is
// listening, so a silent budget costs one nil check at the give-up point and
// nothing on the path that spends it.
func (budget *SearchBudget) Observe(reason EvidenceReason, at token.Pos, build func() map[string]string) {
	if budget == nil || budget.observer == nil {
		return
	}
	var details map[string]string
	if build != nil {
		details = build()
	}
	budget.observer(reason.String(), at, details)
}

// Exhausted reports whether the budget ran out, so a caller can trace the
// bailout and decline to retain an answer that was cut short.
func (budget *SearchBudget) Exhausted() bool {
	return budget != nil && budget.exhausted
}

// exhaust marks the budget spent and, the first time, tells the recorder.
func (budget *SearchBudget) exhaust(pool bool) {
	if budget.exhausted {
		return
	}
	budget.exhausted = true
	if record := exhaustionRecording.Load(); record != nil {
		(*record)(Exhaustion{Site: budget.site, Limit: budget.limit, Pool: pool})
	}
}

// Exhaustion is one question that ran out of budget: the code that asked it,
// its limit, and whether the candidate-wide pool ran out rather than the
// question's own limit. gohawk dump budget collects them, because a question
// cut short answers conservatively and says so nowhere else.
type Exhaustion struct {
	Site  string
	Limit int
	Pool  bool
}

var exhaustionRecording atomic.Pointer[func(Exhaustion)]

// RecordExhaustions hands every budget exhaustion in this process to record
// until the returned function stops it. While nothing records, a budget pays
// one atomic load when it is made and one when it runs out.
func RecordExhaustions(record func(Exhaustion)) (stop func()) {
	exhaustionRecording.Store(&record)
	return func() { exhaustionRecording.Store(nil) }
}

// budgetSite names the function that made the budget, skipping the budget's
// own constructors.
func budgetSite() string {
	callers := make([]uintptr, 8)
	frames := runtime.CallersFrames(callers[:runtime.Callers(3, callers)])
	for {
		frame, more := frames.Next()
		name := frame.Function
		if !strings.HasSuffix(name, "ssaflow.NewSearchBudget") && !strings.HasSuffix(name, "ssaflow.(*SearchBudget).Within") {
			return strings.TrimPrefix(name, "github.com/kojah/gohawk/internal/")
		}
		if !more {
			return ""
		}
	}
}
