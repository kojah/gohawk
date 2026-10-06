package cancellationownership

import (
	"context"

	"cancellationdep"
)

// Argument cases. A helper that calls the cancel function only behind a
// Boolean parameter settles it at a call whose constant argument selects
// that branch, locally or through an imported summary. Locally, a variable
// flag or a constant selecting the other branch leaves it uncalled on some
// path.
//
// An imported helper is settled through its summary: an unconditional call,
// or a case the constant argument selects. A helper that calls cancel on
// another goroutine, or calls a different argument, settles nothing here.
//
// Gap: an imported helper called with the constant that skips the call is
// not reported. Summary cases carry only positive guarantees, so nothing
// says the skipping branch never cancels, and an imported helper that may
// cancel stays unknown for this analyzer.

func stopUnlessKept(cancel context.CancelFunc, keep bool) {
	if keep {
		return
	}
	cancel()
}

func constantFlagCancelsLocally() {
	_, cancel := context.WithCancel(context.Background())
	stopUnlessKept(cancel, false)
}

func constantFlagKeepsLocally() {
	_, cancel := context.WithCancel(context.Background()) // want "cancel function from context.WithCancel is not called on every return path"
	stopUnlessKept(cancel, true)
}

func variableFlagKeepsLocally(keep bool) {
	_, cancel := context.WithCancel(context.Background()) // want "cancel function from context.WithCancel is not called on every return path"
	stopUnlessKept(cancel, keep)
}

func constantFlagCancelsImported() {
	_, cancel := context.WithCancel(context.Background())
	cancellationdep.MaybeInvoke(cancel, true)
}

func importedCaseOnAnotherGoroutineIsUnknown() {
	_, cancel := context.WithCancel(context.Background())
	cancellationdep.InvokeLater(cancel, true)
}

func importedCaseCancelsOnlyTheOtherArgument() {
	_, cancel := context.WithCancel(context.Background())
	_, other := context.WithCancel(context.Background())
	cancellationdep.InvokeOther(cancel, other, true)
}
