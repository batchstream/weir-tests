# Database saturation comparison

Backend **search**. Started 2026-10-03T23:27:09.540253195Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 44360 | 0 | 44360 | 0 | 2217.8 | 3.839 | 99.9 | 98.8 | 98.8 | true |
| 1 | 8 | 1 | weir | 75209 | 0 | 75209 | 0 | 3760.1 | 3.839 | 78.6 | 0.0 | 99.2 | true |
| 1 | 8 | 2 | direct | 112793 | 0 | 112793 | 0 | 5639.4 | 1.791 | 99.8 | 96.6 | 96.6 | true |
| 1 | 8 | 2 | weir | 75381 | 0 | 75381 | 0 | 3768.8 | 3.839 | 77.2 | 0.0 | 98.9 | true |
| 1 | 8 | 3 | direct | 114561 | 0 | 114561 | 0 | 5727.7 | 1.791 | 99.7 | 96.8 | 96.8 | true |
| 1 | 8 | 3 | weir | 75288 | 0 | 75288 | 0 | 3764.1 | 3.839 | 75.1 | 0.0 | 99.2 | true |
| 1 | 32 | 1 | direct | 141307 | 0 | 141307 | 0 | 7064.2 | 9.215 | 99.8 | 98.8 | 98.8 | true |
| 1 | 32 | 1 | weir | 113490 | 0 | 113490 | 0 | 5673.3 | 11.263 | 75.3 | 0.0 | 99.6 | true |
| 1 | 32 | 2 | direct | 137516 | 0 | 137516 | 0 | 6874.5 | 11.263 | 99.7 | 98.4 | 98.4 | true |
| 1 | 32 | 2 | weir | 113683 | 0 | 113683 | 0 | 5683.3 | 11.263 | 76.9 | 0.0 | 99.5 | true |
| 1 | 32 | 3 | direct | 138986 | 0 | 138986 | 0 | 6948.4 | 10.239 | 99.7 | 98.5 | 98.5 | true |
| 1 | 32 | 3 | weir | 114088 | 0 | 114088 | 0 | 5703.5 | 10.239 | 76.4 | 0.0 | 99.4 | true |
| 1 | 128 | 1 | direct | 153034 | 0 | 153034 | 0 | 7649.0 | 45.055 | 99.9 | 99.0 | 99.0 | true |
| 1 | 128 | 1 | weir | 174898 | 0 | 174898 | 0 | 8739.8 | 28.671 | 63.2 | 0.0 | 99.6 | true |
| 1 | 128 | 2 | direct | 151375 | 0 | 151375 | 0 | 7565.6 | 45.055 | 99.9 | 98.5 | 98.5 | true |
| 1 | 128 | 2 | weir | 173855 | 0 | 173855 | 0 | 8689.6 | 28.671 | 63.3 | 0.0 | 99.4 | true |
| 1 | 128 | 3 | direct | 152292 | 0 | 152292 | 0 | 7611.9 | 45.055 | 100.1 | 98.8 | 98.8 | true |
| 1 | 128 | 3 | weir | 174697 | 0 | 174697 | 0 | 8731.3 | 28.671 | 63.3 | 0.0 | 99.7 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 68647 | 1.096 | 75209/0 | 0.021 | 1.080 | 0 | 32.000 |
| 1 | 8 | 2 | 68942 | 1.093 | 75381/0 | 0.021 | 1.078 | 0 | 32.000 |
| 1 | 8 | 3 | 68870 | 1.093 | 75288/0 | 0.021 | 1.062 | 0 | 32.000 |
| 1 | 32 | 1 | 72426 | 1.567 | 113490/0 | 0.116 | 2.342 | 0 | 32.000 |
| 1 | 32 | 2 | 72970 | 1.558 | 113683/0 | 0.114 | 2.322 | 0 | 32.000 |
| 1 | 32 | 3 | 73115 | 1.560 | 114088/0 | 0.115 | 2.288 | 0 | 32.000 |
| 1 | 128 | 1 | 52627 | 3.323 | 174898/0 | 0.527 | 4.363 | 0 | 32.000 |
| 1 | 128 | 2 | 52708 | 3.298 | 173855/0 | 0.511 | 4.255 | 0 | 32.000 |
| 1 | 128 | 3 | 54212 | 3.222 | 174697/0 | 0.529 | 4.343 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 7608.8 (128 workers), Weir 8720.2 (128 workers), observed change +14.61%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 4528.3 | 3764.3 | -16.87% | 1.151 / 2.303 / 26.623 | 2.047 / 3.839 / 5.119 |
| 32 | 6962.4 | 5686.7 | -18.32% | 3.327 / 10.239 / 36.863 | 5.631 / 11.263 / 13.311 |
| 128 | 7608.8 | 8720.2 | +14.61% | 13.311 / 45.055 / 53.247 | 14.335 / 28.671 / 36.863 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
