# Database saturation comparison

Backend **search**. Started 2026-10-04T03:06:27.400501925Z.

independent OS client processes send one record per native call or unary SDK RPC; native single-record snapshot transaction FindOne/compute/ReplaceOne/commit or real-time single-ID mget/compute/version-conditional PUT; SDK Lua AtomicTransform computes the same revision toggle from the current source; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 8588 | 8588 | 0 | 429.3 | 73.727 | 100.0 | 98.3 | 98.3 | true |
| 1 | 8 | 1 | weir | 0 | 11533 | 11533 | 0 | 576.5 | 57.343 | 99.9 | 98.9 | 98.9 | true |
| 1 | 8 | 2 | direct | 0 | 25486 | 25486 | 0 | 1274.0 | 9.215 | 100.0 | 96.3 | 96.3 | true |
| 1 | 8 | 2 | weir | 0 | 23742 | 23742 | 0 | 1186.9 | 10.239 | 96.7 | 97.9 | 97.9 | true |
| 1 | 8 | 3 | direct | 0 | 28058 | 28058 | 0 | 1402.6 | 7.679 | 99.5 | 96.9 | 96.9 | true |
| 1 | 8 | 3 | weir | 0 | 27498 | 27498 | 0 | 1374.6 | 8.191 | 95.8 | 98.4 | 98.4 | true |
| 1 | 32 | 1 | direct | 0 | 30062 | 30062 | 0 | 1501.7 | 28.671 | 99.5 | 95.8 | 95.8 | true |
| 1 | 32 | 1 | weir | 0 | 28389 | 28389 | 0 | 1418.2 | 30.719 | 97.4 | 98.7 | 98.7 | true |
| 1 | 32 | 2 | direct | 0 | 29647 | 29647 | 0 | 1481.2 | 30.719 | 100.2 | 96.4 | 96.4 | true |
| 1 | 32 | 2 | weir | 0 | 29150 | 29150 | 0 | 1456.4 | 28.671 | 98.2 | 98.7 | 98.7 | true |
| 1 | 32 | 3 | direct | 0 | 30315 | 30315 | 0 | 1514.5 | 28.671 | 99.9 | 95.9 | 95.9 | true |
| 1 | 32 | 3 | weir | 0 | 29322 | 29322 | 0 | 1464.7 | 28.671 | 98.5 | 98.4 | 98.4 | true |
| 1 | 128 | 1 | direct | 0 | 30652 | 30652 | 0 | 1527.9 | 114.687 | 99.9 | 95.9 | 95.9 | true |
| 1 | 128 | 1 | weir | 0 | 29466 | 29466 | 0 | 1468.1 | 114.687 | 97.8 | 99.1 | 99.1 | true |
| 1 | 128 | 2 | direct | 0 | 30043 | 30043 | 0 | 1497.5 | 131.071 | 99.9 | 95.9 | 95.9 | true |
| 1 | 128 | 2 | weir | 0 | 29158 | 29158 | 0 | 1451.7 | 122.879 | 98.1 | 98.3 | 98.3 | true |
| 1 | 128 | 3 | direct | 0 | 30792 | 30792 | 0 | 1534.9 | 114.687 | 100.1 | 95.8 | 95.8 | true |
| 1 | 128 | 3 | weir | 0 | 29980 | 29980 | 0 | 1493.8 | 106.495 | 98.4 | 99.1 | 99.1 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 11533 | 1.000 | 0/11533 | 0.016 | 12.976 | 0 | 32.000 |
| 1 | 8 | 2 | 23742 | 1.000 | 0/23742 | 0.015 | 5.853 | 0 | 32.000 |
| 1 | 8 | 3 | 27498 | 1.000 | 0/27498 | 0.015 | 4.954 | 0 | 32.000 |
| 1 | 32 | 1 | 28389 | 1.000 | 0/28389 | 0.024 | 21.491 | 0 | 32.000 |
| 1 | 32 | 2 | 29150 | 1.000 | 0/29150 | 0.025 | 20.905 | 0 | 32.000 |
| 1 | 32 | 3 | 29322 | 1.000 | 0/29322 | 0.026 | 20.710 | 0 | 32.000 |
| 1 | 128 | 1 | 29466 | 1.000 | 0/29466 | 63.882 | 21.676 | 0 | 32.000 |
| 1 | 128 | 2 | 29158 | 1.000 | 0/29158 | 64.450 | 21.912 | 0 | 32.000 |
| 1 | 128 | 3 | 29980 | 1.000 | 0/29980 | 62.722 | 21.295 | 0 | 32.000 |

**Batch 1 at demonstrated database CPU saturation:** direct 1499.1 logical ops/s (32 workers), Weir 1446.4 logical ops/s (32 workers), Weir/direct 0.9648, change -3.52%.

Best verified business QPS within this ladder: direct 1520.1 (128 workers), Weir 1471.2 (128 workers), observed change -3.22%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 1035.3 | 1046.0 | +1.03% | 5.119 / 11.263 / 73.727 | 6.143 / 12.287 / 57.343 |
| 32 | 1499.1 | 1446.4 | -3.52% | 20.479 / 28.671 / 65.535 | 22.527 / 30.719 / 65.535 |
| 128 | 1520.1 | 1471.2 | -3.22% | 81.919 / 114.687 / 163.839 | 90.111 / 114.687 / 163.839 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors/commitTransaction/abortTransaction deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
