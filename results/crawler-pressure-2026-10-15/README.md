# Crawler database pressure calibration

The larger client matrix demonstrated sustained native database CPU pressure in the first short 96-products/s stage. It does not yet prove a sustainable database capacity or a Weir throughput/CPU benefit. The prospectively specified native-only rule selects **512 total workers and formal rates 45, 57 and 68 products/s**; see [the frozen selection](SELECTION.md). No formal results are included in this revision.

The 512-worker workflow passed its full original Go audit. The 2048-worker workflow failed, including timed deadline/unknown outcomes and two warmup failures; every original stage, error, receipt and owned cleanup is retained. Its failed audit remains failed.

Both runs use frozen client `488b4575932265030b81312319b707e45649a028`, baseline Weir `0f6fac3edb462e088decff98b67560dae2c23c9a`, and metadata-only candidate `745501319ae59a16351294c2eaed58019288c3ad`. They preserve single-record native updates/SDK calls, the two-step crawler dependency chain, four OS clients, query-visible durable completion and all resource limits in [PLAN.md](PLAN.md). Sink and business repository changes are excluded.

## 512 workers

[Original workflow](https://github.com/batchstream/weir-tests/actions/runs/38736840274), runner exit `0`, original whole-report Go audit exit `0`. All nine stage positions and immutable provenance checks passed; the 2048-worker overall business correctness audit did not pass.

| Offered/s | Path | Completed/offered | Rejected | Failed / unknown | Completed/s with drain | DB CPU % | Sustained CPU |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | --- |
| 96 | Native | 480/480 | 0 | 0 / 0 | 57.70 | 98.55 | yes |
| 96 | Weir baseline | 480/480 | 0 | 0 / 0 | 67.26 | 96.92 | yes |
| 96 | Weir metadata | 480/480 | 0 | 0 / 0 | 60.16 | 82.14 | no |
| 384 | Weir baseline | 1156/1920 | 764 | 0 / 0 | 97.93 | 82.12 | no |
| 384 | Weir metadata | 1248/1920 | 672 | 0 / 0 | 96.01 | 81.95 | no |
| 384 | Native | 1521/1920 | 399 | 0 / 0 | 149.75 | 64.93 | no |
| 768 | Weir metadata | 1480/3840 | 2360 | 0 / 0 | 117.77 | 62.96 | no |
| 768 | Native | 1536/3840 | 2304 | 0 / 0 | 163.55 | 46.31 | no |
| 768 | Weir baseline | 1485/3840 | 2355 | 0 / 0 | 125.45 | 62.72 | no |

![Timed calibration observations, 512 workers](plots/calibration512-two-step.png)

## 2048 workers

[Original workflow](https://github.com/batchstream/weir-tests/actions/runs/38736847759), runner exit `1`, original whole-report Go audit exit `1`. All nine stage positions and immutable provenance checks passed; the 2048-worker overall business correctness audit did not pass.

| Offered/s | Path | Completed/offered | Rejected | Failed / unknown | Completed/s with drain | DB CPU % | Sustained CPU |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | --- |
| 96 | Native | 480/480 | 0 | 0 / 0 | 53.55 | 93.63 | yes |
| 96 | Weir baseline | 480/480 | 0 | 0 / 0 | 58.10 | 90.72 | yes |
| 96 | Weir metadata | 480/480 | 0 | 0 / 0 | 67.20 | 73.31 | no |
| 384 | Weir baseline | 1060/1920 | 0 | 860 / 694 | 70.35 | 71.10 | no |
| 384 | Weir metadata | 1316/1920 | 0 | 604 / 474 | 89.80 | 58.61 | no |
| 384 | Native | 1920/1920 | 0 | 0 / 0 | 198.95 | 77.92 | no |
| 768 | Weir metadata | warmup failed | 0 | 0 / 0 | not timed | unavailable | no |
| 768 | Native | 1320/3840 | 0 | 2520 / 2520 | 86.82 | 88.37 | no |
| 768 | Weir baseline | warmup failed | 0 | 0 / 0 | not timed | unavailable | no |

![Timed calibration observations, 2048 workers](plots/calibration2048-two-step.png)

All timed failures in the 2048-worker table were also timeouts. At 768 products/s, the candidate additionally had 18 failed/timed-out warmup products (18 unknown), and baseline had 35 (33 unknown); neither stage reached timing. Timed field-oracle failures on partial/unknown business outcomes mean those stages are unqualified; they are not silently treated as successful business updates or proof of data corruption. Detailed latency, drain, generator lag, field counts, call ledgers, CPU coverage and every negative row are in [stages.csv](stages.csv) and [analysis.json](analysis.json).

The sustained CPU rule is unchanged: at least five valid intervals, >=80% sampling coverage, >=90% mean CPU, and >=90% CPU over >=80% of the whole measurement including drain. For the selected native stage, mean CPU was 98.548%, coverage 99.926%, and time above the threshold 87.394%; all 480 products completed without failure or rejection and generator lag was 2.314 ms. Its p99 upper bound of 5.243 seconds and 3.319-second drain exceed the later formal all-arrivals limits. It is a diagnostic selection point, not a sustainable-rate claim.

The first short native stage is affected by the limited 2-second warmup and 5-second arrival period. Later 512-worker native stages show 64.93% and 46.31% CPU despite higher offered rates. Cold execution, JIT, database history and order effects are possible explanations, not isolated causes. The formal matrix must re-establish actual pressure in every round after 10-second warmup. Do not merge absolute CPU or throughput from different runners into a capacity curve. The larger worker level also enlarges the seeded working set (3073 versus 12289 records), so the two levels do not isolate concurrency alone.

## Reproduce the evidence

The two archives contain **32 files** with compressed SHA-256, uncompressed SHA-256 and byte length recorded in each `raw/*/files.json`. They preserve 18 original stages, of which 16 reached timing. Source, command, server binary, GitHub workflow identity, owned database logs and cleanup are checked, with no backend HTTP counts invented while the observer is disabled. The source-specific full Go auditor passes all nine 512-worker stages and fails the original 2048-worker report. No partial-stage helper or modified report is used.

Build `cmd/weir-crawler` from a clean checkout/archive of client commit `488b4575932265030b81312319b707e45649a028` using `go build -trimpath -buildvcs=false`, then supply that executable:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 results/crawler-pressure-2026-10-15/analyze.py \
  --manifest results/crawler-pressure-2026-10-15/manifest.json \
  --auditor /absolute/path/to/rebuilt-weir-crawler \
  --output /tmp/weir-pressure-rebuilt
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover \
  -s results/crawler-pressure-2026-10-15 -p test_analysis.py
```

[auditor-build.json](auditor-build.json) preserves the historical clean source/archive hash, Darwin binary hash and build metadata. A rebuilt Linux or differently linked binary need not share those bytes. The analyzer checks the declared historical source and records the **actual supplied auditor hash**, historical hash and `auditor_matches_historical_binary` flag; it does not reject a valid source rebuild solely for a different executable hash. The caller must rebuild from the exact source above; a different hash by itself does not establish its source. Analysis-source hashes are retained in output. The portability validator correction changes no numeric calculation, workload, pressure threshold or selection rule.

The independent frozen analysis and plotting sources are in this folder. Historical batching and metadata-only result folders remain unchanged. The analysis performs no remote requests or database writes.
