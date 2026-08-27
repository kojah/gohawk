package enumfield

type record struct {
	State string // want "field State uses a closed string domain; define a named string type and constants" State:"closedStringDomain"
}

type kindRecord struct {
	Kind string // want "field Kind uses a closed string domain; define a named string type and constants" Kind:"closedStringDomain"
}

type parser struct {
	Command string
}

type incidental struct {
	Status string
}

type operation struct {
	Phase string // want "field Phase uses a closed string domain; define a named string type and constants" Phase:"closedStringDomain"
}

type decision struct {
	Action string // want "field Action uses a closed string domain; define a named string type and constants" Action:"closedStringDomain"
}

type localRecord struct {
	State string // want "field State uses a closed string domain; define a named string type and constants" State:"closedStringDomain"
}

type helperRecord struct {
	Status string // want "field Status uses a closed string domain; define a named string type and constants" Status:"closedStringDomain"
}

type localHelperRecord struct {
	Outcome string // want "field Outcome uses a closed string domain; define a named string type and constants" Outcome:"closedStringDomain"
}

type flavor string

type pointerRecord struct {
	Kind *string // want "field Kind uses a closed string domain; define a named string type and constants" Kind:"closedStringDomain"
}

type openRecord struct {
	Mode string
}

type union interface{ decisionKind() }

type approveDecision struct {
	Action string // want "field Action uses a closed string domain; define a named string type and constants" Action:"closedStringDomain"
}

func (approveDecision) decisionKind() {}

type noneDecision struct {
	Action string // want "field Action uses a closed string domain; define a named string type and constants" Action:"closedStringDomain"
}

func (noneDecision) decisionKind() {}

type recordWire struct {
	State string
}

type state string

type typedRecord struct {
	State state
}

func classify(value record) bool {
	switch value.State {
	case "ready":
		return true
	case "done":
		return false
	default:
		return false
	}
}

func classifyKind(value kindRecord) bool {
	return value.Kind == "first" || value.Kind == "second"
}

func parse(value parser) bool {
	return value.Command == "run" || value.Command == "list"
}

func oneComparison(value incidental) bool {
	return value.Status == "ready"
}

func decode(value recordWire) bool {
	return value.State == "ready" || value.State == "done"
}

func operations() []operation {
	return []operation{{Phase: "queued"}, {Phase: "running"}}
}

func decisions() []decision {
	return []decision{{Action: "approve"}, {Action: "none"}}
}

func localValue(ambiguous bool) localRecord {
	state := "unknown"
	if ambiguous {
		state = "ambiguous"
	}
	return localRecord{State: state}
}

func helperValue(failed bool) string {
	if failed {
		return "failed"
	}
	return "passed"
}

func fromHelper(failed bool) helperRecord {
	return helperRecord{Status: helperValue(failed)}
}

func localHelperValue(failed bool) string {
	outcome := "passed"
	if failed {
		outcome = "failed"
	}
	return outcome
}

func fromLocalHelper(failed bool) localHelperRecord {
	return localHelperRecord{Outcome: localHelperValue(failed)}
}

func erased(value flavor) pointerRecord {
	kind := string(value)
	return pointerRecord{Kind: &kind}
}

func remainsOpen(value string) openRecord {
	return openRecord{Mode: value}
}

func openHelper(value string) string { return value }

func remainsOpenThroughHelper(value string) openRecord {
	return openRecord{Mode: openHelper(value)}
}

func unionValue(approve bool) union {
	if approve {
		return approveDecision{Action: "approve"}
	}
	return noneDecision{Action: "none"}
}
