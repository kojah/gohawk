package enumfield

import stdjson "encoding/json"

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

type externalRecord struct {
	Code string `json:"code"`
}

type taggedLocalRecord struct {
	Reason string `json:"reason"` // want "field Reason uses a closed string domain; define a named string type and constants" Reason:"closedStringDomain"
}

type ignoredExternalRecord struct {
	Code string `json:"-"` // want "field Code uses a closed string domain; define a named string type and constants" Code:"closedStringDomain"
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

func decodeExternal(data []byte) externalRecord {
	value := externalRecord{}
	_ = stdjson.Unmarshal(data, &value)
	return value
}

func classifyExternal(value externalRecord) bool {
	return value.Code == "known_one" || value.Code == "known_two"
}

func taggedLocal(first bool) taggedLocalRecord {
	if first {
		return taggedLocalRecord{Reason: "first"}
	}
	return taggedLocalRecord{Reason: "second"}
}

func decodeIgnored(data []byte) ignoredExternalRecord {
	value := ignoredExternalRecord{}
	_ = stdjson.Unmarshal(data, &value)
	return value
}

func classifyIgnored(value ignoredExternalRecord) bool {
	return value.Code == "known_one" || value.Code == "known_two"
}

func unionValue(approve bool) union {
	if approve {
		return approveDecision{Action: "approve"}
	}
	return noneDecision{Action: "none"}
}
