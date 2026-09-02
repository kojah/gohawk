#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${GH_TOKEN:-}" ]]; then
  echo "GH_TOKEN must read the private repositories that provide installed skills" >&2
  exit 1
fi

root=$(git rev-parse --show-toplevel)
skills_root="$root/.agents/skills"

metadata_value() {
  local key=$1
  local file=$2

  awk -v key="$key" '$1 == key ":" {
    sub(/^[^:]+:[[:space:]]*/, "")
    print
    exit
  }' "$file"
}

checked=0
failed=0

while IFS= read -r -d '' skill_file; do
  relative_file=${skill_file#"$root/"}
  skill_name=$(basename "$(dirname "$skill_file")")

  # Project-local skills are authored in this repository and have no upstream
  # to be current against. They opt out with `source: project` in their
  # metadata; everything else must be a vault-managed skill with pinned source.
  if [[ "$(metadata_value source "$skill_file")" == "project" ]]; then
    echo "$skill_name is project-local; not a managed skill"
    continue
  fi

  source_repo=$(metadata_value github-repo "$skill_file")
  source_path=$(metadata_value github-path "$skill_file")
  installed_tree=$(metadata_value github-tree-sha "$skill_file")

  if [[ -z "$source_repo" || -z "$source_path" || -z "$installed_tree" ]]; then
    echo "::error file=$relative_file::$skill_name lacks gh skill source metadata"
    failed=1
    continue
  fi

  repo=${source_repo#https://github.com/}
  repo=${repo%.git}
  owner=${repo%%/*}
  name=${repo#*/}
  remote_tree=$(gh api graphql \
    -f owner="$owner" \
    -f name="$name" \
    -f expression="HEAD:$source_path" \
    -f query='query($owner: String!, $name: String!, $expression: String!) {
      repository(owner: $owner, name: $name) {
        object(expression: $expression) { ... on Tree { oid } }
      }
    }' \
    --jq '.data.repository.object.oid')
  checked=$((checked + 1))

  if [[ -z "$remote_tree" ]]; then
    echo "::error file=$relative_file::cannot resolve $repo/$source_path on its default branch"
    failed=1
    continue
  fi

  if [[ "$installed_tree" != "$remote_tree" ]]; then
    echo "::error file=$relative_file::$skill_name is out of date with $repo/$source_path"
    printf 'installed tree: %s\nupstream tree:  %s\n' "$installed_tree" "$remote_tree"
    failed=1
    continue
  fi

  echo "$skill_name is current ($installed_tree)"
done < <(find "$skills_root" -mindepth 2 -maxdepth 2 -name SKILL.md -print0 | sort -z)

if (( checked == 0 )); then
  echo "no managed project skills found under $skills_root" >&2
  exit 1
fi

exit "$failed"
