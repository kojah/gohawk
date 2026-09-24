package ssainfer

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// A completion proof says that a callee settles the target; a caller that
// exports the proof as a claim about the target's contents also needs to know
// where. The coverage rule accepts a call on anything derived from a mapped
// local, so a deferred literal closing j.out proves completion of j, and the
// exported claim would say j was closed. This file records, for one body's
// coverage, the static path beneath the target that each completing call was
// made on, and keeps the path only while every completing call agrees. The
// proof's Path is then exact or absent, never a guess: a receiver with no
// static path, two calls on different paths, a summary, and an invoked
// callback all make it unknown, and a caller that needs the path treats
// unknown as no claim.

// completionPaths folds the paths of the completing calls seen during one
// body's coverage.
type completionPaths struct {
	path  string
	seen  bool
	exact bool
}

// record folds one completing call made on path beneath the target; known
// is false when the call's path could not be named.
func (paths *completionPaths) record(path string, known bool) {
	if paths == nil {
		return
	}
	switch {
	case !known:
		paths.exact = false
	case !paths.seen:
		paths.path, paths.exact = path, true
	case paths.path != path:
		paths.exact = false
	}
	paths.seen = true
}

// merge folds the paths of another body's coverage, as when every callee of
// a dynamic call must complete the target.
func (paths *completionPaths) merge(other completionPaths) {
	if !other.seen {
		return
	}
	paths.record(other.path, other.exact)
}

// known reports whether every completing call was on one static path.
func (paths *completionPaths) known() bool {
	return paths.seen && paths.exact
}

// receiverPath names the path beneath the target of a completing call's
// receiver, which receives established stands for the target through local.
func (search *completionSearch) receiverPath(local mappedLocal, receiver, target ssa.Value) (string, bool) {
	if search.exactTarget {
		return search.mappedPath(local, target, nil, true)
	}
	actual, ok := heapmodel.AccessPathFromParameter(receiver, local.local)
	return search.mappedPath(local, target, actual, ok)
}

// mappedPath translates a path beneath the local, on which a completing
// call was made, onto the target the local stands for. The local's own
// relation to the target decides the translation, and any relation the
// search cannot express as a static path leaves the result unknown.
func (search *completionSearch) mappedPath(local mappedLocal, target ssa.Value, actual []string, known bool) (string, bool) {
	if !known {
		return "", false
	}
	switch local.kind {
	case localExact:
		if len(local.path) > 0 {
			// The local holds the target at local.path; a call there is on
			// the target itself, and a call on the local or elsewhere is
			// on the aggregate around it.
			return "", ssaflow.JoinAccessPath(actual) == ssaflow.JoinAccessPath(local.path)
		}
		supplied, ok := heapmodel.AccessPathOf(local.supplied, target)
		if !ok {
			return "", false
		}
		return ssaflow.JoinAccessPath(append(append([]string(nil), supplied...), actual...)), true
	case localProjection:
		// The local is a proper projection of the target, and a
		// completion is a call on the local itself.
		if len(actual) > 0 {
			return "", false
		}
		supplied, ok := heapmodel.AccessPathOf(local.supplied, target)
		return ssaflow.JoinAccessPath(supplied), ok
	case localOwner:
		// The call was on the path beneath the local that mirrors the
		// target's path beneath the supplied owner: the target itself.
		return "", true
	case localCallback:
	}
	return "", false
}
