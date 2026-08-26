package enumfieldconsumer

import "enumfieldsource"

type Projection struct {
	State string // want "field State uses a closed string domain; define a named string type and constants" State:"closedStringDomain"
}

func Project(value enumfieldsource.Record) Projection {
	return Projection{State: value.State}
}
