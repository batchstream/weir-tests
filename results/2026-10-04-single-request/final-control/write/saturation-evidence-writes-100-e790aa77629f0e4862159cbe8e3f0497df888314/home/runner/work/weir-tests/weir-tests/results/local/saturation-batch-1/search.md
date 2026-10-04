# Database saturation comparison

Backend **search**. Started 2026-10-04T00:21:02.106828916Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 12935 | 12935 | 0 | 646.6 | 57.343 | 100.1 | 98.4 | 98.4 | true |
| 1 | 8 | 1 | weir | 0 | 18081 | 18081 | 0 | 903.8 | 45.055 | 96.1 | 81.5 | 97.3 | true |
| 1 | 8 | 2 | direct | 0 | 31655 | 31655 | 0 | 1582.4 | 6.655 | 72.4 | 5.3 | 95.4 | true |
| 1 | 8 | 2 | weir | 0 | 27907 | 27907 | 0 | 1395.0 | 7.679 | 73.1 | 0.0 | 96.3 | true |
| 1 | 8 | 3 | direct | 0 | 33224 | 33224 | 0 | 1660.9 | 6.143 | 69.9 | 5.3 | 95.0 | true |
| 1 | 8 | 3 | weir | 0 | 30187 | 30187 | 0 | 1509.0 | 6.655 | 68.7 | 5.3 | 96.3 | true |
| 1 | 32 | 1 | direct | 0 | 35165 | 35165 | 0 | 1756.8 | 22.527 | 69.7 | 0.0 | 95.3 | true |
| 1 | 32 | 1 | weir | 0 | 30480 | 30480 | 0 | 1522.6 | 26.623 | 68.8 | 0.0 | 96.3 | true |
| 1 | 32 | 2 | direct | 0 | 34783 | 34783 | 0 | 1737.6 | 24.575 | 67.2 | 0.0 | 95.1 | true |
| 1 | 32 | 2 | weir | 0 | 30231 | 30231 | 0 | 1510.1 | 26.623 | 68.3 | 5.3 | 96.1 | true |
| 1 | 32 | 3 | direct | 0 | 35768 | 35768 | 0 | 1787.1 | 22.527 | 67.0 | 0.0 | 95.1 | true |
| 1 | 32 | 3 | weir | 0 | 31025 | 31025 | 0 | 1549.4 | 26.623 | 67.3 | 0.0 | 96.1 | true |
| 1 | 128 | 1 | direct | 0 | 38004 | 38004 | 0 | 1894.6 | 81.919 | 70.1 | 0.0 | 95.0 | true |
| 1 | 128 | 1 | weir | 0 | 30135 | 30135 | 0 | 1501.2 | 106.495 | 68.9 | 0.0 | 95.9 | true |
| 1 | 128 | 2 | direct | 0 | 38729 | 38729 | 0 | 1930.9 | 81.919 | 65.6 | 0.0 | 95.0 | true |
| 1 | 128 | 2 | weir | 0 | 30265 | 30265 | 0 | 1507.8 | 106.495 | 65.7 | 0.0 | 95.9 | true |
| 1 | 128 | 3 | direct | 0 | 39472 | 39472 | 0 | 1968.3 | 81.919 | 69.0 | 0.0 | 95.0 | true |
| 1 | 128 | 3 | weir | 0 | 31118 | 31118 | 0 | 1550.0 | 106.495 | 66.1 | 0.0 | 95.8 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 18081 | 1.000 | 0/18081 | 0.011 | 8.348 | 0 | 32.000 |
| 1 | 8 | 2 | 27907 | 1.000 | 0/27907 | 0.010 | 5.300 | 0 | 32.000 |
| 1 | 8 | 3 | 30187 | 1.000 | 0/30187 | 0.010 | 4.879 | 0 | 32.000 |
| 1 | 32 | 1 | 30480 | 1.000 | 0/30480 | 0.012 | 20.560 | 0 | 32.000 |
| 1 | 32 | 2 | 30231 | 1.000 | 0/30231 | 0.012 | 20.739 | 0 | 32.000 |
| 1 | 32 | 3 | 31025 | 1.000 | 0/31025 | 0.012 | 20.208 | 0 | 32.000 |
| 1 | 128 | 1 | 30135 | 1.000 | 0/30135 | 63.282 | 21.267 | 0 | 32.000 |
| 1 | 128 | 2 | 30265 | 1.000 | 0/30265 | 63.014 | 21.174 | 0 | 32.000 |
| 1 | 128 | 3 | 31118 | 1.000 | 0/31118 | 61.284 | 20.597 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 1931.3 (128 workers), Weir 1527.4 (32 workers), observed change -20.91%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 1296.6 | 1269.3 | -2.11% | 4.607 / 8.191 / 57.343 | 5.119 / 8.191 / 45.055 |
| 32 | 1760.5 | 1527.4 | -13.24% | 18.431 / 22.527 / 36.863 | 20.479 / 26.623 / 40.959 |
| 128 | 1931.3 | 1519.7 | -21.31% | 65.535 / 81.919 / 106.495 | 81.919 / 106.495 / 147.455 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
