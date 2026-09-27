package producerlifecycle

import "testing"

func TestProducerReasonCodes(t *testing.T) {
	want := map[producerReason]string{
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
	}
	if len(want) != int(producerReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range producerReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []producerReason{producerReasonCount, 255} {
		if reason.String() != "invalid-producer-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
