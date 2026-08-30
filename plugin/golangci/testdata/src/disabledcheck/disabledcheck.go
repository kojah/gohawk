package disabledcheck

import "context"

func misplaced(value string, ctx context.Context) {}

func accept(context.Context) {}

func nilContext() {
	accept(nil) // want "do not pass nil context.Context"
}
