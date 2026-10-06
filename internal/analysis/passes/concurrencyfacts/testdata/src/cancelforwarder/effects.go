package cancelforwarder

import (
	"canceldependency"
	"context"
)

func Stop(cancel context.CancelFunc) { canceldependency.Stop(cancel) }
func Launch(ctx context.Context)     { canceldependency.Launch(ctx) }
