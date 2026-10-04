# Database saturation comparison

Backend **mongo**. Started 2026-10-04T00:11:44.002441917Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 92222 | 10355 | 102577 | 0 | 5128.5 | 2.303 | 99.6 | 98.9 | 98.9 | true |
| 1 | 8 | 1 | weir | 58961 | 6618 | 65579 | 0 | 3278.6 | 5.119 | 68.8 | 0.0 | 94.9 | true |
| 1 | 8 | 2 | direct | 91365 | 10267 | 101632 | 0 | 5081.3 | 2.303 | 99.5 | 98.0 | 98.0 | true |
| 1 | 8 | 2 | weir | 66326 | 7423 | 73749 | 0 | 3687.0 | 4.095 | 79.7 | 0.0 | 94.7 | true |
| 1 | 8 | 3 | direct | 91295 | 10259 | 101554 | 0 | 5077.3 | 2.303 | 99.5 | 98.2 | 98.2 | true |
| 1 | 8 | 3 | weir | 63457 | 7130 | 70587 | 0 | 3528.8 | 4.607 | 76.1 | 0.0 | 94.3 | true |
| 1 | 32 | 1 | direct | 96831 | 10830 | 107661 | 0 | 5372.3 | 40.959 | 100.1 | 99.0 | 99.0 | true |
| 1 | 32 | 1 | weir | 76760 | 8563 | 85323 | 0 | 4264.4 | 15.359 | 91.0 | 72.5 | 95.2 | true |
| 1 | 32 | 2 | direct | 94663 | 10606 | 105269 | 0 | 5262.8 | 40.959 | 100.1 | 98.6 | 98.6 | true |
| 1 | 32 | 2 | weir | 76584 | 8561 | 85145 | 0 | 4256.2 | 14.335 | 92.3 | 95.7 | 95.7 | true |
| 1 | 32 | 3 | direct | 96315 | 10784 | 107099 | 0 | 5354.0 | 40.959 | 100.1 | 98.3 | 98.3 | true |
| 1 | 32 | 3 | weir | 73935 | 8256 | 82191 | 0 | 4108.7 | 15.359 | 91.1 | 84.6 | 95.9 | true |
| 1 | 128 | 1 | direct | 88292 | 10035 | 98327 | 0 | 4913.1 | 73.727 | 100.1 | 98.9 | 98.9 | true |
| 1 | 128 | 1 | weir | 84707 | 9580 | 94287 | 0 | 4709.7 | 40.959 | 98.4 | 95.2 | 95.2 | true |
| 1 | 128 | 2 | direct | 96795 | 10978 | 107773 | 0 | 5385.2 | 65.535 | 99.9 | 98.2 | 98.2 | true |
| 1 | 128 | 2 | weir | 85254 | 9653 | 94907 | 0 | 4740.4 | 40.959 | 99.1 | 96.0 | 96.0 | true |
| 1 | 128 | 3 | direct | 94137 | 10696 | 104833 | 0 | 5238.0 | 73.727 | 100.1 | 99.4 | 99.4 | true |
| 1 | 128 | 3 | weir | 83681 | 9487 | 93168 | 0 | 4653.8 | 45.055 | 98.5 | 95.7 | 95.7 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 65579 | 1.000 | 58961/6618 | 0.030 | 1.044 | 0 | 32.000 |
| 1 | 8 | 2 | 73749 | 1.000 | 66326/7423 | 0.029 | 0.970 | 0 | 32.000 |
| 1 | 8 | 3 | 70587 | 1.000 | 63457/7130 | 0.028 | 1.003 | 0 | 32.000 |
| 1 | 32 | 1 | 85323 | 1.000 | 76760/8563 | 0.283 | 2.625 | 0 | 32.000 |
| 1 | 32 | 2 | 85145 | 1.000 | 76584/8561 | 0.295 | 2.661 | 0 | 32.000 |
| 1 | 32 | 3 | 82191 | 1.000 | 73935/8256 | 0.293 | 2.731 | 0 | 32.000 |
| 1 | 128 | 1 | 94287 | 1.000 | 84707/9580 | 14.027 | 4.581 | 0 | 32.000 |
| 1 | 128 | 2 | 94907 | 1.000 | 85254/9653 | 14.162 | 4.568 | 0 | 32.000 |
| 1 | 128 | 3 | 93168 | 1.000 | 83681/9487 | 14.290 | 4.602 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 5329.7 (32 workers), Weir 4701.3 (128 workers), observed change -11.79%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 5095.7 | 3498.1 | -31.35% | 0.895 / 2.303 / 36.863 | 2.303 / 4.607 / 6.143 |
| 32 | 5329.7 | 4209.8 | -21.01% | 3.327 / 40.959 / 53.247 | 7.167 / 15.359 / 20.479 |
| 128 | 5178.7 | 4701.3 | -9.22% | 15.359 / 73.727 / 81.919 | 26.623 / 40.959 / 53.247 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
