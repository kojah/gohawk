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

func TestServiceLoopReasonCodes(t *testing.T) {
	if len(loopReasonCodes) != int(loopReasonCount) {
		t.Fatal("every service-loop reason needs a boundary spelling")
	}
	seen := map[string]bool{}
	for reason := range loopReasonCount {
		code := reason.String()
		if reason != loopReasonNone && (code == "" || seen[code]) {
			t.Errorf("reason %d: missing or duplicate code %q", reason, code)
		}
		seen[code] = true
	}
	if loopReasonCount.String() != "invalid-service-loop-reason" {
		t.Errorf("invalid reason: %q", loopReasonCount.String())
	}
}
