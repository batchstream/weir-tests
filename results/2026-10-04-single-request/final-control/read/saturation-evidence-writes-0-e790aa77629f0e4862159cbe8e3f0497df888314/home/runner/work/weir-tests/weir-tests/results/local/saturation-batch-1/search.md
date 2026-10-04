# Database saturation comparison

Backend **search**. Started 2026-10-04T00:20:54.438495429Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 104134 | 0 | 104134 | 0 | 5206.4 | 1.791 | 100.0 | 99.8 | 99.8 | true |
| 1 | 8 | 1 | weir | 104278 | 0 | 104278 | 0 | 5213.6 | 2.815 | 82.4 | 0.0 | 99.5 | true |
| 1 | 8 | 2 | direct | 186356 | 0 | 186356 | 0 | 9315.5 | 1.151 | 99.8 | 99.7 | 99.7 | true |
| 1 | 8 | 2 | weir | 103800 | 0 | 103800 | 0 | 5189.7 | 2.815 | 81.4 | 0.0 | 99.6 | true |
| 1 | 8 | 3 | direct | 183887 | 0 | 183887 | 0 | 9194.1 | 1.151 | 99.6 | 99.5 | 99.5 | true |
| 1 | 8 | 3 | weir | 103130 | 0 | 103130 | 0 | 5156.2 | 2.815 | 79.3 | 0.0 | 99.6 | true |
| 1 | 32 | 1 | direct | 226802 | 0 | 226802 | 0 | 11339.3 | 4.607 | 99.6 | 100.0 | 100.0 | true |
| 1 | 32 | 1 | weir | 143035 | 0 | 143035 | 0 | 7150.5 | 8.191 | 87.4 | 0.0 | 95.0 | true |
| 1 | 32 | 2 | direct | 234494 | 0 | 234494 | 0 | 11723.7 | 4.607 | 99.7 | 99.9 | 99.9 | true |
| 1 | 32 | 2 | weir | 145310 | 0 | 145310 | 0 | 7264.4 | 8.191 | 85.3 | 0.0 | 95.0 | true |
| 1 | 32 | 3 | direct | 236183 | 0 | 236183 | 0 | 11791.2 | 4.095 | 99.8 | 99.3 | 99.3 | true |
| 1 | 32 | 3 | weir | 144316 | 0 | 144316 | 0 | 7214.6 | 8.191 | 85.7 | 0.0 | 94.8 | true |
| 1 | 128 | 1 | direct | 258892 | 0 | 258892 | 0 | 12942.3 | 40.959 | 100.0 | 96.4 | 96.4 | true |
| 1 | 128 | 1 | weir | 159080 | 0 | 159080 | 0 | 7950.6 | 24.575 | 90.0 | 42.3 | 95.4 | true |
| 1 | 128 | 2 | direct | 256226 | 0 | 256226 | 0 | 12808.4 | 40.959 | 99.9 | 96.0 | 96.0 | true |
| 1 | 128 | 2 | weir | 157850 | 0 | 157850 | 0 | 7888.5 | 24.575 | 87.0 | 0.0 | 94.9 | true |
| 1 | 128 | 3 | direct | 259118 | 0 | 259118 | 0 | 12951.8 | 40.959 | 99.9 | 95.2 | 95.2 | true |
| 1 | 128 | 3 | weir | 155027 | 0 | 155027 | 0 | 7746.1 | 24.575 | 85.1 | 0.0 | 94.9 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 104278 | 1.000 | 104278/0 | 0.015 | 0.798 | 0 | 32.000 |
| 1 | 8 | 2 | 103800 | 1.000 | 103800/0 | 0.015 | 0.801 | 0 | 32.000 |
| 1 | 8 | 3 | 103130 | 1.000 | 103130/0 | 0.016 | 0.793 | 0 | 32.000 |
| 1 | 32 | 1 | 143035 | 1.000 | 143035/0 | 0.119 | 2.041 | 0 | 32.000 |
| 1 | 32 | 2 | 145310 | 1.000 | 145310/0 | 0.125 | 1.974 | 0 | 32.000 |
| 1 | 32 | 3 | 144316 | 1.000 | 144316/0 | 0.128 | 1.980 | 0 | 32.000 |
| 1 | 128 | 1 | 159080 | 1.000 | 159080/0 | 8.716 | 3.178 | 0 | 32.000 |
| 1 | 128 | 2 | 157850 | 1.000 | 157850/0 | 8.633 | 3.211 | 0 | 32.000 |
| 1 | 128 | 3 | 155027 | 1.000 | 155027/0 | 8.693 | 3.292 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 12900.8 (128 workers), Weir 7861.7 (128 workers), observed change -39.06%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 7905.4 | 5186.5 | -34.39% | 0.639 / 1.279 / 22.527 | 1.407 / 2.815 / 3.839 |
| 32 | 11618.2 | 7209.9 | -37.94% | 1.791 / 4.095 / 36.863 | 4.607 / 8.191 / 11.263 |
| 128 | 12900.8 | 7861.7 | -39.06% | 6.655 / 40.959 / 49.151 | 16.383 / 24.575 / 28.671 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
