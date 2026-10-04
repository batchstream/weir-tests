# Database saturation comparison

Backend **search**. Started 2026-10-04T00:09:42.367706596Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 243742 | 0 | 243742 | 0 | 12186.8 | 0.959 | 99.8 | 96.1 | 96.1 | true |
| 1 | 8 | 1 | weir | 144192 | 0 | 144192 | 0 | 7209.1 | 2.303 | 66.6 | 0.0 | 95.9 | true |
| 1 | 8 | 2 | direct | 294213 | 0 | 294213 | 0 | 14710.3 | 0.831 | 99.8 | 95.2 | 95.2 | true |
| 1 | 8 | 2 | weir | 142076 | 0 | 142076 | 0 | 7103.6 | 2.303 | 66.3 | 0.0 | 95.7 | true |
| 1 | 8 | 3 | direct | 290208 | 0 | 290208 | 0 | 14510.0 | 0.831 | 99.7 | 95.2 | 95.2 | true |
| 1 | 8 | 3 | weir | 139097 | 0 | 139097 | 0 | 6954.6 | 2.559 | 65.7 | 0.0 | 95.7 | true |
| 1 | 32 | 1 | direct | 364497 | 0 | 364497 | 0 | 18223.4 | 3.071 | 99.6 | 96.3 | 96.3 | true |
| 1 | 32 | 1 | weir | 213022 | 0 | 213022 | 0 | 10650.1 | 6.143 | 67.1 | 0.0 | 96.0 | true |
| 1 | 32 | 2 | direct | 368678 | 0 | 368678 | 0 | 18432.9 | 3.327 | 99.8 | 96.0 | 96.0 | true |
| 1 | 32 | 2 | weir | 215341 | 0 | 215341 | 0 | 10765.9 | 6.143 | 67.2 | 0.0 | 95.9 | true |
| 1 | 32 | 3 | direct | 359787 | 0 | 359787 | 0 | 17988.3 | 3.071 | 99.6 | 96.5 | 96.5 | true |
| 1 | 32 | 3 | weir | 215459 | 0 | 215459 | 0 | 10771.2 | 6.143 | 67.4 | 0.0 | 95.8 | true |
| 1 | 128 | 1 | direct | 392442 | 0 | 392442 | 0 | 19620.1 | 26.623 | 99.9 | 97.1 | 97.1 | true |
| 1 | 128 | 1 | weir | 323311 | 0 | 323311 | 0 | 16161.8 | 15.359 | 58.9 | 0.0 | 96.0 | true |
| 1 | 128 | 2 | direct | 394274 | 0 | 394274 | 0 | 19710.3 | 28.671 | 99.9 | 96.5 | 96.5 | true |
| 1 | 128 | 2 | weir | 341006 | 0 | 341006 | 0 | 17045.9 | 14.335 | 59.4 | 0.0 | 96.4 | true |
| 1 | 128 | 3 | direct | 391241 | 0 | 391241 | 0 | 19559.1 | 26.623 | 99.8 | 97.5 | 97.5 | true |
| 1 | 128 | 3 | weir | 331767 | 0 | 331767 | 0 | 16584.7 | 15.359 | 59.0 | 0.0 | 95.6 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 132341 | 1.090 | 144192/0 | 0.009 | 0.532 | 0 | 32.000 |
| 1 | 8 | 2 | 130225 | 1.091 | 142076/0 | 0.009 | 0.543 | 0 | 32.000 |
| 1 | 8 | 3 | 127846 | 1.088 | 139097/0 | 0.009 | 0.554 | 0 | 32.000 |
| 1 | 32 | 1 | 147255 | 1.447 | 213022/0 | 0.048 | 1.193 | 0 | 32.000 |
| 1 | 32 | 2 | 149486 | 1.441 | 215341/0 | 0.047 | 1.186 | 0 | 32.000 |
| 1 | 32 | 3 | 149596 | 1.440 | 215459/0 | 0.047 | 1.191 | 0 | 32.000 |
| 1 | 128 | 1 | 110822 | 2.917 | 323311/0 | 0.266 | 2.387 | 0 | 32.000 |
| 1 | 128 | 2 | 116465 | 2.928 | 341006/0 | 0.251 | 2.262 | 0 | 32.000 |
| 1 | 128 | 3 | 113980 | 2.911 | 331767/0 | 0.266 | 2.336 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 19629.8 (128 workers), Weir 16597.4 (128 workers), observed change -15.45%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 13802.4 | 7089.1 | -48.64% | 0.447 / 0.831 / 2.815 | 0.959 / 2.303 / 3.583 |
| 32 | 18214.9 | 10729.1 | -41.10% | 1.279 / 3.071 / 22.527 | 2.815 / 6.143 / 8.191 |
| 128 | 19629.8 | 16597.4 | -15.45% | 4.607 / 28.671 / 36.863 | 7.167 / 15.359 / 20.479 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
