package channelprotocol

import (
	"go/types"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/syntax"
)

// The prerequisite owns effect collection, bindings and serialization. This
// analyzer owns which complete event relationships prove a waiting cycle.
type (
	summaryEngine     struct{ *concurrencyfacts.Engine }
	summary           = concurrencyfacts.Summary
	operation         = concurrencyfacts.Operation
	resourceReference = concurrencyfacts.Reference
)

const (
	sendOperation      = concurrencyfacts.Send
	receiveOperation   = concurrencyfacts.Receive
	closeOperation     = concurrencyfacts.Close
	groupAddOperation  = concurrencyfacts.GroupAdd
	groupDoneOperation = concurrencyfacts.GroupDone
	groupWaitOperation = concurrencyfacts.GroupWait
)

func waitGroupPointer(value types.Type) bool {
	pointer, ok := value.Underlying().(*types.Pointer)
	return ok && syntax.NamedType(pointer.Elem(), "sync", "WaitGroup")
}
