# Database saturation comparison

Backend **mongo**. Started 2026-10-04T00:11:44.180695575Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 212733 | 0 | 212733 | 0 | 10636.4 | 1.023 | 99.9 | 99.4 | 99.4 | true |
| 1 | 8 | 1 | weir | 102387 | 0 | 102387 | 0 | 5118.9 | 3.071 | 75.4 | 0.0 | 99.7 | true |
| 1 | 8 | 2 | direct | 206314 | 0 | 206314 | 0 | 10315.3 | 1.023 | 99.9 | 99.6 | 99.6 | true |
| 1 | 8 | 2 | weir | 108588 | 0 | 108588 | 0 | 5428.7 | 2.815 | 81.6 | 0.0 | 99.8 | true |
| 1 | 8 | 3 | direct | 208189 | 0 | 208189 | 0 | 10409.0 | 1.023 | 99.8 | 99.4 | 99.4 | true |
| 1 | 8 | 3 | weir | 105715 | 0 | 105715 | 0 | 5285.3 | 2.815 | 80.5 | 0.0 | 94.9 | true |
| 1 | 32 | 1 | direct | 264720 | 0 | 264720 | 0 | 13208.2 | 4.607 | 100.2 | 99.8 | 99.8 | true |
| 1 | 32 | 1 | weir | 142030 | 0 | 142030 | 0 | 7100.4 | 9.215 | 93.0 | 95.0 | 95.0 | true |
| 1 | 32 | 2 | direct | 267837 | 0 | 267837 | 0 | 13390.8 | 4.607 | 100.2 | 95.8 | 95.8 | true |
| 1 | 32 | 2 | weir | 143296 | 0 | 143296 | 0 | 7163.5 | 9.215 | 91.5 | 84.2 | 94.8 | true |
| 1 | 32 | 3 | direct | 264497 | 0 | 264497 | 0 | 13223.8 | 4.607 | 100.1 | 94.8 | 94.8 | true |
| 1 | 32 | 3 | weir | 144245 | 0 | 144245 | 0 | 7210.8 | 9.215 | 92.6 | 94.9 | 94.9 | true |
| 1 | 128 | 1 | direct | 284542 | 0 | 284542 | 0 | 14223.1 | 45.055 | 100.1 | 96.0 | 96.0 | true |
| 1 | 128 | 1 | weir | 162800 | 0 | 162800 | 0 | 8136.1 | 24.575 | 97.8 | 95.5 | 95.5 | true |
| 1 | 128 | 2 | direct | 288710 | 0 | 288710 | 0 | 14404.2 | 45.055 | 100.2 | 95.3 | 95.3 | true |
| 1 | 128 | 2 | weir | 164436 | 0 | 164436 | 0 | 8218.3 | 24.575 | 98.1 | 95.0 | 95.0 | true |
| 1 | 128 | 3 | direct | 287096 | 0 | 287096 | 0 | 14351.8 | 45.055 | 100.1 | 96.7 | 96.7 | true |
| 1 | 128 | 3 | weir | 163140 | 0 | 163140 | 0 | 8152.6 | 24.575 | 97.8 | 95.1 | 95.1 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 102387 | 1.000 | 102387/0 | 0.020 | 0.655 | 0 | 32.000 |
| 1 | 8 | 2 | 108588 | 1.000 | 108588/0 | 0.020 | 0.630 | 0 | 32.000 |
| 1 | 8 | 3 | 105715 | 1.000 | 105715/0 | 0.019 | 0.645 | 0 | 32.000 |
| 1 | 32 | 1 | 142030 | 1.000 | 142030/0 | 0.183 | 1.498 | 0 | 32.000 |
| 1 | 32 | 2 | 143296 | 1.000 | 143296/0 | 0.181 | 1.491 | 0 | 32.000 |
| 1 | 32 | 3 | 144245 | 1.000 | 144245/0 | 0.182 | 1.476 | 0 | 32.000 |
| 1 | 128 | 1 | 162800 | 1.000 | 162800/0 | 7.765 | 2.597 | 0 | 32.000 |
| 1 | 128 | 2 | 164436 | 1.000 | 164436/0 | 7.793 | 2.570 | 0 | 32.000 |
| 1 | 128 | 3 | 163140 | 1.000 | 163140/0 | 7.700 | 2.599 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 14326.4 (128 workers), Weir 8169.0 (128 workers), observed change -42.98%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 10453.6 | 5277.7 | -49.51% | 0.447 / 1.023 / 2.047 | 1.407 / 3.071 / 4.095 |
| 32 | 13274.2 | 7158.2 | -46.07% | 1.279 / 4.607 / 40.959 | 4.607 / 9.215 / 11.263 |
| 128 | 14326.4 | 8169.0 | -42.98% | 5.119 / 45.055 / 53.247 | 15.359 / 24.575 / 28.671 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
