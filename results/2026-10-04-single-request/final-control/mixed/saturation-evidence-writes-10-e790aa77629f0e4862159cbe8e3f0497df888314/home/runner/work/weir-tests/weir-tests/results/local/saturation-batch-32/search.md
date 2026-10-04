# Database saturation comparison

Backend **search**. Started 2026-10-04T00:40:04.098409771Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 23867 | 2596 | 26463 | 0 | 1323.0 | 57.343 | 99.9 | 98.9 | 98.9 | true |
| 1 | 8 | 1 | weir | 29689 | 3283 | 32972 | 0 | 1648.4 | 10.239 | 100.0 | 99.0 | 99.0 | true |
| 1 | 8 | 2 | direct | 72211 | 8107 | 80318 | 0 | 4015.6 | 2.815 | 99.7 | 98.3 | 98.3 | true |
| 1 | 8 | 2 | weir | 42395 | 4742 | 47137 | 0 | 2356.5 | 5.631 | 96.8 | 99.0 | 99.0 | true |
| 1 | 8 | 3 | direct | 76157 | 8510 | 84667 | 0 | 4233.2 | 2.815 | 99.7 | 97.0 | 97.0 | true |
| 1 | 8 | 3 | weir | 54159 | 6084 | 60243 | 0 | 3011.9 | 4.607 | 89.7 | 22.0 | 99.1 | true |
| 1 | 32 | 1 | direct | 89290 | 9994 | 99284 | 0 | 4963.5 | 36.863 | 99.8 | 99.0 | 99.0 | true |
| 1 | 32 | 1 | weir | 77814 | 8711 | 86525 | 0 | 4324.4 | 14.335 | 92.6 | 88.8 | 99.9 | true |
| 1 | 32 | 2 | direct | 96404 | 10795 | 107199 | 0 | 5358.7 | 36.863 | 99.9 | 98.8 | 98.8 | true |
| 1 | 32 | 2 | weir | 85371 | 9580 | 94951 | 0 | 4746.8 | 13.311 | 91.0 | 66.8 | 99.8 | true |
| 1 | 32 | 3 | direct | 93180 | 10442 | 103622 | 0 | 5180.0 | 36.863 | 99.9 | 98.8 | 98.8 | true |
| 1 | 32 | 3 | weir | 83032 | 9295 | 92327 | 0 | 4615.1 | 13.311 | 91.4 | 77.4 | 99.6 | true |
| 1 | 128 | 1 | direct | 98166 | 11081 | 109247 | 0 | 5459.5 | 61.439 | 100.0 | 98.9 | 98.9 | true |
| 1 | 128 | 1 | weir | 126763 | 14104 | 140867 | 0 | 7038.8 | 36.863 | 84.1 | 5.6 | 94.4 | true |
| 1 | 128 | 2 | direct | 96736 | 10951 | 107687 | 0 | 5382.6 | 61.439 | 100.1 | 98.9 | 98.9 | true |
| 1 | 128 | 2 | weir | 129490 | 14455 | 143945 | 0 | 7194.3 | 36.863 | 82.8 | 0.0 | 95.2 | true |
| 1 | 128 | 3 | direct | 96391 | 10948 | 107339 | 0 | 5364.5 | 61.439 | 99.9 | 98.9 | 98.9 | true |
| 1 | 128 | 3 | weir | 123117 | 13710 | 136827 | 0 | 6838.9 | 36.863 | 83.3 | 16.8 | 95.1 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 30788 | 1.071 | 29689/3283 | 0.022 | 3.631 | 0 | 32.000 |
| 1 | 8 | 2 | 43651 | 1.080 | 42395/4742 | 0.022 | 2.249 | 0 | 32.000 |
| 1 | 8 | 3 | 55634 | 1.083 | 54159/6084 | 0.021 | 1.548 | 0 | 32.000 |
| 1 | 32 | 1 | 59249 | 1.460 | 77814/8711 | 0.112 | 3.658 | 0 | 32.000 |
| 1 | 32 | 2 | 64481 | 1.473 | 85371/9580 | 0.109 | 3.082 | 0 | 32.000 |
| 1 | 32 | 3 | 63001 | 1.465 | 83032/9295 | 0.110 | 3.270 | 0 | 32.000 |
| 1 | 128 | 1 | 47676 | 2.955 | 126763/14104 | 0.645 | 5.767 | 0 | 32.000 |
| 1 | 128 | 2 | 48177 | 2.988 | 129490/14455 | 0.586 | 5.532 | 0 | 32.000 |
| 1 | 128 | 3 | 45901 | 2.981 | 123117/13710 | 0.702 | 6.131 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 5402.2 (128 workers), Weir 7024.0 (128 workers), observed change +30.02%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 3190.6 | 2338.9 | -26.69% | 1.279 / 3.583 / 53.247 | 2.559 / 5.631 / 45.055 |
| 32 | 5167.4 | 4562.1 | -11.71% | 3.839 / 36.863 / 49.151 | 6.143 / 13.311 / 28.671 |
| 128 | 5402.2 | 7024.0 | +30.02% | 16.383 / 61.439 / 73.727 | 18.431 / 36.863 / 53.247 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
