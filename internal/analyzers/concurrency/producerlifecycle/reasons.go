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
	reasonOneShotWorker
	reasonChannelEscapes
	reasonWorkerOperationUnsupported
	reasonWorkerLaunchedRepeatedly
	reasonChannelBuffered
	reasonCallerOperationsMixed
	reasonCallerCompletesEveryReturn
	reasonReturnWithoutCounterpart
	reasonLocalChannel
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
	reasonOneShotWorker:              "one-shot-worker",
	reasonChannelEscapes:             "channel-escapes",
	reasonWorkerOperationUnsupported: "worker-operation-unsupported",
	reasonWorkerLaunchedRepeatedly:   "worker-launched-repeatedly",
	reasonChannelBuffered:            "channel-buffered",
	reasonCallerOperationsMixed:      "caller-operations-mixed",
	reasonCallerCompletesEveryReturn: "caller-completes-every-return",
	reasonReturnWithoutCounterpart:   "return-without-counterpart",
	reasonLocalChannel:               "local-channel",
}

func (reason producerReason) String() string {
	if int(reason) >= len(producerReasonCodes) {
		return "invalid-producer-reason"
	}
	return producerReasonCodes[reason]
}

type producerProof struct {
	State  ssaflow.EvidenceState
	Reason producerReason
}

func (proof producerProof) Proven() bool { return proof.State == ssaflow.EvidenceProven }
func (proof producerProof) Known() bool  { return proof.State != ssaflow.EvidenceUnknown }
