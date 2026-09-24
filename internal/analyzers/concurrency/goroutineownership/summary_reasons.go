package goroutineownership

// summaryJoinReason classifies this check's evidence independently of upstream model failures.
type summaryJoinReason uint8

const (
	summaryJoinNone summaryJoinReason = iota
	summaryJoinConcurrencyJoinBudgetExhausted
	summaryJoinConcurrencyJoinNotApplicable
	summaryJoinConcurrencyJoinNotSynchronous
	summaryJoinConcurrencySummaryJoin
	summaryJoinConcurrencySummaryNoExactJoin
	summaryJoinReasonCount
)

func (reason summaryJoinReason) String() string {
	switch reason {
	case summaryJoinNone:
		return ""
	case summaryJoinConcurrencyJoinBudgetExhausted:
		return "concurrency-join-budget-exhausted"
	case summaryJoinConcurrencyJoinNotApplicable:
		return "concurrency-join-not-applicable"
	case summaryJoinConcurrencyJoinNotSynchronous:
		return "concurrency-join-not-synchronous"
	case summaryJoinConcurrencySummaryJoin:
		return "concurrency-summary-join"
	case summaryJoinConcurrencySummaryNoExactJoin:
		return "concurrency-summary-no-exact-join"
	default:
		return "invalid-goroutineownership-reason"
	}
}
