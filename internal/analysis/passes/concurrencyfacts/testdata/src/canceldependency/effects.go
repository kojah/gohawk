package canceldependency

import "context"

func Stop(cancel context.CancelFunc) { cancel() }
func Wait(ctx context.Context)       { <-ctx.Done() }
func Observe(ctx context.Context)    { ctx.Done() }
func Launch(ctx context.Context)     { go Wait(ctx) }
