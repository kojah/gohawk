#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
	echo "usage: $0 GOHAWK_BINARY GO_MINOR_VERSION" >&2
	exit 2
fi

gohawk_binary=$1
go_minor_version=$2
repository_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
fixture_directory="$repository_root/internal/cli/testdata/toolchains/go$go_minor_version"

if [[ ! -x "$gohawk_binary" ]]; then
	echo "gohawk binary is not executable: $gohawk_binary" >&2
	exit 2
fi
if [[ ! -d "$fixture_directory" ]]; then
	echo "unsupported Go compatibility fixture: $go_minor_version" >&2
	exit 2
fi

actual_version=$(GOTOOLCHAIN=local go env GOVERSION)
case "$actual_version" in
	"go$go_minor_version" | "go$go_minor_version".*) ;;
	*)
		echo "Go $go_minor_version compatibility test is running with $actual_version" >&2
		exit 2
		;;
esac

cd "$fixture_directory"
GOTOOLCHAIN=local "$gohawk_binary" ./...
