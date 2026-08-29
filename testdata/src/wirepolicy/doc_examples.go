package wirepolicy

//gohawk:example flagged Missing serialization tags
type EventRow struct {
	ID   string // want "serialized field ID requires an explicit json or toml tag"
	Kind string // want "serialized field Kind requires an explicit json or toml tag"
}

//gohawk:example end

//gohawk:example flagged Positional wire struct literal
type TaggedEventRow struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

var event = TaggedEventRow{"42", "created"} // want "persisted or wire struct literal must use field keys"

//gohawk:example end

//gohawk:example ok
type AuditRow struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

var audit = AuditRow{ID: "42", Kind: "created"}

//gohawk:example end
