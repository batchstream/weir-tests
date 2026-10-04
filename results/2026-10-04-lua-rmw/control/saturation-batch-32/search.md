# Database saturation comparison

Backend **search**. Started 2026-10-04T03:25:37.085504342Z.

independent OS client processes send one record per native call or unary SDK RPC; native single-record snapshot transaction FindOne/compute/ReplaceOne/commit or real-time single-ID mget/compute/version-conditional PUT; SDK Lua AtomicTransform computes the same revision toggle from the current source; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 8383 | 8383 | 0 | 419.0 | 73.727 | 99.8 | 98.8 | 98.8 | true |
| 1 | 8 | 1 | weir | 0 | 11593 | 11593 | 0 | 579.5 | 57.343 | 99.9 | 97.8 | 97.8 | true |
| 1 | 8 | 2 | direct | 0 | 23640 | 23640 | 0 | 1181.8 | 11.263 | 100.0 | 96.1 | 96.1 | true |
| 1 | 8 | 2 | weir | 0 | 23271 | 23271 | 0 | 1163.3 | 10.239 | 97.3 | 98.4 | 98.4 | true |
| 1 | 8 | 3 | direct | 0 | 27898 | 27898 | 0 | 1394.5 | 9.215 | 99.8 | 96.2 | 96.2 | true |
| 1 | 8 | 3 | weir | 0 | 26807 | 26807 | 0 | 1340.1 | 9.215 | 96.5 | 98.2 | 98.2 | true |
| 1 | 32 | 1 | direct | 0 | 29576 | 29576 | 0 | 1477.6 | 30.719 | 100.0 | 95.7 | 95.7 | true |
| 1 | 32 | 1 | weir | 0 | 29862 | 29862 | 0 | 1491.9 | 30.719 | 98.4 | 98.3 | 98.3 | true |
| 1 | 32 | 2 | direct | 0 | 29848 | 29848 | 0 | 1491.1 | 28.671 | 99.6 | 95.9 | 95.9 | true |
| 1 | 32 | 2 | weir | 0 | 31226 | 31226 | 0 | 1560.2 | 26.623 | 98.6 | 98.1 | 98.1 | true |
| 1 | 32 | 3 | direct | 0 | 29415 | 29415 | 0 | 1469.2 | 28.671 | 99.9 | 96.0 | 96.0 | true |
| 1 | 32 | 3 | weir | 0 | 30011 | 30011 | 0 | 1499.4 | 28.671 | 98.5 | 98.6 | 98.6 | true |
| 1 | 128 | 1 | direct | 0 | 29899 | 29899 | 0 | 1490.2 | 114.687 | 99.9 | 95.7 | 95.7 | true |
| 1 | 128 | 1 | weir | 0 | 61038 | 61038 | 0 | 3048.4 | 81.919 | 93.3 | 92.8 | 98.4 | true |
| 1 | 128 | 2 | direct | 0 | 28301 | 28301 | 0 | 1410.7 | 131.071 | 100.1 | 95.7 | 95.7 | true |
| 1 | 128 | 2 | weir | 0 | 61793 | 61793 | 0 | 3085.7 | 81.919 | 92.3 | 88.0 | 98.9 | true |
| 1 | 128 | 3 | direct | 0 | 29387 | 29387 | 0 | 1464.1 | 131.071 | 100.0 | 96.0 | 96.0 | true |
| 1 | 128 | 3 | weir | 0 | 61079 | 61079 | 0 | 3049.8 | 81.919 | 92.1 | 88.1 | 99.0 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 11425 | 1.015 | 0/11593 | 0.016 | 12.864 | 0 | 32.000 |
| 1 | 8 | 2 | 22806 | 1.020 | 0/23271 | 0.015 | 6.004 | 0 | 32.000 |
| 1 | 8 | 3 | 26211 | 1.023 | 0/26807 | 0.015 | 5.099 | 0 | 32.000 |
| 1 | 32 | 1 | 28716 | 1.040 | 0/29862 | 0.026 | 20.294 | 0 | 32.000 |
| 1 | 32 | 2 | 29872 | 1.045 | 0/31226 | 0.026 | 19.285 | 0 | 32.000 |
| 1 | 32 | 3 | 28934 | 1.037 | 0/30011 | 0.024 | 20.172 | 0 | 32.000 |
| 1 | 128 | 1 | 23379 | 2.611 | 0/61038 | 0.951 | 24.272 | 0 | 32.000 |
| 1 | 128 | 2 | 24046 | 2.570 | 0/61793 | 0.852 | 23.160 | 0 | 32.000 |
| 1 | 128 | 3 | 23807 | 2.566 | 0/61079 | 0.833 | 23.623 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 1479.3 (32 workers), Weir 3061.3 (128 workers), observed change +106.94%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 998.4 | 1027.6 | +2.93% | 5.119 / 14.335 / 73.727 | 6.143 / 12.287 / 57.343 |
| 32 | 1479.3 | 1517.2 | +2.56% | 20.479 / 28.671 / 57.343 | 20.479 / 28.671 / 61.439 |
| 128 | 1455.0 | 3061.3 | +110.40% | 90.111 / 122.879 / 180.223 | 40.959 / 81.919 / 106.495 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors/commitTransaction/abortTransaction deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
