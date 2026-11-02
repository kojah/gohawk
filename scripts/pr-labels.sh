#!/usr/bin/env bash
# Apply the pull request labels that follow from facts alone: changed paths
# and the template's Intent and AI assistance sections. The pr-classify
# agentic workflow labels the remaining dimensions; the review skill's
# section 3.6 defines them all.
#
# Usage: scripts/pr-labels.sh NUMBER   (needs GH_TOKEN and GH_REPO)
#
# area: labels and the category: label are only added, never removed, and the
# category is set only when the pull request has none, so a maintainer's
# label wins. needs:intent and needs:ai-disclosure track the description and
# are added or removed on every run.
set -euo pipefail

number=${1:?usage: pr-labels.sh NUMBER}
repo=${GH_REPO:?GH_REPO must name owner/repo}

pr=$(gh api "repos/$repo/pulls/$number")
body=$(jq -r '.body // ""' <<<"$pr" | tr -d '\r')
labels=$(jq -r '.labels[].name' <<<"$pr")
files=$(gh api --paginate "repos/$repo/pulls/$number/files" -q '.[].filename')

has_label() { grep -qxF "$1" <<<"$labels"; }
has_prefix() { grep -q "^$1" <<<"$labels"; }

add=()
remove=()

# area: from changed paths.
docs_only=true
tooling_or_docs=true
while IFS= read -r file; do
	[[ -n $file ]] || continue
	case $file in
	internal/analysis/analyzers/concurrency/*) add+=(area:concurrency) ;;
	internal/analysis/analyzers/resources/*) add+=(area:resources) ;;
	internal/engine/*) add+=(area:engine) ;;
	analyzers/* | docs/analyzers/*) add+=(area:catalog) ;;
	esac
	case $file in
	docs/* | site/* | talks/* | .agents/* | *.md | LICENSE) ;;
	.github/* | scripts/* | tools/* | benchmarks/* | .greptile/* | .claude/* | \
		Makefile | go.mod | go.sum | hyrule.yaml | boiler.yaml | .golangci.yml | \
		.go-version | .gitignore | .gitattributes | .pre-commit-hooks.yaml | \
		.deadcode-baseline | .hyrule-manifest.json)
		docs_only=false
		;;
	*)
		docs_only=false
		tooling_or_docs=false
		;;
	esac
done <<<"$files"

# section TITLE prints a template section's text without HTML comments.
section() {
	awk -v title="## $1" '
		$0 == title { inside = 1; next }
		/^## / { inside = 0 }
		inside { print }
	' <<<"$body" | perl -0pe 's/<!--.*?-->//gs'
}

# Intent: the boxes the author ticked, mapped to category labels.
intent=$(section Intent | grep -iE '^\s*[-*] \[x\]' || true)
declare -A intents=(
	[precision]=category:improves-precision
	[recall]=category:improves-recall
	[new check]=category:new-check
	[reliability]=category:reliability
	[performance]=category:performance
	[refactor]=category:refactor
	[documentation]=category:docs
	[tooling]=category:tooling
)
declared=()
for key in "${!intents[@]}"; do
	if grep -qiE "^\s*[-*] \[x\] \*\*$key\*\*" <<<"$intent"; then
		declared+=("${intents[$key]}")
	fi
done

if ((${#declared[@]} == 0)); then
	add+=(needs:intent)
else
	remove+=(needs:intent)
fi

if ! has_prefix category:; then
	if ((${#declared[@]} == 1)); then
		add+=("${declared[0]}")
	elif ((${#declared[@]} == 0)) && [[ -n $files ]]; then
		if $docs_only; then
			add+=(category:docs)
		elif $tooling_or_docs; then
			add+=(category:tooling)
		fi
	fi
fi

# AI assistance: any text left once comments are stripped counts, including
# "None"; an untouched template does not.
if [[ -n $(section 'AI assistance' | tr -d '[:space:]') ]]; then
	remove+=(needs:ai-disclosure)
else
	add+=(needs:ai-disclosure)
fi

for label in $(printf '%s\n' "${add[@]}" | sort -u); do
	has_label "$label" || gh pr edit "$number" --repo "$repo" --add-label "$label" >/dev/null
	echo "label: $label"
done
for label in "${remove[@]}"; do
	if has_label "$label"; then
		gh pr edit "$number" --repo "$repo" --remove-label "$label" >/dev/null
		echo "unlabel: $label"
	fi
done
