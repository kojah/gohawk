package is

import "testing"

type I struct{ relaxed bool }

func New(*testing.T) *I        { return &I{} }
func NewRelaxed(*testing.T) *I { return &I{relaxed: true} }
func (*I) Fail()               {}
func (*I) NoErr(error)         {}
