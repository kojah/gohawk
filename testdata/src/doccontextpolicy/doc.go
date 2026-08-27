package doccontextpolicy

import "context"

//gohawk:example flagged
func LoadUser(id string, ctx context.Context) error { // want "context.Context must be first parameter"
	return nil
}

//gohawk:example end

//gohawk:example ok
func LoadUserCorrectly(ctx context.Context, id string) error { return nil }

//gohawk:example end
