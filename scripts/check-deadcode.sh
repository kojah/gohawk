#!/usr/bin/env bash
# Fail on internal functions that nothing in the module can reach.
#
# golangci-lint's `unused` check skips every exported identifier, so an
# exported helper under internal/ that lost its last caller is never reported
# even though no code outside the module can import it. This gate runs
# golang.org/x/tools/cmd/deadcode, which computes reachability from the
# module's entry points instead of from syntax.
#
# Roots are every main package plus the golangci-lint plugin's test binary,
# because the plugin exposes its entry point through module registration
# rather than a main function. Packages outside internal/ are the library and
# plugin API surface, so their exported functions are legitimately reachable
# from other modules and are not judged here.
#
# .deadcode-baseline lists findings that predate the gate. A finding outside
# the baseline fails, and so does a baseline entry the tool no longer reports,
# so the baseline can only shrink.
set -euo pipefail

root=$(git rev-parse --show-toplevel)
cd "$root"

baseline="$root/.deadcode-baseline"
DEADCODE=${DEADCODE:-go run golang.org/x/tools/cmd/deadcode@v0.49.0}

# deadcode prints one `file:line:col: unreachable func: Name` line per
# finding. Line numbers are dropped so unrelated edits do not churn the
# baseline.
findings=$(
  $DEADCODE -test . ./tools/gendocs ./tools/measure ./plugin/golangci |
    awk -F: '/unreachable func: / {
      name = $NF
      sub(/^[[:space:]]*unreachable func: /, "", name)
      sub(/^[[:space:]]+/, "", name)
      print $1 " " name
    }' |
    grep '^internal/' |
    sort -u || true
)

known=$(grep -v '^[[:space:]]*#' "$baseline" | grep -v '^[[:space:]]*$' | sort -u || true)

new=$(comm -23 <(printf '%s\n' "$findings") <(printf '%s\n' "$known") | grep -v '^$' || true)
stale=$(comm -13 <(printf '%s\n' "$findings") <(printf '%s\n' "$known") | grep -v '^$' || true)

status=0
if [[ -n "$new" ]]; then
  echo "unreachable internal functions (delete them, or move test-only helpers into _test.go files):" >&2
  printf '%s\n' "$new" | sed 's/^/  /' >&2
  status=1
fi
if [[ -n "$stale" ]]; then
  echo "baseline entries that are no longer reported; remove them from .deadcode-baseline:" >&2
  printf '%s\n' "$stale" | sed 's/^/  /' >&2
  status=1
fi
exit "$status"
