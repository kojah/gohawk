# Usage: jq -s --arg root '/absolute/repository/package/scope/' \
#   -f scripts/heap-loss-summary.jq trace.jsonl
# Only exported-function lifecycle summaries are counted. Dependencies outside
# root are excluded. Repeated identical test/build observations count once;
# conflicting snapshots require investigation rather than arbitrary selection.
def total(key): map((.details[key] // "0") | tonumber) | add // 0;
def histogram: group_by(.) | map({key: .[0], value: length}) | from_entries;
def call_counts:
  [.[].details | to_entries[] | select(.key | startswith("calls-"))
    | select(.value | test("^[0-9]+$"))]
  | group_by(.key)
  | map({key: .[0].key, value: (map(.value | tonumber) | add)})
  | from_entries;

[.[] | select(.analyzer == "lifecyclefacts" and .phase == "decision"
  and .reason == "function-summarized" and (.candidate | startswith($root)))] as $events
| ($events | unique_by([.candidate, .function, .details])) as $snapshots
| ($events | unique_by([.candidate, .function])) as $functions
| if ($snapshots | length) != ($functions | length) then
    error("conflicting observations for one function; inspect snapshots before aggregating")
  else $functions end
| {
    root: $root,
    events: ($events | length),
    functions: length,
    test_source_functions: (map(select(.candidate | test("_test\\.go:"))) | length),
    cache_status: (map(if .details["heap-cached"] != "true" then "not-cached"
      elif .details["heap-building"] == "true" then "building" else "cached" end) | histogram),
    build_reasons: (map(.details["heap-build-reason"]) | histogram),
    truncated_summaries: (map(select((.details["heap-truncated-count"] // "0" | tonumber) > 0)) | length),
    truncated_slots: total("heap-truncated-count"),
    widening_sites: total("heap-widening-sites"),
    escape_origins: total("heap-escape-origins"),
    call_counts: call_counts
  }
