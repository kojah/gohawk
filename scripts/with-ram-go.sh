#!/usr/bin/env bash

# Keep disposable Go build and analysis writes off the shared disk. The caller
# owns a reusable workspace on tmpfs; results can be copied out after the job.
set -euo pipefail

usage() {
	printf '%s\n' \
		'Usage: scripts/with-ram-go.sh WORKSPACE COMMAND [ARG ...]' \
		'' \
		'WORKSPACE must be on tmpfs, such as /dev/shm/gohawk-perf.' \
		'The Go cache is retained there between calls; one command runs at a time.' \
		'GOHAWK_GO_PROCS sets Go parallelism (default: 2). Verification jobs are 1.' \
		'Temporary files and caches stay in RAM; explicit output paths are preserved.'
}

fail() {
	printf 'with-ram-go: %s\n' "$*" >&2
	exit 2
}

if [[ ${1:-} == --help || ${1:-} == -h ]]; then
	usage
	exit 0
fi
(($# >= 2)) || { usage >&2; exit 2; }

workspace=$1
shift
parallelism=${GOHAWK_GO_PROCS:-2}
[[ $parallelism =~ ^[1-9][0-9]*$ ]] || fail 'GOHAWK_GO_PROCS must be a positive integer'
command -v stat >/dev/null || fail 'stat is required'
command -v flock >/dev/null || fail 'flock is required'
command -v nice >/dev/null || fail 'nice is required'

umask 077
mkdir -p -- "$workspace"
[[ $(stat -f -c %T -- "$workspace") == tmpfs ]] || fail 'workspace must be on tmpfs'
workspace=$(cd "$workspace" && pwd -P)
exec 9>"$workspace/.lock"
flock -n 9 || fail 'another command is using this workspace'
mkdir -p -- "$workspace/cache" "$workspace/tmp"

export GOCACHE="$workspace/cache"
export GOTMPDIR="$workspace/tmp"
export TMPDIR="$workspace/tmp"
export GOMAXPROCS="$parallelism"
export GOFLAGS="${GOFLAGS:-} -p=$parallelism"
export VERIFY_JOBS=1

if command -v ionice >/dev/null; then
	exec ionice -c 3 nice -n 10 "$@"
fi
exec nice -n 10 "$@"
