# Database saturation comparison

Backend **search**. Started 2026-10-03T23:07:46.34783523Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 24457 | 2674 | 27131 | 0 | 1356.4 | 61.439 | 99.6 | 98.9 | 98.9 | true |
| 1 | 8 | 1 | weir | 30321 | 3342 | 33663 | 0 | 1683.0 | 9.215 | 99.9 | 98.8 | 98.8 | true |
| 1 | 8 | 2 | direct | 72111 | 8050 | 80161 | 0 | 4007.8 | 2.815 | 99.6 | 98.4 | 98.4 | true |
| 1 | 8 | 2 | weir | 48024 | 5396 | 53420 | 0 | 2670.7 | 5.119 | 94.1 | 82.0 | 97.9 | true |
| 1 | 8 | 3 | direct | 73249 | 8175 | 81424 | 0 | 4070.6 | 2.559 | 99.7 | 97.0 | 97.0 | true |
| 1 | 8 | 3 | weir | 56733 | 6369 | 63102 | 0 | 3154.7 | 4.607 | 89.7 | 33.0 | 98.5 | true |
| 1 | 32 | 1 | direct | 96920 | 10850 | 107770 | 0 | 5387.7 | 36.863 | 99.8 | 97.8 | 97.8 | true |
| 1 | 32 | 1 | weir | 83455 | 9351 | 92806 | 0 | 4639.3 | 13.311 | 92.0 | 77.5 | 99.7 | true |
| 1 | 32 | 2 | direct | 95889 | 10729 | 106618 | 0 | 5330.2 | 36.863 | 99.8 | 98.4 | 98.4 | true |
| 1 | 32 | 2 | weir | 83234 | 9329 | 92563 | 0 | 4626.7 | 13.311 | 90.8 | 50.1 | 94.6 | true |
| 1 | 32 | 3 | direct | 96492 | 10800 | 107292 | 0 | 5363.7 | 36.863 | 99.8 | 99.0 | 99.0 | true |
| 1 | 32 | 3 | weir | 88025 | 9878 | 97903 | 0 | 4894.2 | 13.311 | 90.0 | 50.1 | 94.7 | true |
| 1 | 128 | 1 | direct | 101254 | 11522 | 112776 | 0 | 5635.6 | 61.439 | 99.9 | 99.0 | 99.0 | true |
| 1 | 128 | 1 | weir | 129765 | 14471 | 144236 | 0 | 7208.8 | 36.863 | 83.0 | 0.0 | 95.4 | true |
| 1 | 128 | 2 | direct | 102314 | 11559 | 113873 | 0 | 5691.7 | 61.439 | 100.0 | 98.9 | 98.9 | true |
| 1 | 128 | 2 | weir | 131642 | 14672 | 146314 | 0 | 7312.4 | 32.767 | 82.0 | 0.0 | 94.8 | true |
| 1 | 128 | 3 | direct | 104586 | 11812 | 116398 | 0 | 5817.8 | 57.343 | 100.1 | 98.9 | 98.9 | true |
| 1 | 128 | 3 | weir | 129580 | 14448 | 144028 | 0 | 7198.8 | 36.863 | 82.1 | 5.7 | 95.5 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 31394 | 1.072 | 30321/3342 | 0.021 | 3.566 | 0 | 32.000 |
| 1 | 8 | 2 | 49298 | 1.084 | 48024/5396 | 0.022 | 1.869 | 0 | 32.000 |
| 1 | 8 | 3 | 57995 | 1.088 | 56733/6369 | 0.021 | 1.445 | 0 | 32.000 |
| 1 | 32 | 1 | 63233 | 1.468 | 83455/9351 | 0.109 | 3.236 | 0 | 32.000 |
| 1 | 32 | 2 | 63022 | 1.469 | 83234/9329 | 0.110 | 3.240 | 0 | 32.000 |
| 1 | 32 | 3 | 65735 | 1.489 | 88025/9878 | 0.112 | 2.863 | 0 | 32.000 |
| 1 | 128 | 1 | 48589 | 2.968 | 129765/14471 | 0.613 | 5.474 | 0 | 32.000 |
| 1 | 128 | 2 | 49344 | 2.965 | 131642/14672 | 0.567 | 5.214 | 0 | 32.000 |
| 1 | 128 | 3 | 48578 | 2.965 | 129580/14448 | 0.632 | 5.540 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 5715.0 (128 workers), Weir 7240.0 (128 workers), observed change +26.68%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 3144.9 | 2502.8 | -20.42% | 1.279 / 3.583 / 57.343 | 2.303 / 5.631 / 40.959 |
| 32 | 5360.5 | 4720.1 | -11.95% | 3.839 / 36.863 / 45.055 | 6.143 / 13.311 / 20.479 |
| 128 | 5715.0 | 7240.0 | +26.68% | 15.359 / 57.343 / 73.727 | 18.431 / 36.863 / 45.055 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
