package producerlifecycle

import (
	proofs "github.com/kojah/gohawk/internal/proof"
)

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
	reasonProducerCountKnown
	reasonReceiverBudgetExhausted
	producerReasonCount
)

var producerReasonCodes = [...]string{
	reasonNone:                       "",
	reasonProducerSend:               "producer-send",
	reasonReceiverDoesNotReturn:      "receiver-does-not-return",
	reasonProducerCountUnknown:       "producer-count-unknown",
	reasonReceiverObligationUnknown:  "receiver-obligation-unknown",
	reasonProducerExceedsReceives:    "producer-exceeds-receives",
	reasonProducerWithinReceiveCount: "producer-within-receive-count",
	reasonReceiverMayDrain:           "receiver-may-drain",
	reasonWorkerChannelUsesUnknown:   "worker-channel-uses-unknown",
	reasonWorkerChannelUsesComplete:  "worker-channel-uses-complete",
	reasonProducerLaunch:             "producer-launch",
	reasonBuiltinNotReceive:          "builtin-not-receive",
	reasonReceiverHelperUnknown:      "receiver-helper-unknown",
	reasonReceiverHelperComplete:     "receiver-helper-complete",
	reasonAsynchronousReceiver:       "asynchronous-receiver",
	reasonProducerCountKnown:         "producer-count-known",
	reasonReceiverBudgetExhausted:    "receiver-budget-exhausted",
}

func (reason producerReason) String() string {
	if int(reason) >= len(producerReasonCodes) {
		return "invalid-producer-reason"
	}
	return producerReasonCodes[reason]
}

type producerProof struct {
	State  proofs.EvidenceState
	Reason producerReason
}

func (proof producerProof) Proven() bool { return proof.State == proofs.EvidenceProven }
func (proof producerProof) Known() bool  { return proof.State != proofs.EvidenceUnknown }
