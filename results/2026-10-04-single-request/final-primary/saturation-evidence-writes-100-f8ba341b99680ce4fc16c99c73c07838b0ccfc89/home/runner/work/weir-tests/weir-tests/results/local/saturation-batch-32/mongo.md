# Database saturation comparison

Backend **mongo**. Started 2026-10-04T00:00:50.239809154Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 53579 | 53579 | 0 | 2678.6 | 3.071 | 98.9 | 96.4 | 96.4 | true |
| 1 | 8 | 1 | weir | 0 | 47218 | 47218 | 0 | 2360.5 | 6.143 | 96.7 | 99.9 | 99.9 | true |
| 1 | 8 | 2 | direct | 0 | 50654 | 50654 | 0 | 2526.9 | 3.327 | 99.1 | 96.0 | 96.0 | true |
| 1 | 8 | 2 | weir | 0 | 47211 | 47211 | 0 | 2355.7 | 6.143 | 99.6 | 98.2 | 98.2 | true |
| 1 | 8 | 3 | direct | 0 | 50692 | 50692 | 0 | 2534.4 | 3.327 | 99.3 | 96.2 | 96.2 | true |
| 1 | 8 | 3 | weir | 0 | 45168 | 45168 | 0 | 2258.0 | 6.143 | 99.5 | 98.2 | 98.2 | true |
| 1 | 32 | 1 | direct | 0 | 51134 | 51134 | 0 | 2553.6 | 61.439 | 99.2 | 97.4 | 97.4 | true |
| 1 | 32 | 1 | weir | 0 | 53379 | 53379 | 0 | 2668.2 | 36.863 | 99.6 | 99.0 | 99.0 | true |
| 1 | 32 | 2 | direct | 0 | 49691 | 49691 | 0 | 2476.2 | 65.535 | 99.4 | 97.5 | 97.5 | true |
| 1 | 32 | 2 | weir | 0 | 52165 | 52165 | 0 | 2604.4 | 36.863 | 99.8 | 99.1 | 99.1 | true |
| 1 | 32 | 3 | direct | 0 | 50335 | 50335 | 0 | 2516.2 | 61.439 | 99.4 | 97.9 | 97.9 | true |
| 1 | 32 | 3 | weir | 0 | 50485 | 50485 | 0 | 2524.0 | 36.863 | 99.8 | 98.2 | 98.2 | true |
| 1 | 128 | 1 | direct | 0 | 51577 | 51577 | 0 | 2577.5 | 98.303 | 99.9 | 98.8 | 98.8 | true |
| 1 | 128 | 1 | weir | 0 | 71938 | 71938 | 0 | 3594.9 | 90.111 | 99.9 | 98.9 | 98.9 | true |
| 1 | 128 | 2 | direct | 0 | 52131 | 52131 | 0 | 2604.6 | 98.303 | 99.8 | 98.3 | 98.3 | true |
| 1 | 128 | 2 | weir | 0 | 70712 | 70712 | 0 | 3532.4 | 90.111 | 100.0 | 98.9 | 98.9 | true |
| 1 | 128 | 3 | direct | 0 | 55247 | 55247 | 0 | 2751.9 | 98.303 | 100.0 | 99.0 | 99.0 | true |
| 1 | 128 | 3 | weir | 0 | 69114 | 69114 | 0 | 3453.6 | 98.303 | 99.5 | 98.8 | 98.8 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 43846 | 1.077 | 0/47218 | 0.024 | 2.117 | 0 | 32.000 |
| 1 | 8 | 2 | 43688 | 1.081 | 0/47211 | 0.020 | 2.354 | 0 | 32.000 |
| 1 | 8 | 3 | 41787 | 1.081 | 0/45168 | 0.021 | 2.517 | 0 | 32.000 |
| 1 | 32 | 1 | 38716 | 1.379 | 0/53379 | 0.096 | 8.558 | 0 | 32.000 |
| 1 | 32 | 2 | 37105 | 1.406 | 0/52165 | 0.107 | 8.490 | 0 | 32.000 |
| 1 | 32 | 3 | 35949 | 1.404 | 0/50485 | 0.106 | 8.973 | 0 | 32.000 |
| 1 | 128 | 1 | 25621 | 2.808 | 0/71938 | 2.521 | 18.457 | 0 | 32.000 |
| 1 | 128 | 2 | 25970 | 2.723 | 0/70712 | 2.455 | 18.628 | 0 | 32.000 |
| 1 | 128 | 3 | 25035 | 2.761 | 0/69114 | 2.497 | 19.389 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 2644.8 (128 workers), Weir 3527.0 (128 workers), observed change +33.36%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 2579.9 | 2324.8 | -9.89% | 1.535 / 3.327 / 49.151 | 2.815 / 6.143 / 16.383 |
| 32 | 2515.3 | 2598.9 | +3.32% | 5.119 / 61.439 / 81.919 | 8.191 / 36.863 / 73.727 |
| 128 | 2644.8 | 3527.0 | +33.36% | 24.575 / 98.303 / 327.679 | 26.623 / 90.111 / 180.223 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
