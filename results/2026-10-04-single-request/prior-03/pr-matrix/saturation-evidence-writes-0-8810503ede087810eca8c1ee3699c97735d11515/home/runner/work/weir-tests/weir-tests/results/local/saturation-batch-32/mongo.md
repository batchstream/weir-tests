# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:58:02.52620518Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 332889 | 0 | 332889 | 0 | 16644.1 | 0.895 | 99.8 | 96.8 | 96.8 | true |
| 1 | 8 | 1 | weir | 149264 | 0 | 149264 | 0 | 7462.9 | 2.303 | 67.4 | 0.0 | 95.4 | true |
| 1 | 8 | 2 | direct | 327495 | 0 | 327495 | 0 | 16374.4 | 0.767 | 100.0 | 96.0 | 96.0 | true |
| 1 | 8 | 2 | weir | 151071 | 0 | 151071 | 0 | 7553.2 | 2.303 | 67.5 | 0.0 | 95.4 | true |
| 1 | 8 | 3 | direct | 327700 | 0 | 327700 | 0 | 16384.3 | 0.767 | 100.0 | 95.7 | 95.7 | true |
| 1 | 8 | 3 | weir | 155654 | 0 | 155654 | 0 | 7782.4 | 2.047 | 69.8 | 0.0 | 95.3 | true |
| 1 | 32 | 1 | direct | 404715 | 0 | 404715 | 0 | 20234.6 | 3.327 | 100.0 | 97.0 | 97.0 | true |
| 1 | 32 | 1 | weir | 231818 | 0 | 231818 | 0 | 11590.2 | 6.143 | 70.6 | 0.0 | 95.8 | true |
| 1 | 32 | 2 | direct | 406477 | 0 | 406477 | 0 | 20322.6 | 3.071 | 100.0 | 97.0 | 97.0 | true |
| 1 | 32 | 2 | weir | 242616 | 0 | 242616 | 0 | 12129.9 | 5.631 | 74.1 | 0.0 | 95.8 | true |
| 1 | 32 | 3 | direct | 402551 | 0 | 402551 | 0 | 20126.5 | 3.327 | 100.0 | 96.0 | 96.0 | true |
| 1 | 32 | 3 | weir | 242188 | 0 | 242188 | 0 | 12108.4 | 5.631 | 73.7 | 0.0 | 95.3 | true |
| 1 | 128 | 1 | direct | 435508 | 0 | 435508 | 0 | 21771.7 | 32.767 | 100.1 | 96.0 | 96.0 | true |
| 1 | 128 | 1 | weir | 407307 | 0 | 407307 | 0 | 20360.9 | 13.311 | 62.7 | 0.0 | 95.5 | true |
| 1 | 128 | 2 | direct | 434017 | 0 | 434017 | 0 | 21696.9 | 32.767 | 100.1 | 96.3 | 96.3 | true |
| 1 | 128 | 2 | weir | 396401 | 0 | 396401 | 0 | 19815.8 | 13.311 | 61.6 | 0.0 | 95.4 | true |
| 1 | 128 | 3 | direct | 431350 | 0 | 431350 | 0 | 21562.9 | 32.767 | 100.1 | 96.1 | 96.1 | true |
| 1 | 128 | 3 | weir | 397406 | 0 | 397406 | 0 | 19868.1 | 13.311 | 62.0 | 0.0 | 95.7 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 134861 | 1.107 | 149264/0 | 0.010 | 0.456 | 0 | 32.000 |
| 1 | 8 | 2 | 136077 | 1.110 | 151071/0 | 0.010 | 0.452 | 0 | 32.000 |
| 1 | 8 | 3 | 139700 | 1.114 | 155654/0 | 0.010 | 0.443 | 0 | 32.000 |
| 1 | 32 | 1 | 147661 | 1.570 | 231818/0 | 0.054 | 0.882 | 0 | 32.000 |
| 1 | 32 | 2 | 151981 | 1.596 | 242616/0 | 0.054 | 0.849 | 0 | 32.000 |
| 1 | 32 | 3 | 150805 | 1.606 | 242188/0 | 0.055 | 0.847 | 0 | 32.000 |
| 1 | 128 | 1 | 115654 | 3.522 | 407307/0 | 0.234 | 1.472 | 0 | 32.000 |
| 1 | 128 | 2 | 115316 | 3.438 | 396401/0 | 0.232 | 1.511 | 0 | 32.000 |
| 1 | 128 | 3 | 116586 | 3.409 | 397406/0 | 0.231 | 1.514 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 21677.1 (128 workers), Weir 20014.9 (128 workers), observed change -7.67%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 16467.6 | 7599.5 | -53.85% | 0.351 / 0.831 / 1.919 | 0.959 / 2.303 / 3.071 |
| 32 | 20227.9 | 11942.8 | -40.96% | 0.895 / 3.327 / 32.767 | 2.559 / 5.631 / 7.679 |
| 128 | 21677.1 | 20014.9 | -7.67% | 3.583 / 32.767 / 40.959 | 6.143 / 13.311 / 18.431 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
