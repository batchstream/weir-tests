# Database saturation comparison

Backend **search**. Started 2026-10-03T22:31:17.969954598Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 31057 | 31057 | 0 | 1552.6 | 6.143 | 84.0 | 47.2 | 99.5 | true |
| 1 | 8 | 1 | weir | 0 | 34122 | 34122 | 0 | 1705.8 | 5.119 | 64.8 | 5.2 | 99.3 | true |
| 1 | 8 | 2 | direct | 0 | 42086 | 42086 | 0 | 2104.0 | 4.607 | 51.6 | 0.0 | 98.8 | true |
| 1 | 8 | 2 | weir | 0 | 36676 | 36676 | 0 | 1833.4 | 5.119 | 52.9 | 0.0 | 99.3 | true |
| 1 | 8 | 3 | direct | 0 | 43141 | 43141 | 0 | 2156.7 | 4.607 | 44.6 | 0.0 | 98.7 | true |
| 1 | 8 | 3 | weir | 0 | 36742 | 36742 | 0 | 1836.8 | 5.119 | 49.3 | 0.0 | 99.1 | true |
| 1 | 32 | 1 | direct | 0 | 43005 | 43005 | 0 | 2148.7 | 18.431 | 43.9 | 0.0 | 98.7 | true |
| 1 | 32 | 1 | weir | 0 | 37755 | 37755 | 0 | 1886.4 | 18.431 | 50.1 | 0.0 | 99.1 | true |
| 1 | 32 | 2 | direct | 0 | 41701 | 41701 | 0 | 2083.5 | 18.431 | 43.7 | 0.0 | 99.2 | true |
| 1 | 32 | 2 | weir | 0 | 36150 | 36150 | 0 | 1806.1 | 20.479 | 51.4 | 0.0 | 99.1 | true |
| 1 | 32 | 3 | direct | 0 | 41217 | 41217 | 0 | 2059.5 | 18.431 | 42.9 | 0.0 | 98.8 | true |
| 1 | 32 | 3 | weir | 0 | 37438 | 37438 | 0 | 1870.5 | 20.479 | 50.7 | 0.0 | 99.1 | true |
| 1 | 128 | 1 | direct | 0 | 48375 | 48375 | 0 | 2412.7 | 61.439 | 46.2 | 0.0 | 98.5 | true |
| 1 | 128 | 1 | weir | 0 | 127842 | 127842 | 0 | 6386.7 | 24.575 | 56.5 | 0.0 | 95.0 | true |
| 1 | 128 | 2 | direct | 0 | 46413 | 46413 | 0 | 2314.9 | 61.439 | 46.0 | 0.0 | 98.5 | true |
| 1 | 128 | 2 | weir | 0 | 128142 | 128142 | 0 | 6401.7 | 24.575 | 55.2 | 0.0 | 94.9 | true |
| 1 | 128 | 3 | direct | 0 | 48056 | 48056 | 0 | 2397.0 | 61.439 | 0.0 | 0.0 | 0.0 | true |
| 1 | 128 | 3 | weir | 0 | 126028 | 126028 | 0 | 6295.9 | 24.575 | 0.0 | 0.0 | 0.0 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 34054 | 1.002 | 0/34122 | 0.006 | 4.428 | 0 | 32.000 |
| 1 | 8 | 2 | 36629 | 1.001 | 0/36676 | 0.006 | 4.108 | 0 | 32.000 |
| 1 | 8 | 3 | 36707 | 1.001 | 0/36742 | 0.006 | 4.119 | 0 | 32.000 |
| 1 | 32 | 1 | 37699 | 1.001 | 0/37755 | 0.007 | 16.703 | 0 | 32.000 |
| 1 | 32 | 2 | 36111 | 1.001 | 0/36150 | 0.007 | 17.459 | 0 | 32.000 |
| 1 | 32 | 3 | 37400 | 1.001 | 0/37438 | 0.007 | 16.851 | 0 | 32.000 |
| 1 | 128 | 1 | 33623 | 3.802 | 0/127842 | 0.309 | 18.971 | 0 | 32.000 |
| 1 | 128 | 2 | 33578 | 3.816 | 0/128142 | 0.295 | 19.005 | 0 | 32.000 |
| 1 | 128 | 3 | 33136 | 3.803 | 0/126028 | 0.299 | 19.251 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 2374.9 (128 workers), Weir 6361.4 (128 workers), observed change +167.86%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 1937.7 | 1792.0 | -7.52% | 3.839 / 4.607 / 24.575 | 4.607 / 5.119 / 9.215 |
| 32 | 2097.2 | 1854.3 | -11.58% | 15.359 / 18.431 / 30.719 | 18.431 / 20.479 / 30.719 |
| 128 | 2374.9 | 6361.4 | +167.86% | 53.247 / 61.439 / 73.727 | 20.479 / 24.575 / 36.863 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
