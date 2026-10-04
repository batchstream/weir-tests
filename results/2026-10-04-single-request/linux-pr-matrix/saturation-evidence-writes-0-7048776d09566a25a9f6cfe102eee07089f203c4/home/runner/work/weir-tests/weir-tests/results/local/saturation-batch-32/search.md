# Database saturation comparison

Backend **search**. Started 2026-10-03T22:31:44.237926864Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 37943 | 0 | 37943 | 0 | 1897.0 | 4.607 | 100.0 | 98.9 | 98.9 | true |
| 1 | 8 | 1 | weir | 73578 | 0 | 73578 | 0 | 3678.6 | 3.839 | 78.4 | 0.0 | 99.3 | true |
| 1 | 8 | 2 | direct | 112687 | 0 | 112687 | 0 | 5634.0 | 1.791 | 99.6 | 97.0 | 97.0 | true |
| 1 | 8 | 2 | weir | 74344 | 0 | 74344 | 0 | 3716.9 | 3.839 | 76.7 | 0.0 | 99.5 | true |
| 1 | 8 | 3 | direct | 111489 | 0 | 111489 | 0 | 5574.2 | 1.919 | 99.7 | 97.7 | 97.7 | true |
| 1 | 8 | 3 | weir | 73119 | 0 | 73119 | 0 | 3655.6 | 3.839 | 76.2 | 0.0 | 99.7 | true |
| 1 | 32 | 1 | direct | 132886 | 0 | 132886 | 0 | 6643.5 | 12.287 | 99.9 | 98.0 | 98.0 | true |
| 1 | 32 | 1 | weir | 109373 | 0 | 109373 | 0 | 5467.2 | 11.263 | 77.3 | 0.0 | 94.7 | true |
| 1 | 32 | 2 | direct | 135220 | 0 | 135220 | 0 | 6759.9 | 11.263 | 99.8 | 98.5 | 98.5 | true |
| 1 | 32 | 2 | weir | 108706 | 0 | 108706 | 0 | 5434.0 | 11.263 | 75.2 | 0.0 | 94.9 | true |
| 1 | 32 | 3 | direct | 137691 | 0 | 137691 | 0 | 6883.7 | 11.263 | 99.9 | 99.0 | 99.0 | true |
| 1 | 32 | 3 | weir | 109501 | 0 | 109501 | 0 | 5474.0 | 11.263 | 75.1 | 0.0 | 95.0 | true |
| 1 | 128 | 1 | direct | 144966 | 0 | 144966 | 0 | 7245.3 | 45.055 | 100.0 | 99.0 | 99.0 | true |
| 1 | 128 | 1 | weir | 170826 | 0 | 170826 | 0 | 8538.7 | 28.671 | 63.2 | 0.0 | 94.9 | true |
| 1 | 128 | 2 | direct | 144101 | 0 | 144101 | 0 | 7202.5 | 45.055 | 100.0 | 99.0 | 99.0 | true |
| 1 | 128 | 2 | weir | 170078 | 0 | 170078 | 0 | 8500.4 | 28.671 | 62.3 | 0.0 | 94.5 | true |
| 1 | 128 | 3 | direct | 141478 | 0 | 141478 | 0 | 7070.5 | 49.151 | 0.0 | 0.0 | 0.0 | true |
| 1 | 128 | 3 | weir | 168161 | 0 | 168161 | 0 | 8404.9 | 28.671 | 61.6 | 0.0 | 38.9 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 67117 | 1.096 | 73578/0 | 0.022 | 1.102 | 0 | 32.000 |
| 1 | 8 | 2 | 67865 | 1.095 | 74344/0 | 0.022 | 1.080 | 0 | 32.000 |
| 1 | 8 | 3 | 66591 | 1.098 | 73119/0 | 0.023 | 1.095 | 0 | 32.000 |
| 1 | 32 | 1 | 69513 | 1.573 | 109373/0 | 0.124 | 2.380 | 0 | 32.000 |
| 1 | 32 | 2 | 69226 | 1.570 | 108706/0 | 0.120 | 2.408 | 0 | 32.000 |
| 1 | 32 | 3 | 69976 | 1.565 | 109501/0 | 0.122 | 2.375 | 0 | 32.000 |
| 1 | 128 | 1 | 51082 | 3.344 | 170826/0 | 0.543 | 4.452 | 0 | 32.000 |
| 1 | 128 | 2 | 50588 | 3.362 | 170078/0 | 0.535 | 4.382 | 0 | 32.000 |
| 1 | 128 | 3 | 50182 | 3.351 | 168161/0 | 0.549 | 4.380 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 7172.8 (128 workers), Weir 8481.3 (128 workers), observed change +18.24%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 4368.4 | 3683.7 | -15.67% | 1.151 / 2.559 / 26.623 | 2.047 / 3.839 / 5.631 |
| 32 | 6762.4 | 5458.4 | -19.28% | 3.327 / 11.263 / 36.863 | 5.631 / 11.263 / 14.335 |
| 128 | 7172.8 | 8481.3 | +18.24% | 14.335 / 45.055 / 53.247 | 14.335 / 28.671 / 36.863 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
