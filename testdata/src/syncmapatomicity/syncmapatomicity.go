package syncmapatomicity

import "sync"

func use(any) {}

func nonAtomicClaim(cache *sync.Map, key string) {
	value, ok := cache.Load(key)
	if ok {
		cache.Delete(key) // want "sync.Map Load and Delete do not atomically claim the value"
		use(value)
	}
}

func nonAtomicInlineClaim(cache *sync.Map, key string) {
	if value, ok := cache.Load(key); ok {
		cache.Delete(key) // want "sync.Map Load and Delete do not atomically claim the value"
		use(value)
	}
}

func nonAtomicParenthesizedClaim(cache *sync.Map, key string) {
	value, ok := cache.Load(key)
	if ok {
		cache.Delete(key) // want "sync.Map Load and Delete do not atomically claim the value"
		use(value)
	}
}

func nonAtomicExplicitTrueClaim(cache *sync.Map, key string) {
	value, ok := cache.Load(key)
	if ok == true {
		cache.Delete(key) // want "sync.Map Load and Delete do not atomically claim the value"
		use(value)
	}
}

func nonAtomicReversedTrueClaim(cache *sync.Map, key string) {
	value, ok := cache.Load(key)
	if true == ok {
		cache.Delete(key) // want "sync.Map Load and Delete do not atomically claim the value"
		use(value)
	}
}

func atomicClaim(cache *sync.Map, key string) {
	if value, deleted := cache.LoadAndDelete(key); deleted {
		use(value)
	}
}

func externallyLocked(cache *sync.Map, mutex *sync.Mutex, key string) {
	mutex.Lock()
	defer mutex.Unlock()
	value, ok := cache.Load(key)
	if ok {
		cache.Delete(key)
		use(value)
	}
}

func observeThenDelete(cache *sync.Map, key string) {
	_, ok := cache.Load(key)
	if ok {
		cache.Delete(key)
	}
}

func deleteDifferentKey(cache *sync.Map, key, other string) {
	value, ok := cache.Load(key)
	if ok {
		cache.Delete(other)
		use(value)
	}
}

func deleteWhenLoadMisses(cache *sync.Map, key string) {
	value, ok := cache.Load(key)
	if !ok {
		cache.Delete(key)
		use(value)
	}
}

func deleteUnlessExplicitlyPresent(cache *sync.Map, key string) {
	value, ok := cache.Load(key)
	if ok != true {
		cache.Delete(key)
		use(value)
	}
}

func shadowedTrueIsNotProof(cache *sync.Map, key string, true bool) {
	value, ok := cache.Load(key)
	if ok == true {
		cache.Delete(key)
		use(value)
	}
}
