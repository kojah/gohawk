package defaultchecks

import "context"

func misplaced(value string, ctx context.Context) {} // want "context.Context must be first parameter"
