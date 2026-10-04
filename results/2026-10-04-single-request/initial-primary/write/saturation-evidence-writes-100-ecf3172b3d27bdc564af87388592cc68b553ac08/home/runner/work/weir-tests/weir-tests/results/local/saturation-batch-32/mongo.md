# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:23:41.178592185Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 58381 | 58381 | 0 | 2918.8 | 2.815 | 99.1 | 96.9 | 96.9 | true |
| 1 | 8 | 1 | weir | 0 | 51767 | 51767 | 0 | 2587.9 | 5.119 | 97.6 | 99.1 | 99.1 | true |
| 1 | 8 | 2 | direct | 0 | 55152 | 55152 | 0 | 2757.4 | 2.815 | 99.2 | 96.5 | 96.5 | true |
| 1 | 8 | 2 | weir | 0 | 51421 | 51421 | 0 | 2570.8 | 5.119 | 99.6 | 98.0 | 98.0 | true |
| 1 | 8 | 3 | direct | 0 | 53068 | 53068 | 0 | 2653.3 | 3.071 | 99.2 | 96.0 | 96.0 | true |
| 1 | 8 | 3 | weir | 0 | 50286 | 50286 | 0 | 2514.0 | 5.119 | 99.5 | 98.3 | 98.3 | true |
| 1 | 32 | 1 | direct | 0 | 55505 | 55505 | 0 | 2774.5 | 61.439 | 99.4 | 98.1 | 98.1 | true |
| 1 | 32 | 1 | weir | 0 | 55643 | 55643 | 0 | 2778.2 | 36.863 | 99.7 | 99.0 | 99.0 | true |
| 1 | 32 | 2 | direct | 0 | 51549 | 51549 | 0 | 2570.2 | 65.535 | 99.4 | 98.6 | 98.6 | true |
| 1 | 32 | 2 | weir | 0 | 58698 | 58698 | 0 | 2934.3 | 36.863 | 99.6 | 98.9 | 98.9 | true |
| 1 | 32 | 3 | direct | 0 | 55527 | 55527 | 0 | 2766.7 | 61.439 | 99.3 | 98.0 | 98.0 | true |
| 1 | 32 | 3 | weir | 0 | 56039 | 56039 | 0 | 2801.3 | 36.863 | 99.7 | 98.4 | 98.4 | true |
| 1 | 128 | 1 | direct | 0 | 56171 | 56171 | 0 | 2806.9 | 98.303 | 100.0 | 98.3 | 98.3 | true |
| 1 | 128 | 1 | weir | 0 | 83252 | 83252 | 0 | 4158.4 | 81.919 | 99.8 | 98.4 | 98.4 | true |
| 1 | 128 | 2 | direct | 0 | 56370 | 56370 | 0 | 2807.0 | 98.303 | 99.9 | 98.4 | 98.4 | true |
| 1 | 128 | 2 | weir | 0 | 72764 | 72764 | 0 | 3626.9 | 90.111 | 100.0 | 98.4 | 98.4 | true |
| 1 | 128 | 3 | direct | 0 | 57849 | 57849 | 0 | 2850.4 | 98.303 | 99.9 | 97.5 | 97.5 | true |
| 1 | 128 | 3 | weir | 0 | 80945 | 80945 | 0 | 4044.2 | 81.919 | 99.9 | 98.9 | 98.9 | true |
| 1 | 512 | 1 | direct | 0 | 50131 | 50131 | 0 | 2490.3 | 425.983 | 99.7 | 94.9 | 94.9 | true |
| 1 | 512 | 1 | weir | 0 | 95945 | 95945 | 0 | 4793.6 | 294.911 | 99.9 | 98.8 | 98.8 | true |
| 1 | 512 | 2 | direct | 0 | 43488 | 43488 | 0 | 2164.6 | 524.287 | 100.0 | 97.4 | 97.4 | true |
| 1 | 512 | 2 | weir | 0 | 101733 | 101733 | 0 | 5082.3 | 393.215 | 100.0 | 98.8 | 98.8 | true |
| 1 | 512 | 3 | direct | 0 | 48506 | 48506 | 0 | 2390.0 | 458.751 | 99.8 | 99.3 | 99.3 | true |
| 1 | 512 | 3 | weir | 0 | 104736 | 104736 | 0 | 5220.5 | 262.143 | 99.9 | 99.0 | 99.0 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 48160 | 1.075 | 0/51767 | 0.018 | 2.054 | 0 | 32.000 |
| 1 | 8 | 2 | 47687 | 1.078 | 0/51421 | 0.017 | 2.207 | 0 | 32.000 |
| 1 | 8 | 3 | 46652 | 1.078 | 0/50286 | 0.016 | 2.275 | 0 | 32.000 |
| 1 | 32 | 1 | 41099 | 1.354 | 0/55643 | 0.087 | 8.263 | 0 | 32.000 |
| 1 | 32 | 2 | 42921 | 1.368 | 0/58698 | 0.087 | 7.782 | 0 | 32.000 |
| 1 | 32 | 3 | 41313 | 1.356 | 0/56039 | 0.081 | 8.215 | 0 | 32.000 |
| 1 | 128 | 1 | 30084 | 2.767 | 0/83252 | 2.012 | 15.905 | 0 | 32.000 |
| 1 | 128 | 2 | 26128 | 2.785 | 0/72764 | 2.290 | 19.352 | 0 | 32.000 |
| 1 | 128 | 3 | 28952 | 2.796 | 0/80945 | 2.187 | 16.768 | 0 | 32.000 |
| 1 | 512 | 1 | 10294 | 9.320 | 0/95945 | 11.140 | 56.285 | 0 | 32.000 |
| 1 | 512 | 2 | 10816 | 9.406 | 0/101733 | 10.767 | 52.048 | 0 | 32.000 |
| 1 | 512 | 3 | 11488 | 9.117 | 0/104736 | 9.349 | 49.392 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 2821.6 (128 workers), Weir 5032.3 (512 workers), observed change +78.35%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 2776.5 | 2557.6 | -7.89% | 1.535 / 2.815 / 49.151 | 2.815 / 5.119 / 15.359 |
| 32 | 2703.7 | 2837.9 | +4.96% | 5.119 / 61.439 / 81.919 | 7.679 / 36.863 / 73.727 |
| 128 | 2821.6 | 3942.9 | +39.74% | 22.527 / 98.303 / 212.991 | 22.527 / 81.919 / 180.223 |
| 512 | 2348.5 | 5032.3 | +114.28% | 196.607 / 491.519 / 720.895 | 81.919 / 294.911 / 589.823 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
