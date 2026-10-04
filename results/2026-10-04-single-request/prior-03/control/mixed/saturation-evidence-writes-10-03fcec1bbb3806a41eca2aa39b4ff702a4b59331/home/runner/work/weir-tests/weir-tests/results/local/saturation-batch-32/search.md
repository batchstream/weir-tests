# Database saturation comparison

Backend **search**. Started 2026-10-03T23:27:21.94208769Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 21756 | 2360 | 24116 | 0 | 1205.6 | 61.439 | 99.8 | 98.9 | 98.9 | true |
| 1 | 8 | 1 | weir | 25319 | 2762 | 28081 | 0 | 1403.9 | 24.575 | 99.9 | 99.5 | 99.5 | true |
| 1 | 8 | 2 | direct | 65470 | 7350 | 72820 | 0 | 3640.7 | 3.583 | 99.6 | 98.5 | 98.5 | true |
| 1 | 8 | 2 | weir | 33871 | 3716 | 37587 | 0 | 1878.2 | 8.191 | 96.1 | 93.4 | 98.9 | true |
| 1 | 8 | 3 | direct | 72480 | 8122 | 80602 | 0 | 4024.7 | 3.071 | 99.6 | 98.0 | 98.0 | true |
| 1 | 8 | 3 | weir | 43078 | 4855 | 47933 | 0 | 2396.3 | 6.143 | 86.8 | 27.4 | 99.4 | true |
| 1 | 32 | 1 | direct | 82458 | 9213 | 91671 | 0 | 4582.7 | 36.863 | 100.0 | 99.0 | 99.0 | true |
| 1 | 32 | 1 | weir | 67319 | 7550 | 74869 | 0 | 3742.0 | 18.431 | 88.3 | 28.0 | 95.2 | true |
| 1 | 32 | 2 | direct | 84266 | 9425 | 93691 | 0 | 4683.9 | 36.863 | 99.8 | 98.5 | 98.5 | true |
| 1 | 32 | 2 | weir | 71900 | 8051 | 79951 | 0 | 3996.4 | 16.383 | 86.3 | 11.0 | 95.4 | true |
| 1 | 32 | 3 | direct | 82562 | 9253 | 91815 | 0 | 4588.7 | 36.863 | 100.0 | 98.5 | 98.5 | true |
| 1 | 32 | 3 | weir | 70005 | 7818 | 77823 | 0 | 3890.0 | 16.383 | 87.8 | 22.7 | 95.8 | true |
| 1 | 128 | 1 | direct | 90292 | 10232 | 100524 | 0 | 5023.3 | 61.439 | 100.0 | 99.0 | 99.0 | true |
| 1 | 128 | 1 | weir | 106903 | 12016 | 118919 | 0 | 5942.3 | 45.055 | 82.7 | 0.0 | 95.3 | true |
| 1 | 128 | 2 | direct | 87822 | 9922 | 97744 | 0 | 4884.9 | 65.535 | 99.9 | 98.8 | 98.8 | true |
| 1 | 128 | 2 | weir | 104673 | 11759 | 116432 | 0 | 5817.6 | 45.055 | 83.5 | 11.1 | 94.5 | true |
| 1 | 128 | 3 | direct | 88453 | 10048 | 98501 | 0 | 4922.4 | 65.535 | 100.0 | 98.9 | 98.9 | true |
| 1 | 128 | 3 | weir | 115819 | 12920 | 128739 | 0 | 6432.6 | 40.959 | 83.0 | 5.5 | 94.6 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 26407 | 1.063 | 25319/2762 | 0.022 | 4.287 | 0 | 32.000 |
| 1 | 8 | 2 | 35000 | 1.074 | 33871/3716 | 0.023 | 2.835 | 0 | 32.000 |
| 1 | 8 | 3 | 44505 | 1.077 | 43078/4855 | 0.023 | 1.943 | 0 | 32.000 |
| 1 | 32 | 1 | 51637 | 1.450 | 67319/7550 | 0.124 | 3.938 | 0 | 32.000 |
| 1 | 32 | 2 | 55199 | 1.448 | 71900/8051 | 0.123 | 3.529 | 0 | 32.000 |
| 1 | 32 | 3 | 53636 | 1.451 | 70005/7818 | 0.125 | 3.676 | 0 | 32.000 |
| 1 | 128 | 1 | 41467 | 2.868 | 106903/12016 | 0.760 | 6.971 | 0 | 32.000 |
| 1 | 128 | 2 | 39809 | 2.925 | 104673/11759 | 0.816 | 7.078 | 0 | 32.000 |
| 1 | 128 | 3 | 43663 | 2.948 | 115819/12920 | 0.658 | 6.179 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 4943.5 (128 workers), Weir 6064.2 (128 workers), observed change +22.67%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 2957.4 | 1892.8 | -36.00% | 1.407 / 4.607 / 57.343 | 2.815 / 8.191 / 45.055 |
| 32 | 4618.4 | 3876.1 | -16.07% | 4.607 / 36.863 / 53.247 | 7.679 / 16.383 / 24.575 |
| 128 | 4943.5 | 6064.2 | +22.67% | 18.431 / 65.535 / 81.919 | 20.479 / 40.959 / 61.439 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
