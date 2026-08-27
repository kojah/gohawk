package syncmapatomicity

import "sync"

//gohawk:example flagged
func take(cache *sync.Map, key string) any {
	value, ok := cache.Load(key)
	if ok {
		cache.Delete(key) // want "sync.Map Load and Delete do not atomically claim the value"
		return value
	}
	return nil
}

//gohawk:example end

//gohawk:example ok
func takeAtomically(cache *sync.Map, key string) any {
	value, deleted := cache.LoadAndDelete(key)
	if deleted {
		return value
	}
	return nil
}

//gohawk:example end
