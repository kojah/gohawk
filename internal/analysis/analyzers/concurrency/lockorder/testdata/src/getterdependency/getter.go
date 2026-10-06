package getterdependency

import "sync"

type Owner struct{ mu sync.Mutex }

func (owner *Owner) Mu() *sync.Mutex { return &owner.mu }
