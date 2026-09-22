package brokerconsumer

import (
	"brokerdependency"
	"brokerforward"
	"sync"
)

func Close(resource *brokerdependency.Resource) { brokerforward.Close(resource) }
func Lock(mu *sync.Mutex)                       { brokerforward.Lock(mu) }
func Result() error                             { return brokerforward.Result() }
