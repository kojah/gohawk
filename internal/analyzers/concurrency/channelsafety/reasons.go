package channelsafety

// safetyReason records send-after-close evidence independently of trace text.
type safetyReason uint8

const (
	safetyReasonNone safetyReason = iota
	safetyReasonReachableSend
	safetyReasonIdentityUnproven
	safetyReasonSendAfterClose
	safetyReasonCount
)

var safetyReasonCodes = [...]string{
	safetyReasonNone:             "",
	safetyReasonReachableSend:    "send-reachable-after-close",
	safetyReasonIdentityUnproven: "send-channel-identity-not-proven",
	safetyReasonSendAfterClose:   "send-after-close-proven",
}

func (reason safetyReason) String() string {
	if int(reason) >= len(safetyReasonCodes) {
		return "invalid-channel-safety-reason"
	}
	return safetyReasonCodes[reason]
}
