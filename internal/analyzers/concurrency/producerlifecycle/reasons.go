package producerlifecycle

import "github.com/kojah/gohawk/internal/ssaflow"

// Producer policy owns its reasons. A shared SSA relationship is supporting
// evidence, not a container for analyzer-specific string classifications.
type producerReason uint8

const (
	reasonNone producerReason = iota
	reasonProducerSend
	reasonReceiverDoesNotReturn
	reasonProducerCountUnknown
	reasonReceiverObligationUnknown
	reasonProducerExceedsReceives
	reasonProducerWithinReceiveCount
	reasonReceiverMayDrain
	reasonWorkerChannelUsesUnknown
	reasonWorkerChannelUsesComplete
	reasonProducerLaunch
	reasonBuiltinNotReceive
	reasonReceiverHelperUnknown
	reasonReceiverHelperComplete
	reasonAsynchronousReceiver
	producerReasonCount
)

func (reason producerReason) String() string {
	switch reason {
	case reasonNone:
		return ""
	case reasonProducerSend:
		return "producer-send"
	case reasonReceiverDoesNotReturn:
		return "receiver-does-not-return"
	case reasonProducerCountUnknown:
		return "producer-count-unknown"
	case reasonReceiverObligationUnknown:
		return "receiver-obligation-unknown"
	case reasonProducerExceedsReceives:
		return "producer-exceeds-receives"
	case reasonProducerWithinReceiveCount:
		return "producer-within-receive-count"
	case reasonReceiverMayDrain:
		return "receiver-may-drain"
	case reasonWorkerChannelUsesUnknown:
		return "worker-channel-uses-unknown"
	case reasonWorkerChannelUsesComplete:
		return "worker-channel-uses-complete"
	case reasonProducerLaunch:
		return "producer-launch"
	case reasonBuiltinNotReceive:
		return "builtin-not-receive"
	case reasonReceiverHelperUnknown:
		return "receiver-helper-unknown"
	case reasonReceiverHelperComplete:
		return "receiver-helper-complete"
	case reasonAsynchronousReceiver:
		return "asynchronous-receiver"
	default:
		return "invalid-producer-reason"
	}
}

type producerProof struct {
	State  ssaflow.EvidenceState
	Reason producerReason
}

func (proof producerProof) Proven() bool { return proof.State == ssaflow.EvidenceProven }
func (proof producerProof) Known() bool  { return proof.State != ssaflow.EvidenceUnknown }
