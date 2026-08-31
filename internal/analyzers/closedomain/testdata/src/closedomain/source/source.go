package enumfieldsource

type Record struct {
	State string // want "field State uses a closed string domain; define a named string type and constants" State:"closedStringDomain"
}

func Ready(value Record) bool {
	return value.State == "ready" || value.State == "done"
}
