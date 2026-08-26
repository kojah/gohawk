package contextpolicyconfig

import "context"

func misplaced(value string, ctx context.Context) {}

type stored struct {
	ctx context.Context
}

func accept(context.Context) {}

func nilContext() {
	accept(nil)
}
