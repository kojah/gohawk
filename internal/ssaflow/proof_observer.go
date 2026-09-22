package ssaflow

import "go/token"

// An Observer hears why a shared proof stopped short: the reason code, the
// position it stopped at, and bounded details such as the instruction that
// blocked it. It reports where evidence ran out, never what an analyzer
// decided, so it changes no answer. ssaflow defines the type over primitives
// so the tracer can satisfy it by method value without either package
// importing the other; a budget carries it to every query that spends that
// budget, which is exactly the scope of one candidate's proof.
type Observer func(reason string, at token.Pos, details map[string]string)
