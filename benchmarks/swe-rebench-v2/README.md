# SWE-rebench V2 Go repair mining

This benchmark mines the executable
[SWE-rebench V2](https://huggingface.co/datasets/nebius/SWE-rebench-V2)
dataset for recurring Go repair patterns that may justify precise gohawk
diagnostics. It explicitly records overlap with gohawk, Go's analyzers,
Staticcheck, and focused golangci-lint analyzers before recommending new work.

Download the converted Parquet file outside the repository and regenerate the
report:

```sh
curl -fL -o /tmp/SWE-rebench-V2.parquet \
  'https://huggingface.co/datasets/nebius/SWE-rebench-V2/resolve/refs%2Fconvert%2Fparquet/default/train/0000.parquet'
printf '%s  %s\n' \
  '0e0bf9355f892ad74ae98d4e1c404f39fd6654a8e351ee3e6ab162e4a64cd3ad' \
  /tmp/SWE-rebench-V2.parquet | sha256sum --check
uv run --with pyarrow scripts/analyze_swe_rebench_go.py \
  --input /tmp/SWE-rebench-V2.parquet \
  --output benchmarks/swe-rebench-v2/report.md
```

The downloaded dataset and repository checkouts are intentionally not
committed. The report contains aggregate counts and a small number of public
instance identifiers, so it remains reviewable and inexpensive to refresh.

Run the script's focused tests with:

```sh
python3 -m unittest scripts/analyze_swe_rebench_go_test.py
```
