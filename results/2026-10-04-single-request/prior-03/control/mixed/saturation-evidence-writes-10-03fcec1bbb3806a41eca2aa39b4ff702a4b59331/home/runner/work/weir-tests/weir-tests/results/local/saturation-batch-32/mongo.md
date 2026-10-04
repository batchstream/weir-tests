# Database saturation comparison

Backend **mongo**. Started 2026-10-03T23:18:09.088594586Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 90175 | 10143 | 100318 | 0 | 5015.6 | 2.303 | 99.6 | 98.4 | 98.4 | true |
| 1 | 8 | 1 | weir | 65115 | 7298 | 72413 | 0 | 3620.3 | 4.607 | 76.2 | 0.0 | 99.8 | true |
| 1 | 8 | 2 | direct | 89568 | 10073 | 99641 | 0 | 4981.7 | 2.559 | 99.5 | 98.7 | 98.7 | true |
| 1 | 8 | 2 | weir | 65317 | 7317 | 72634 | 0 | 3631.3 | 4.095 | 77.7 | 0.0 | 94.7 | true |
| 1 | 8 | 3 | direct | 89172 | 10028 | 99200 | 0 | 4959.7 | 2.303 | 99.5 | 98.3 | 98.3 | true |
| 1 | 8 | 3 | weir | 66321 | 7415 | 73736 | 0 | 3686.4 | 4.095 | 78.2 | 0.0 | 94.8 | true |
| 1 | 32 | 1 | direct | 94123 | 10546 | 104669 | 0 | 5232.5 | 40.959 | 100.1 | 98.8 | 98.8 | true |
| 1 | 32 | 1 | weir | 94491 | 10566 | 105057 | 0 | 5252.1 | 12.287 | 85.3 | 0.0 | 95.2 | true |
| 1 | 32 | 2 | direct | 94698 | 10574 | 105272 | 0 | 5262.5 | 40.959 | 100.0 | 98.9 | 98.9 | true |
| 1 | 32 | 2 | weir | 94159 | 10526 | 104685 | 0 | 5232.9 | 12.287 | 84.5 | 5.5 | 94.8 | true |
| 1 | 32 | 3 | direct | 95410 | 10671 | 106081 | 0 | 5302.9 | 40.959 | 100.1 | 98.9 | 98.9 | true |
| 1 | 32 | 3 | weir | 94164 | 10524 | 104688 | 0 | 5233.2 | 12.287 | 85.0 | 5.6 | 95.2 | true |
| 1 | 128 | 1 | direct | 88465 | 10073 | 98538 | 0 | 4923.4 | 73.727 | 100.2 | 99.4 | 99.4 | true |
| 1 | 128 | 1 | weir | 138936 | 15462 | 154398 | 0 | 7714.6 | 32.767 | 77.6 | 0.0 | 94.9 | true |
| 1 | 128 | 2 | direct | 91276 | 10326 | 101602 | 0 | 5077.2 | 73.727 | 100.1 | 98.9 | 98.9 | true |
| 1 | 128 | 2 | weir | 138065 | 15400 | 153465 | 0 | 7668.8 | 32.767 | 78.9 | 0.0 | 95.4 | true |
| 1 | 128 | 3 | direct | 87111 | 9897 | 97008 | 0 | 4848.2 | 73.727 | 100.1 | 99.4 | 99.4 | true |
| 1 | 128 | 3 | weir | 131082 | 14635 | 145717 | 0 | 7280.4 | 36.863 | 79.3 | 0.0 | 95.4 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 65535 | 1.105 | 65115/7298 | 0.026 | 0.984 | 0 | 32.000 |
| 1 | 8 | 2 | 65464 | 1.110 | 65317/7317 | 0.026 | 0.990 | 0 | 32.000 |
| 1 | 8 | 3 | 66573 | 1.108 | 66321/7415 | 0.026 | 0.983 | 0 | 32.000 |
| 1 | 32 | 1 | 64116 | 1.639 | 94491/10566 | 0.141 | 1.966 | 0 | 32.000 |
| 1 | 32 | 2 | 63832 | 1.640 | 94159/10526 | 0.143 | 1.976 | 0 | 32.000 |
| 1 | 32 | 3 | 63775 | 1.642 | 94164/10524 | 0.141 | 1.990 | 0 | 32.000 |
| 1 | 128 | 1 | 43970 | 3.511 | 138936/15462 | 0.601 | 3.687 | 0 | 32.000 |
| 1 | 128 | 2 | 43698 | 3.512 | 138065/15400 | 0.583 | 3.879 | 0 | 32.000 |
| 1 | 128 | 3 | 42566 | 3.423 | 131082/14635 | 0.631 | 4.282 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 5266.0 (32 workers), Weir 7554.6 (128 workers), observed change +43.46%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 4985.7 | 3646.0 | -26.87% | 0.895 / 2.303 / 40.959 | 2.047 / 4.095 / 5.631 |
| 32 | 5266.0 | 5239.4 | -0.50% | 3.327 / 40.959 / 53.247 | 5.631 / 12.287 / 16.383 |
| 128 | 4949.6 | 7554.6 | +52.63% | 16.383 / 73.727 / 81.919 | 16.383 / 32.767 / 45.055 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
