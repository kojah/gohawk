package lockorder

import "sync"

var cacheGate, incompleteGate, escapedGate, wrongGate, otherGate, modifiedGate sync.Mutex

func inspectCache(miss bool) (bool, int) {
	cacheGate.Lock()
	if !miss {
		cacheGate.Unlock()
		return true, 1
	}
	return false, 0
}

func useCache(miss bool) {
	found, _ := inspectCache(miss)
	if found {
		return
	}
	cacheGate.Unlock()
}

func inspectIncompleteCache(miss bool) (bool, int) {
	incompleteGate.Lock()
	if !miss {
		incompleteGate.Unlock()
		return true, 1
	}
	return false, 0 // want "lock .* is not released on this return path"
}

func completeCacheCaller(miss bool) {
	found, _ := inspectIncompleteCache(miss)
	if found {
		return
	}
	incompleteGate.Unlock()
}

func incompleteCacheCaller(miss bool) { inspectIncompleteCache(miss) }

func inspectEscapedCache(miss bool) (bool, int) {
	escapedGate.Lock()
	if !miss {
		escapedGate.Unlock()
		return true, 1
	}
	return false, 0 // want "lock .* is not released on this return path"
}

var exportedCacheCallback = inspectEscapedCache

func escapedCacheCaller(miss bool) {
	found, _ := inspectEscapedCache(miss)
	if found {
		return
	}
	escapedGate.Unlock()
}

func inspectWrongCache(miss bool) (bool, int) {
	wrongGate.Lock()
	if !miss {
		wrongGate.Unlock()
		return true, 1
	}
	return false, 0 // want "lock .* is not released on this return path"
}

func wrongCacheCaller(miss bool) {
	found, _ := inspectWrongCache(miss)
	if found {
		return
	}
	otherGate.Unlock()
}

func inspectModifiedCache(miss bool) (bool, int) {
	modifiedGate.Lock()
	if !miss {
		modifiedGate.Unlock()
		return true, 1
	}
	return false, 0 // want "lock .* is not released on this return path"
}

func modifiedCacheCaller(miss, skip bool) {
	found, _ := inspectModifiedCache(miss)
	if found || skip {
		return
	}
	modifiedGate.Unlock()
}
