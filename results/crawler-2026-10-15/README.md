# Weir crawler database-load experiment, 2026-10-15

This experiment tests whether Weir reduces database work at the same completed business throughput, or increases completed throughput at the same database CPU budget. Extra Weir CPU and total infrastructure cost are outside the question. The business repositories are unchanged; their existing worker rules are read-only fixture references. Sink is excluded and no new Lua helper API is introduced.

## Workload and controls

Every path runs the same existing two-step product rules, followed by seller updates: product offer reconciliation, product merge, then seller merge, with query visibility after each of the three phases. The native control uses atomic server-side Painless updates in bulk requests. Weir runs the frozen Lua translation and returns ordinary documents. The generated page contains 20 products, each with two seller documents, existing and incoming offers, descriptions, scalar fields, unions and capped histories. A page succeeds only when every mutation is durably acknowledged and query-visible. Partial pages count as failed business products.

Four independent OS client processes share 32 goroutine workers in the formal cases. Arrivals follow absolute timestamps, independent of completion, with bounded queues and explicit rejected/failed/unknown/timeout accounting. Worker-owned keys model the stated low-conflict workload. The synthetic shared placeholder is selected by `(worker + page) % 100 == 0`; the short matrices do not establish a production frequency. No private records, business configuration or credentials are imported.

All paths within a run share one exclusively owned Elasticsearch container: **1 CPU quota, 512 MiB JVM heap, 1536 MiB container memory**, two indices with one primary each, zero replicas, one-second refresh and request translog durability. Native allows four total conflict attempts, matching Weir's four attempts. Both initial and post-warmup resets settle for at least two quiet seconds. The same database survives the rotating path order, so persistent segments, JIT and background maintenance can still vary. Native round-to-round CPU drift is retained, not averaged away as proof of a benefit.

Fixed-rate formal cases use 10 s warmup, 20 s arrivals and three rotating rounds. Throughput divides complete business products by the arrival interval plus all subsequent drain. Latency starts at the planned arrival and ends at visibility. The independent Go oracle reads every seeded document and checks every field/type; the offline auditor reconstructs the canonical expected hash from acknowledged page ordinals. A failed page may legitimately have written some records before another record fails: an oracle mismatch in such a stage is not evidence of a successful acknowledgement losing data.

The predeclared measurement gate requires matching clean immutable receipts, the complete matrix, successful cleanup and field/offline audits, no failed/unknown/timed-out work, and at least 80% CPU sample coverage. The stricter all-arrivals gate also requires no rejection, all arrivals completed, p99 <=5 s, drain <=3 s and generator lag <=50 ms in all three rounds. These are experiment limits, not production SLOs. Histogram quantiles are inclusive upper bounds with at most 12.5% plus 1 microsecond bucket approximation. Database CPU is integrated from cumulative owned-container counters, clipped to the stage; missing intervals are not extrapolated.

## Frozen experiments

| Run | Purpose | Client SHA | Matrix | Workflow |
| --- | --- | --- | --- | --- |
| Calibration | Diagnose rates/setup, no formal claim | `e391cb59a7c7c80775d4c987f6bd598183a68cc4` | 20/80/160 fixed rate plus 32/128-worker saturation, 2 s warmup/5 s arrivals, one round | [38717858422](https://github.com/batchstream/weir-tests/actions/runs/38717858422) |
| Main | Existing rules, old versus new Weir | `2ef2675d2acb24308d1d7bd2dd798c6826ad7b83` | 20/80/160 fixed rate, 32 workers, three rounds | [38719747415](https://github.com/batchstream/weir-tests/actions/runs/38719747415) |
| Cap control | Same new server, cap 0 versus 16 | `2ef2675d2acb24308d1d7bd2dd798c6826ad7b83` | 80 fixed rate, 32 workers, three rounds | [38720284963](https://github.com/batchstream/weir-tests/actions/runs/38720284963) |
| Direct control | Same cap comparison without measurement proxy | `d799170c06a625a400f1e32d9203b3fb816b463d` | 80 fixed rate, 32 workers, three rounds | [38722843843](https://github.com/batchstream/weir-tests/actions/runs/38722843843) |

The main comparison freezes old server `4fe287e5915766ec3f48c8f42abde6b884d89bdd` and new server `c3101c6628872345d6fb09b977ce853f25936b79`, both with unlimited adapter executions. Both cap controls use exactly `c3101c6628872345d6fb09b977ce853f25936b79` and identical binary hashes; only the optimized path sets `max_in_flight=16`. Live before/after gauges independently attest the cap. Server/client source, binary/build identity, exact command, runner hardware/kernel, image digest, CPU quota and cleanup identities are checked from raw receipts.

The [initial plan](PLAN.md), [formal decision](FORMAL-DECISION.md), [cap decision](CAP-DECISION.md) and [direct-control decision](DIRECT-DECISION.md) retain the sequence and reasons for changes. No deadline, CPU or heap limit was relaxed to obtain a favorable result. Different runners' absolute rates/CPU are never treated as paired observations.

## Retained failures and measurement limitation

The calibration retained all 15 stages. Its oversized 81.5 MiB reset request triggered an Elasticsearch parent memory breaker; saturation warmups also exceeded the unchanged deadline. Initialization was then bounded to 8 MiB chunks with strict per-item acknowledgements, and the formal run used 32 workers. These calibration observations do not qualify as capacity or savings measurements.

The main proxy run retained all 27 stages: five failed before timing; all nine native stages completed every offered product; timed Weir stages reported 280 failed business products. The cap proxy run retained all nine stages. It increased observed adapter batch fill from about 2.9 to 14.3–14.6 records per execution, but cap 16 had failures in every round and exceeded the predeclared p99/drain limits. Larger batches alone did not demonstrate a useful throughput improvement.

Both proxy runs contain response-copy aborts (`use of closed network connection`). The old observer updated counters only after `ReverseProxy.ServeHTTP` returned, so Go's `ErrAbortHandler` panic omitted these requests from its ledger. Consequently, those HTTP counts are not complete failure accounting, and the artifacts cannot identify whether Weir core, the upstream response or the proxy transport initiated the failure. The direct control bypasses the proxy for all three paths; its HTTP request/byte metrics are deliberately unavailable. Database CPU, native index counters and correctness/visibility checks remain measured. The observer fix records aborts in a defer and rethrows the same panic; it does not change production Weir.

## Reproduce the analysis

Raw artifacts are retained under `raw/` with deterministic gzip encoding. Each `files.json` stores uncompressed byte length/SHA-256 and compressed SHA-256. The machine-readable [manifest](manifest.json) binds every report to its workflow and immutable client. The generated [stage table](analysis/stages.csv) retains every stage, including setup/warmup failures, and [paired ratios](analysis/paired-rounds.csv) retain unfavorable and unqualified observations.

From the repository root, build the offline auditor and rerun the receipt, arithmetic, oracle and aggregation checks without contacting databases:

```sh
go build -trimpath -buildvcs=false -o .tools/weir-crawler-audit ./cmd/weir-crawler
python3 results/crawler-2026-10-15/analyze.py \
  --manifest results/crawler-2026-10-15/manifest.json \
  --repo . --auditor .tools/weir-crawler-audit \
  --output results/crawler-2026-10-15/analysis
```

`go_audit_exit` records failures as evidence; it does not reinterpret an incomplete run as passed. Plotting optionally uses the pinned packages in `requirements-plot.txt`:

```sh
python3 results/crawler-2026-10-15/plot.py \
  --analysis results/crawler-2026-10-15/analysis/analysis.json \
  --output results/crawler-2026-10-15/plots
```

Cleanup receipts attest the final owned server process and removal of the owned database container. Earlier server replacements synchronously wait for process shutdown in the fixture code; the artifacts do not contain a separate historical OS-termination receipt for each replacement. The generated fixture uses one hostname and ASCII brand values and does not cover all production branches, arbitrary Lua, replication, multi-node database scaling or production capacity.
