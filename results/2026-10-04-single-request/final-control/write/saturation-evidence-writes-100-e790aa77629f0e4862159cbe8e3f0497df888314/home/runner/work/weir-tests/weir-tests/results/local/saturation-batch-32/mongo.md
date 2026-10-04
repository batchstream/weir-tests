# Database saturation comparison

Backend **mongo**. Started 2026-10-04T00:30:57.506468697Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 54076 | 54076 | 0 | 2703.4 | 3.071 | 99.1 | 96.6 | 96.6 | true |
| 1 | 8 | 1 | weir | 0 | 48936 | 48936 | 0 | 2446.5 | 5.631 | 99.7 | 98.6 | 98.6 | true |
| 1 | 8 | 2 | direct | 0 | 49057 | 49057 | 0 | 2452.7 | 3.583 | 99.5 | 97.0 | 97.0 | true |
| 1 | 8 | 2 | weir | 0 | 46963 | 46963 | 0 | 2347.8 | 6.143 | 99.5 | 99.0 | 99.0 | true |
| 1 | 8 | 3 | direct | 0 | 48436 | 48436 | 0 | 2421.7 | 3.839 | 99.2 | 96.3 | 96.3 | true |
| 1 | 8 | 3 | weir | 0 | 43656 | 43656 | 0 | 2182.5 | 6.655 | 99.6 | 98.3 | 98.3 | true |
| 1 | 32 | 1 | direct | 0 | 50696 | 50696 | 0 | 2534.4 | 61.439 | 99.3 | 97.9 | 97.9 | true |
| 1 | 32 | 1 | weir | 0 | 52796 | 52796 | 0 | 2638.2 | 36.863 | 100.1 | 99.0 | 99.0 | true |
| 1 | 32 | 2 | direct | 0 | 49463 | 49463 | 0 | 2472.6 | 65.535 | 99.4 | 98.0 | 98.0 | true |
| 1 | 32 | 2 | weir | 0 | 56725 | 56725 | 0 | 2832.0 | 32.767 | 99.6 | 98.5 | 98.5 | true |
| 1 | 32 | 3 | direct | 0 | 54023 | 54023 | 0 | 2700.6 | 61.439 | 99.3 | 97.8 | 97.8 | true |
| 1 | 32 | 3 | weir | 0 | 51084 | 51084 | 0 | 2553.3 | 36.863 | 99.9 | 99.0 | 99.0 | true |
| 1 | 128 | 1 | direct | 0 | 51606 | 51606 | 0 | 2578.0 | 98.303 | 99.9 | 98.9 | 98.9 | true |
| 1 | 128 | 1 | weir | 0 | 78372 | 78372 | 0 | 3917.8 | 81.919 | 99.9 | 99.0 | 99.0 | true |
| 1 | 128 | 2 | direct | 0 | 52531 | 52531 | 0 | 2616.5 | 98.303 | 100.0 | 98.4 | 98.4 | true |
| 1 | 128 | 2 | weir | 0 | 69582 | 69582 | 0 | 3476.4 | 90.111 | 99.6 | 98.6 | 98.6 | true |
| 1 | 128 | 3 | direct | 0 | 51838 | 51838 | 0 | 2590.5 | 98.303 | 99.9 | 98.8 | 98.8 | true |
| 1 | 128 | 3 | weir | 0 | 74104 | 74104 | 0 | 3701.9 | 90.111 | 99.9 | 98.9 | 98.9 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 45307 | 1.080 | 0/48936 | 0.020 | 2.231 | 0 | 32.000 |
| 1 | 8 | 2 | 43426 | 1.081 | 0/46963 | 0.021 | 2.345 | 0 | 32.000 |
| 1 | 8 | 3 | 40519 | 1.077 | 0/43656 | 0.021 | 2.621 | 0 | 32.000 |
| 1 | 32 | 1 | 37958 | 1.391 | 0/52796 | 0.107 | 8.615 | 0 | 32.000 |
| 1 | 32 | 2 | 40444 | 1.403 | 0/56725 | 0.107 | 7.449 | 0 | 32.000 |
| 1 | 32 | 3 | 37116 | 1.376 | 0/51084 | 0.103 | 8.934 | 0 | 32.000 |
| 1 | 128 | 1 | 28123 | 2.787 | 0/78372 | 2.157 | 16.213 | 0 | 32.000 |
| 1 | 128 | 2 | 24916 | 2.793 | 0/69582 | 2.512 | 19.553 | 0 | 32.000 |
| 1 | 128 | 3 | 26222 | 2.826 | 0/74104 | 2.346 | 17.474 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 2595.0 (128 workers), Weir 3698.7 (128 workers), observed change +42.53%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 2525.9 | 2325.6 | -7.93% | 1.535 / 3.327 / 53.247 | 2.815 / 6.143 / 18.431 |
| 32 | 2569.2 | 2674.5 | +4.10% | 5.119 / 61.439 / 81.919 | 8.191 / 36.863 / 73.727 |
| 128 | 2595.0 | 3698.7 | +42.53% | 28.671 / 98.303 / 212.991 | 24.575 / 90.111 / 180.223 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
