# Database saturation comparison

Backend **mongo**. Started 2026-10-03T23:18:07.468769669Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 41754 | 41754 | 0 | 2087.5 | 4.095 | 55.4 | 0.0 | 99.2 | true |
| 1 | 8 | 1 | weir | 0 | 49000 | 49000 | 0 | 2446.9 | 4.607 | 71.4 | 32.2 | 95.4 | true |
| 1 | 8 | 2 | direct | 0 | 69684 | 69684 | 0 | 3483.9 | 2.559 | 91.9 | 68.0 | 99.4 | true |
| 1 | 8 | 2 | weir | 0 | 34655 | 34655 | 0 | 1732.6 | 4.607 | 51.2 | 10.6 | 100.0 | true |
| 1 | 8 | 3 | direct | 0 | 59732 | 59732 | 0 | 2982.2 | 2.559 | 84.1 | 63.2 | 99.6 | true |
| 1 | 8 | 3 | weir | 0 | 61149 | 61149 | 0 | 3055.4 | 3.839 | 94.5 | 85.3 | 95.9 | true |
| 1 | 32 | 1 | direct | 0 | 68905 | 68905 | 0 | 3405.6 | 57.343 | 94.8 | 85.0 | 95.4 | true |
| 1 | 32 | 1 | weir | 0 | 70290 | 70290 | 0 | 3510.1 | 28.671 | 96.2 | 76.0 | 97.2 | true |
| 1 | 32 | 2 | direct | 0 | 66181 | 66181 | 0 | 3308.6 | 57.343 | 88.6 | 57.9 | 95.3 | true |
| 1 | 32 | 2 | weir | 0 | 72419 | 72419 | 0 | 3617.5 | 28.671 | 96.6 | 85.8 | 96.3 | true |
| 1 | 32 | 3 | direct | 0 | 66820 | 66820 | 0 | 3329.6 | 61.439 | 92.4 | 80.0 | 96.0 | true |
| 1 | 32 | 3 | weir | 0 | 61629 | 61629 | 0 | 3075.1 | 30.719 | 89.6 | 75.4 | 96.6 | true |
| 1 | 128 | 1 | direct | 0 | 70209 | 70209 | 0 | 3507.7 | 98.303 | 96.0 | 87.3 | 97.7 | true |
| 1 | 128 | 1 | weir | 0 | 98475 | 98475 | 0 | 4897.2 | 73.727 | 96.4 | 81.0 | 96.7 | true |
| 1 | 128 | 2 | direct | 0 | 72057 | 72057 | 0 | 3569.1 | 90.111 | 96.7 | 84.9 | 95.6 | true |
| 1 | 128 | 2 | weir | 0 | 89960 | 89960 | 0 | 4449.4 | 90.111 | 87.9 | 58.4 | 95.3 | true |
| 1 | 128 | 3 | direct | 0 | 66894 | 66894 | 0 | 3343.4 | 106.495 | 94.2 | 70.4 | 97.0 | true |
| 1 | 128 | 3 | weir | 0 | 92024 | 92024 | 0 | 4586.1 | 90.111 | 93.2 | 64.5 | 96.1 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 45530 | 1.076 | 0/49000 | 0.011 | 2.488 | 0 | 32.000 |
| 1 | 8 | 2 | 32227 | 1.075 | 0/34655 | 0.012 | 3.882 | 0 | 32.000 |
| 1 | 8 | 3 | 56600 | 1.080 | 0/61149 | 0.011 | 1.945 | 0 | 32.000 |
| 1 | 32 | 1 | 52201 | 1.347 | 0/70290 | 0.064 | 6.569 | 0 | 32.000 |
| 1 | 32 | 2 | 53490 | 1.354 | 0/72419 | 0.064 | 6.319 | 0 | 32.000 |
| 1 | 32 | 3 | 45925 | 1.342 | 0/61629 | 0.060 | 7.711 | 0 | 32.000 |
| 1 | 128 | 1 | 35013 | 2.813 | 0/98475 | 2.156 | 13.671 | 0 | 32.000 |
| 1 | 128 | 2 | 31261 | 2.878 | 0/89960 | 2.472 | 15.683 | 0 | 32.000 |
| 1 | 128 | 3 | 32605 | 2.822 | 0/92024 | 2.244 | 15.212 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 3473.7 (128 workers), Weir 4643.9 (128 workers), observed change +33.69%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 2851.3 | 2411.8 | -15.41% | 1.279 / 2.559 / 45.055 | 2.047 / 4.607 / 32.767 |
| 32 | 3348.1 | 3400.8 | +1.57% | 3.583 / 57.343 / 90.111 | 6.143 / 28.671 / 73.727 |
| 128 | 3473.7 | 4643.9 | +33.69% | 16.383 / 98.303 / 212.991 | 16.383 / 81.919 / 180.223 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
