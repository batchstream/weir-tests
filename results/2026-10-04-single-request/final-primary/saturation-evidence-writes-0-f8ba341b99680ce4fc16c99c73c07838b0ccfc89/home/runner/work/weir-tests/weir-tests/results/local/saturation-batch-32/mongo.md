# Database saturation comparison

Backend **mongo**. Started 2026-10-04T00:00:32.974810417Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 322551 | 0 | 322551 | 0 | 16127.1 | 0.895 | 100.0 | 96.8 | 96.8 | true |
| 1 | 8 | 1 | weir | 142246 | 0 | 142246 | 0 | 7112.0 | 2.559 | 65.1 | 0.0 | 95.5 | true |
| 1 | 8 | 2 | direct | 324966 | 0 | 324966 | 0 | 16247.9 | 0.831 | 100.0 | 95.5 | 95.5 | true |
| 1 | 8 | 2 | weir | 142530 | 0 | 142530 | 0 | 7126.2 | 2.303 | 65.2 | 0.0 | 95.7 | true |
| 1 | 8 | 3 | direct | 324941 | 0 | 324941 | 0 | 16246.6 | 0.831 | 100.0 | 95.8 | 95.8 | true |
| 1 | 8 | 3 | weir | 144668 | 0 | 144668 | 0 | 7233.2 | 2.303 | 66.5 | 0.0 | 95.6 | true |
| 1 | 32 | 1 | direct | 378676 | 0 | 378676 | 0 | 18933.0 | 3.583 | 100.0 | 96.5 | 96.5 | true |
| 1 | 32 | 1 | weir | 219050 | 0 | 219050 | 0 | 10951.2 | 6.143 | 71.8 | 0.0 | 96.5 | true |
| 1 | 32 | 2 | direct | 381603 | 0 | 381603 | 0 | 19078.8 | 3.583 | 100.0 | 97.0 | 97.0 | true |
| 1 | 32 | 2 | weir | 228703 | 0 | 228703 | 0 | 11434.5 | 5.631 | 72.1 | 0.0 | 95.9 | true |
| 1 | 32 | 3 | direct | 394155 | 0 | 394155 | 0 | 19706.6 | 3.327 | 100.1 | 96.8 | 96.8 | true |
| 1 | 32 | 3 | weir | 231724 | 0 | 231724 | 0 | 11584.7 | 5.631 | 71.4 | 0.0 | 95.8 | true |
| 1 | 128 | 1 | direct | 408478 | 0 | 408478 | 0 | 20419.4 | 32.767 | 100.1 | 96.0 | 96.0 | true |
| 1 | 128 | 1 | weir | 368495 | 0 | 368495 | 0 | 18418.0 | 14.335 | 60.5 | 0.0 | 95.9 | true |
| 1 | 128 | 2 | direct | 389786 | 0 | 389786 | 0 | 19485.9 | 36.863 | 100.3 | 97.1 | 97.1 | true |
| 1 | 128 | 2 | weir | 351576 | 0 | 351576 | 0 | 17572.2 | 15.359 | 59.0 | 0.0 | 96.0 | true |
| 1 | 128 | 3 | direct | 391694 | 0 | 391694 | 0 | 19581.6 | 36.863 | 100.3 | 96.6 | 96.6 | true |
| 1 | 128 | 3 | weir | 360872 | 0 | 360872 | 0 | 18041.0 | 14.335 | 59.9 | 0.0 | 95.9 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 127979 | 1.111 | 142246/0 | 0.011 | 0.473 | 0 | 32.000 |
| 1 | 8 | 2 | 128517 | 1.109 | 142530/0 | 0.011 | 0.475 | 0 | 32.000 |
| 1 | 8 | 3 | 130164 | 1.111 | 144668/0 | 0.011 | 0.469 | 0 | 32.000 |
| 1 | 32 | 1 | 134754 | 1.626 | 219050/0 | 0.064 | 0.933 | 0 | 32.000 |
| 1 | 32 | 2 | 144065 | 1.587 | 228703/0 | 0.055 | 0.889 | 0 | 32.000 |
| 1 | 32 | 3 | 145538 | 1.592 | 231724/0 | 0.056 | 0.881 | 0 | 32.000 |
| 1 | 128 | 1 | 109995 | 3.350 | 368495/0 | 0.243 | 1.613 | 0 | 32.000 |
| 1 | 128 | 2 | 101871 | 3.451 | 351576/0 | 0.261 | 1.681 | 0 | 32.000 |
| 1 | 128 | 3 | 106213 | 3.398 | 360872/0 | 0.250 | 1.639 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 19829.0 (128 workers), Weir 18010.4 (128 workers), observed change -9.17%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 16207.2 | 7157.1 | -55.84% | 0.351 / 0.831 / 1.919 | 0.959 / 2.303 / 3.583 |
| 32 | 19239.5 | 11323.4 | -41.14% | 0.959 / 3.583 / 32.767 | 2.559 / 6.143 / 7.679 |
| 128 | 19829.0 | 18010.4 | -9.17% | 3.839 / 36.863 / 45.055 | 6.655 / 14.335 / 20.479 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
