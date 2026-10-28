package cli

import "strings"

// Workspace guidance follows go's refusal of a directory pattern that lies
// outside every module of a go.work workspace, such as ./... at the workspace
// root, with the pattern that names the whole workspace.

// goWorkRefusal is the fragment of go's error for a directory pattern that
// resolves outside every module listed in go.work. The rest of the line names
// the pattern and the directory, which vary.
const goWorkRefusal = "does not contain modules listed in go.work"

const workspaceHintLine = `gohawk: run "gohawk work" to analyze every module in the go.work workspace, or run gohawk inside a module directory`

// workspaceHint returns the line to print after go's own error when go refused
// a directory pattern in workspace mode, and "" for every other failure.
func workspaceHint(goStderr []byte) string {
	if !strings.Contains(string(goStderr), goWorkRefusal) {
		return ""
	}
	return workspaceHintLine
}
