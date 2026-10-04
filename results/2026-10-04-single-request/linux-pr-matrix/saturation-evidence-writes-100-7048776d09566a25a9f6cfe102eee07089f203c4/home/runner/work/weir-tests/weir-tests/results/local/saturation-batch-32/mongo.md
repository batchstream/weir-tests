# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:22:05.725670983Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 71759 | 71759 | 0 | 3586.1 | 14.335 | 69.5 | 41.9 | 98.9 | true |
| 1 | 8 | 1 | weir | 0 | 79450 | 79450 | 0 | 3972.2 | 3.583 | 89.0 | 58.3 | 95.9 | true |
| 1 | 8 | 2 | direct | 0 | 96263 | 96263 | 0 | 4812.5 | 1.791 | 99.8 | 99.7 | 99.7 | true |
| 1 | 8 | 2 | weir | 0 | 81425 | 81425 | 0 | 4070.8 | 3.071 | 93.8 | 90.0 | 95.2 | true |
| 1 | 8 | 3 | direct | 0 | 90702 | 90702 | 0 | 4534.6 | 1.919 | 99.1 | 95.3 | 95.3 | true |
| 1 | 8 | 3 | weir | 0 | 73885 | 73885 | 0 | 3676.9 | 3.583 | 94.3 | 95.6 | 95.6 | true |
| 1 | 32 | 1 | direct | 0 | 114150 | 114150 | 0 | 5706.7 | 49.151 | 98.1 | 91.7 | 97.0 | true |
| 1 | 32 | 1 | weir | 0 | 98278 | 98278 | 0 | 4913.2 | 18.431 | 99.7 | 96.5 | 96.5 | true |
| 1 | 32 | 2 | direct | 0 | 113466 | 113466 | 0 | 5672.8 | 49.151 | 98.7 | 95.5 | 95.5 | true |
| 1 | 32 | 2 | weir | 0 | 99970 | 99970 | 0 | 4996.7 | 18.431 | 98.5 | 91.3 | 96.6 | true |
| 1 | 32 | 3 | direct | 0 | 109872 | 109872 | 0 | 5493.2 | 49.151 | 98.3 | 91.3 | 96.4 | true |
| 1 | 32 | 3 | weir | 0 | 95633 | 95633 | 0 | 4780.7 | 18.431 | 99.6 | 96.2 | 96.2 | true |
| 1 | 128 | 1 | direct | 0 | 109168 | 109168 | 0 | 5457.4 | 81.919 | 99.7 | 97.7 | 97.7 | true |
| 1 | 128 | 1 | weir | 0 | 159782 | 159782 | 0 | 7987.3 | 49.151 | 98.8 | 95.7 | 95.7 | true |
| 1 | 128 | 2 | direct | 0 | 100243 | 100243 | 0 | 4961.2 | 81.919 | 99.5 | 96.9 | 96.9 | true |
| 1 | 128 | 2 | weir | 0 | 148897 | 148897 | 0 | 7443.3 | 49.151 | 96.8 | 86.0 | 96.5 | true |
| 1 | 128 | 3 | direct | 0 | 98710 | 98710 | 0 | 4932.1 | 81.919 | 99.8 | 97.0 | 97.0 | true |
| 1 | 128 | 3 | weir | 0 | 142636 | 142636 | 0 | 7114.9 | 61.439 | 99.1 | 97.0 | 97.0 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 73565 | 1.080 | 0/79450 | 0.009 | 1.408 | 0 | 32.000 |
| 1 | 8 | 2 | 75052 | 1.085 | 0/81425 | 0.008 | 1.416 | 0 | 32.000 |
| 1 | 8 | 3 | 68263 | 1.082 | 0/73885 | 0.009 | 1.558 | 0 | 32.000 |
| 1 | 32 | 1 | 73409 | 1.339 | 0/98278 | 0.048 | 4.391 | 0 | 32.000 |
| 1 | 32 | 2 | 74878 | 1.335 | 0/99970 | 0.046 | 4.332 | 0 | 32.000 |
| 1 | 32 | 3 | 72315 | 1.322 | 0/95633 | 0.044 | 4.754 | 0 | 32.000 |
| 1 | 128 | 1 | 55568 | 2.875 | 0/159782 | 1.132 | 7.907 | 0 | 32.000 |
| 1 | 128 | 2 | 51405 | 2.897 | 0/148897 | 1.331 | 8.426 | 0 | 32.000 |
| 1 | 128 | 3 | 49694 | 2.870 | 0/142636 | 1.289 | 8.621 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 5624.2 (32 workers), Weir 7514.9 (128 workers), observed change +33.62%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 4311.0 | 3906.3 | -9.39% | 1.023 / 2.303 / 30.719 | 1.791 / 3.327 / 5.631 |
| 32 | 5624.2 | 4896.9 | -12.93% | 2.559 / 49.151 / 65.535 | 4.607 / 18.431 / 57.343 |
| 128 | 5116.4 | 7514.9 | +46.88% | 10.239 / 81.919 / 106.495 | 12.287 / 53.247 / 90.111 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
