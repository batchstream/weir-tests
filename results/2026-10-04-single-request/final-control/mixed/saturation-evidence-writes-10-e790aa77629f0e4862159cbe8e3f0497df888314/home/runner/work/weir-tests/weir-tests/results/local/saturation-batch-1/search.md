# Database saturation comparison

Backend **search**. Started 2026-10-04T00:20:56.778820909Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 23728 | 2595 | 26323 | 0 | 1316.0 | 61.439 | 99.7 | 98.9 | 98.9 | true |
| 1 | 8 | 1 | weir | 30091 | 3324 | 33415 | 0 | 1670.5 | 10.239 | 100.0 | 98.9 | 98.9 | true |
| 1 | 8 | 2 | direct | 74473 | 8301 | 82774 | 0 | 4138.4 | 2.815 | 99.9 | 97.1 | 97.1 | true |
| 1 | 8 | 2 | weir | 45011 | 5045 | 50056 | 0 | 2502.5 | 5.631 | 96.5 | 93.4 | 98.9 | true |
| 1 | 8 | 3 | direct | 78741 | 8804 | 87545 | 0 | 4377.0 | 2.815 | 99.7 | 98.0 | 98.0 | true |
| 1 | 8 | 3 | weir | 57529 | 6447 | 63976 | 0 | 3198.4 | 4.607 | 90.4 | 32.5 | 98.1 | true |
| 1 | 32 | 1 | direct | 95675 | 10700 | 106375 | 0 | 5318.0 | 36.863 | 99.8 | 98.3 | 98.3 | true |
| 1 | 32 | 1 | weir | 73168 | 8186 | 81354 | 0 | 4066.1 | 14.335 | 97.6 | 100.0 | 100.0 | true |
| 1 | 32 | 2 | direct | 93010 | 10422 | 103432 | 0 | 5170.4 | 36.863 | 99.8 | 99.0 | 99.0 | true |
| 1 | 32 | 2 | weir | 74037 | 8282 | 82319 | 0 | 4115.2 | 14.335 | 97.3 | 95.1 | 95.1 | true |
| 1 | 32 | 3 | direct | 89579 | 10044 | 99623 | 0 | 4980.3 | 36.863 | 99.8 | 98.9 | 98.9 | true |
| 1 | 32 | 3 | weir | 76120 | 8521 | 84641 | 0 | 4231.0 | 14.335 | 97.5 | 94.7 | 94.7 | true |
| 1 | 128 | 1 | direct | 97664 | 11102 | 108766 | 0 | 5426.7 | 61.439 | 100.0 | 98.5 | 98.5 | true |
| 1 | 128 | 1 | weir | 76538 | 8632 | 85170 | 0 | 4254.1 | 45.055 | 98.8 | 95.9 | 95.9 | true |
| 1 | 128 | 2 | direct | 98468 | 11159 | 109627 | 0 | 5478.3 | 61.439 | 100.1 | 98.9 | 98.9 | true |
| 1 | 128 | 2 | weir | 80129 | 9092 | 89221 | 0 | 4455.9 | 45.055 | 98.8 | 95.3 | 95.3 | true |
| 1 | 128 | 3 | direct | 104365 | 11778 | 116143 | 0 | 5803.7 | 57.343 | 100.0 | 98.8 | 98.8 | true |
| 1 | 128 | 3 | weir | 74724 | 8437 | 83161 | 0 | 4154.4 | 61.439 | 99.1 | 94.4 | 94.4 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 33415 | 1.000 | 30091/3324 | 0.023 | 3.558 | 0 | 32.000 |
| 1 | 8 | 2 | 50056 | 1.000 | 45011/5045 | 0.023 | 2.061 | 0 | 32.000 |
| 1 | 8 | 3 | 63976 | 1.000 | 57529/6447 | 0.022 | 1.413 | 0 | 32.000 |
| 1 | 32 | 1 | 81354 | 1.000 | 73168/8186 | 0.193 | 4.098 | 0 | 32.000 |
| 1 | 32 | 2 | 82319 | 1.000 | 74037/8282 | 0.191 | 3.924 | 0 | 32.000 |
| 1 | 32 | 3 | 84641 | 1.000 | 76120/8521 | 0.192 | 3.808 | 0 | 32.000 |
| 1 | 128 | 1 | 85170 | 1.000 | 76538/8632 | 17.310 | 5.927 | 0 | 32.000 |
| 1 | 128 | 2 | 89221 | 1.000 | 80129/9092 | 16.350 | 5.679 | 0 | 32.000 |
| 1 | 128 | 3 | 83161 | 1.000 | 74724/8437 | 18.101 | 6.316 | 0 | 32.000 |

**Batch 1 at demonstrated database CPU saturation:** direct 5156.2 logical ops/s (32 workers), Weir 4137.4 logical ops/s (32 workers), Weir/direct 0.8024, change -19.76%.

Best verified business QPS within this ladder: direct 5569.5 (128 workers), Weir 4288.2 (128 workers), observed change -23.01%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 3277.1 | 2457.1 | -25.02% | 1.279 / 3.583 / 53.247 | 2.303 / 5.631 / 45.055 |
| 32 | 5156.2 | 4137.4 | -19.76% | 3.839 / 36.863 / 53.247 | 7.167 / 14.335 / 26.623 |
| 128 | 5569.5 | 4288.2 | -23.01% | 15.359 / 61.439 / 73.727 | 28.671 / 49.151 / 73.727 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
