#!/usr/bin/env bash

set -euo pipefail

usage() {
	cat <<'EOF'
Usage: scripts/benchmark-dogfood.sh [options]

Measure whole-repository gohawk runtime and peak resident memory.

Options:
  --manifest PATH       Repository manifest (default: benchmarks/repositories.tsv)
  --only NAME           Measure only the named manifest entry (repeatable)
  --runs COUNT          Measured runs per repository (default: 3)
  --output DIRECTORY    New result directory (default: benchmark-results/<UTC time>)
  --gohawk-arg=ARG      Pass an analyzer-selection argument to gohawk (repeatable)
  --keep-workdir        Keep temporary repository checkouts
  -h, --help            Show this help
EOF
}

fail() {
	printf 'benchmark-dogfood: %s\n' "$*" >&2
	exit 1
}

require_command() {
	command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

selected_repository() {
	local candidate=$1
	local selected

	if ((${#only_repositories[@]} == 0)); then
		return 0
	fi
	for selected in "${only_repositories[@]}"; do
		if [[ $candidate == "$selected" ]]; then
			return 0
		fi
	done
	return 1
}

run_gohawk() {
	local checkout=$1
	local package_pattern=$2
	local log_file=$3

	set +e
	(
		cd "$checkout"
		"$gohawk_binary" "${gohawk_arguments[@]}" "$package_pattern"
	) >"$log_file" 2>&1
	command_status=$?
	set -e
}

repository_root=$(git rev-parse --show-toplevel 2>/dev/null) || fail "run from a gohawk checkout"
manifest_path="$repository_root/benchmarks/repositories.tsv"
runs=3
output_directory="$repository_root/benchmark-results/$(date -u +%Y%m%dT%H%M%SZ)"
keep_workdir=false
declare -a only_repositories=()
declare -a gohawk_arguments=()

while (($# > 0)); do
	case $1 in
	--manifest)
		(($# >= 2)) || fail "--manifest requires a path"
		manifest_path=$2
		shift 2
		;;
	--only)
		(($# >= 2)) || fail "--only requires a repository name"
		only_repositories+=("$2")
		shift 2
		;;
	--runs)
		(($# >= 2)) || fail "--runs requires a count"
		runs=$2
		shift 2
		;;
	--output)
		(($# >= 2)) || fail "--output requires a directory"
		output_directory=$2
		shift 2
		;;
	--gohawk-arg=*)
		gohawk_arguments+=("${1#*=}")
		shift
		;;
	--keep-workdir)
		keep_workdir=true
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		fail "unknown option: $1"
		;;
	esac
done

[[ $runs =~ ^[1-9][0-9]*$ ]] || fail "--runs must be a positive integer"
[[ -f $manifest_path ]] || fail "manifest not found: $manifest_path"
[[ ! -e $output_directory ]] || fail "output path already exists: $output_directory"

require_command git
require_command go
require_command awk
require_command wc

mkdir -p "$output_directory/logs"
work_directory=$(mktemp -d "${TMPDIR:-/tmp}/gohawk-benchmark.XXXXXX")

cleanup() {
	if [[ $keep_workdir == true ]]; then
		printf 'Temporary repositories retained at %s\n' "$work_directory"
		return
	fi
	case $work_directory in
	"${TMPDIR:-/tmp}"/gohawk-benchmark.*)
		rm -rf -- "$work_directory"
		;;
	*)
		printf 'Refusing to remove unexpected work directory: %s\n' "$work_directory" >&2
		;;
	esac
}
trap cleanup EXIT

gohawk_binary="$work_directory/gohawk"
measure_binary="$work_directory/measure"
go build -trimpath -o "$gohawk_binary" "$repository_root"
go build -trimpath -o "$measure_binary" "$repository_root/internal/cmd/measure"

gohawk_revision=$(git -C "$repository_root" rev-parse HEAD)
if [[ -n $(git -C "$repository_root" status --porcelain) ]]; then
	gohawk_tree=dirty
	printf 'Warning: the gohawk worktree is dirty; results will not be fully reproducible.\n' >&2
else
	gohawk_tree=clean
fi

go_version=$(go version)
host_description=$(uname -a)
processor_count=$(getconf _NPROCESSORS_ONLN 2>/dev/null || printf 'unknown')
memory_description=$(awk '/^MemTotal:/ { print $2 " " $3 }' /proc/meminfo 2>/dev/null || true)
if [[ -z $memory_description ]]; then
	memory_description=unknown
fi

{
	printf '# Dogfooding benchmark\n\n'
	printf -- '- Generated: `%s`\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	printf -- '- gohawk revision: `%s` (%s worktree)\n' "$gohawk_revision" "$gohawk_tree"
	printf -- '- Go: `%s`\n' "$go_version"
	printf -- '- Host: `%s`\n' "$host_description"
	printf -- '- Logical processors: `%s`\n' "$processor_count"
	printf -- '- Physical memory: `%s`\n' "$memory_description"
	printf -- '- Measured runs per repository: `%s`\n' "$runs"
	if ((${#gohawk_arguments[@]} == 0)); then
		printf -- '- gohawk profile: default\n'
	else
		printf -- '- Additional gohawk arguments:'
		printf ' `%s`' "${gohawk_arguments[@]}"
		printf '\n'
	fi
	printf '\nClone time, dependency downloads, binary construction, package counting, and one warm-up run are excluded. Peak RSS is reported by the repository-owned measurement helper.\n\n'
	printf '| Repository | Revision | Packages | Run | Wall time (s) | Peak RSS (MiB) | Exit | Output |\n'
	printf '|---|---|---:|---:|---:|---:|---:|---|\n'
} >"$output_directory/summary.md"

printf 'repository,revision,packages,run,wall_seconds,peak_rss_kib,exit_code,log\n' >"$output_directory/results.csv"

measured_repositories=0
while IFS=$'\t' read -r repository_name repository_url requested_revision package_pattern extra; do
	[[ -n $repository_name ]] || continue
	[[ $repository_name == \#* ]] && continue
	[[ -z ${extra:-} ]] || fail "manifest row for $repository_name has more than four fields"
	[[ -n ${repository_url:-} && -n ${requested_revision:-} && -n ${package_pattern:-} ]] || \
		fail "manifest row for $repository_name must have four tab-separated fields"
	[[ $repository_name =~ ^[a-zA-Z0-9._-]+$ ]] || \
		fail "manifest repository name contains unsafe characters: $repository_name"
	selected_repository "$repository_name" || continue

	checkout="$work_directory/repositories/$repository_name"
	mkdir -p "$checkout"
	git -C "$checkout" init --quiet
	if [[ $repository_url == . ]]; then
		repository_url=$repository_root
		requested_revision=$(git -C "$repository_root" rev-parse "${requested_revision}^{commit}")
	fi
	git -C "$checkout" remote add origin "$repository_url"
	git -C "$checkout" fetch --quiet --depth=1 origin "$requested_revision"
	git -C "$checkout" checkout --quiet --detach FETCH_HEAD
	resolved_revision=$(git -C "$checkout" rev-parse HEAD)

	package_log="$output_directory/logs/$repository_name-packages.log"
	if ! package_count=$(
		cd "$checkout"
		go list "$package_pattern" 2>"$package_log" | wc -l
	); then
		fail "go list failed for $repository_name; see $package_log"
	fi
	package_count=${package_count//[[:space:]]/}

	warmup_log="$output_directory/logs/$repository_name-warmup.log"
	run_gohawk "$checkout" "$package_pattern" "$warmup_log"
	if [[ $command_status -ne 0 && $command_status -ne 3 ]]; then
		fail "warm-up failed for $repository_name with exit $command_status; see $warmup_log"
	fi

	for ((run = 1; run <= runs; run++)); do
		run_log="$output_directory/logs/$repository_name-run-$run.log"
		timing_file="$work_directory/$repository_name-run-$run.time"
		set +e
		(
			cd "$checkout"
			"$measure_binary" -output "$timing_file" -- \
				"$gohawk_binary" "${gohawk_arguments[@]}" "$package_pattern"
		) >"$run_log" 2>&1
		command_status=$?
		set -e
		if [[ $command_status -ne 0 && $command_status -ne 3 ]]; then
			fail "measurement failed for $repository_name with exit $command_status; see $run_log"
		fi
		IFS=$'\t' read -r wall_seconds peak_rss_kib reported_status <"$timing_file"
		[[ $reported_status == "$command_status" ]] || \
			fail "time reported exit $reported_status for $repository_name, command returned $command_status"
		peak_rss_mib=$(awk -v kib="$peak_rss_kib" 'BEGIN { printf "%.1f", kib / 1024 }')
		log_name="logs/$repository_name-run-$run.log"
		printf '%s,%s,%s,%s,%s,%s,%s,%s\n' \
			"$repository_name" "$resolved_revision" "$package_count" "$run" \
			"$wall_seconds" "$peak_rss_kib" "$command_status" "$log_name" \
			>>"$output_directory/results.csv"
		printf '| %s | `%s` | %s | %s | %s | %s | %s | [%s](%s) |\n' \
			"$repository_name" "${resolved_revision:0:12}" "$package_count" "$run" \
			"$wall_seconds" "$peak_rss_mib" "$command_status" "log" "$log_name" \
			>>"$output_directory/summary.md"
	done

	measured_repositories=$((measured_repositories + 1))
done <"$manifest_path"

if ((measured_repositories == 0)); then
	fail "no manifest entries matched the requested repositories"
fi

printf 'Benchmark results written to %s\n' "$output_directory"
