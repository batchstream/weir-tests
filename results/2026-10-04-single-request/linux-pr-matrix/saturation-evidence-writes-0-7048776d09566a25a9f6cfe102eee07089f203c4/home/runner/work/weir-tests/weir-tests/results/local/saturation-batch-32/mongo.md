# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:22:32.878998562Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 123324 | 0 | 123324 | 0 | 6165.8 | 1.791 | 100.0 | 99.0 | 99.0 | true |
| 1 | 8 | 1 | weir | 77570 | 0 | 77570 | 0 | 3878.0 | 3.839 | 65.0 | 0.0 | 99.8 | true |
| 1 | 8 | 2 | direct | 123258 | 0 | 123258 | 0 | 6162.6 | 1.791 | 99.8 | 99.0 | 99.0 | true |
| 1 | 8 | 2 | weir | 81185 | 0 | 81185 | 0 | 4058.8 | 3.583 | 68.1 | 0.0 | 99.7 | true |
| 1 | 8 | 3 | direct | 123429 | 0 | 123429 | 0 | 6171.1 | 1.791 | 100.0 | 98.6 | 98.6 | true |
| 1 | 8 | 3 | weir | 80484 | 0 | 80484 | 0 | 4023.7 | 3.583 | 67.5 | 0.0 | 99.7 | true |
| 1 | 32 | 1 | direct | 129756 | 0 | 129756 | 0 | 6486.8 | 20.479 | 100.1 | 98.3 | 98.3 | true |
| 1 | 32 | 1 | weir | 124717 | 0 | 124717 | 0 | 6234.6 | 10.239 | 69.4 | 0.0 | 99.7 | true |
| 1 | 32 | 2 | direct | 129991 | 0 | 129991 | 0 | 6498.5 | 18.431 | 100.0 | 98.4 | 98.4 | true |
| 1 | 32 | 2 | weir | 124617 | 0 | 124617 | 0 | 6229.8 | 10.239 | 69.7 | 0.0 | 94.5 | true |
| 1 | 32 | 3 | direct | 130531 | 0 | 130531 | 0 | 6525.4 | 18.431 | 100.0 | 98.0 | 98.0 | true |
| 1 | 32 | 3 | weir | 125155 | 0 | 125155 | 0 | 6256.0 | 10.239 | 70.0 | 0.0 | 99.6 | true |
| 1 | 128 | 1 | direct | 126241 | 0 | 126241 | 0 | 6308.3 | 57.343 | 100.1 | 98.9 | 98.9 | true |
| 1 | 128 | 1 | weir | 198428 | 0 | 198428 | 0 | 9915.0 | 26.623 | 54.6 | 0.0 | 99.5 | true |
| 1 | 128 | 2 | direct | 127334 | 0 | 127334 | 0 | 6363.1 | 57.343 | 100.2 | 98.8 | 98.8 | true |
| 1 | 128 | 2 | weir | 203460 | 0 | 203460 | 0 | 10167.8 | 24.575 | 55.8 | 0.0 | 99.5 | true |
| 1 | 128 | 3 | direct | 124203 | 0 | 124203 | 0 | 6205.3 | 57.343 | 100.2 | 99.6 | 99.6 | true |
| 1 | 128 | 3 | weir | 196247 | 0 | 196247 | 0 | 9807.7 | 26.623 | 55.6 | 0.0 | 99.4 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 69319 | 1.119 | 77570/0 | 0.026 | 0.857 | 0 | 32.000 |
| 1 | 8 | 2 | 72431 | 1.121 | 81185/0 | 0.026 | 0.828 | 0 | 32.000 |
| 1 | 8 | 3 | 72155 | 1.115 | 80484/0 | 0.026 | 0.839 | 0 | 32.000 |
| 1 | 32 | 1 | 70492 | 1.769 | 124717/0 | 0.147 | 1.589 | 0 | 32.000 |
| 1 | 32 | 2 | 71088 | 1.753 | 124617/0 | 0.139 | 1.586 | 0 | 32.000 |
| 1 | 32 | 3 | 71648 | 1.747 | 125155/0 | 0.138 | 1.575 | 0 | 32.000 |
| 1 | 128 | 1 | 49706 | 3.992 | 198428/0 | 0.538 | 2.746 | 0 | 32.000 |
| 1 | 128 | 2 | 51221 | 3.972 | 203460/0 | 0.531 | 2.716 | 0 | 32.000 |
| 1 | 128 | 3 | 51003 | 3.848 | 196247/0 | 0.524 | 2.805 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 6503.6 (32 workers), Weir 9963.5 (128 workers), observed change +53.20%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 6166.5 | 3986.8 | -35.35% | 0.831 / 1.791 / 30.719 | 1.919 / 3.839 / 5.119 |
| 32 | 6503.6 | 6240.1 | -4.05% | 3.071 / 18.431 / 45.055 | 5.119 / 10.239 / 13.311 |
| 128 | 6292.2 | 9963.5 | +58.35% | 14.335 / 57.343 / 65.535 | 12.287 / 26.623 / 36.863 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
