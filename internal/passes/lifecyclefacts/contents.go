package lifecyclefacts

import (
	"slices"
	"strings"
)

// A caller that hands an aggregate to a callee it cannot read needs to know
// whether a resource inside that aggregate can outlive the call. Retained
// answers that for the parameter itself and deliberately not for what is
// loaded out of it, so the Kept claims record the loose complement: the
// access paths beneath a struct-shaped parameter whose contents the callee
// may keep. The claim is a query over the heap projection, beside the
// other transfer claims; this file holds only its vocabulary. Nothing here
// is an ownership transfer; the strict Stored bit is untouched.

// Kept is one loose content-retention claim: the value at Path beneath
// Parameter, or something loaded out of it, may be kept beyond the call. The
// empty path is the parameter itself or a whole copy of it, which keeps
// every path beneath it.
type Kept struct {
	Parameter int
	Path      string
}

// keepsContentsAt reports whether one of the kept paths reaches the value
// at path: the claim is the path itself, a prefix that keeps the aggregate
// around it, or a longer path inside it, since keeping part of a resource
// keeps the resource. The empty path on either side reaches everything.
func keepsContentsAt(kept []string, path string) bool {
	return slices.ContainsFunc(kept, func(claim string) bool {
		return claim == "" || path == "" || claim == path ||
			strings.HasPrefix(path, claim+"/") || strings.HasPrefix(claim, path+"/")
	})
}
