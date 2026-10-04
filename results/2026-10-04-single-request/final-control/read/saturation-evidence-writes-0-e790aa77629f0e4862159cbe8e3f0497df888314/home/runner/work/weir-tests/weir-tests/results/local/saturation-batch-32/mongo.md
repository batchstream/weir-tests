# Database saturation comparison

Backend **mongo**. Started 2026-10-04T00:30:43.712329639Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 208533 | 0 | 208533 | 0 | 10426.2 | 1.023 | 99.9 | 99.7 | 99.7 | true |
| 1 | 8 | 1 | weir | 109808 | 0 | 109808 | 0 | 5490.0 | 2.815 | 78.5 | 0.0 | 94.8 | true |
| 1 | 8 | 2 | direct | 205403 | 0 | 205403 | 0 | 10269.8 | 1.151 | 99.9 | 99.8 | 99.8 | true |
| 1 | 8 | 2 | weir | 109982 | 0 | 109982 | 0 | 5498.8 | 2.815 | 79.7 | 0.0 | 99.9 | true |
| 1 | 8 | 3 | direct | 206683 | 0 | 206683 | 0 | 10333.7 | 1.023 | 100.0 | 99.6 | 99.6 | true |
| 1 | 8 | 3 | weir | 111394 | 0 | 111394 | 0 | 5569.3 | 2.815 | 80.1 | 0.0 | 99.6 | true |
| 1 | 32 | 1 | direct | 264481 | 0 | 264481 | 0 | 13223.0 | 4.607 | 100.1 | 95.3 | 95.3 | true |
| 1 | 32 | 1 | weir | 171867 | 0 | 171867 | 0 | 8592.4 | 7.167 | 82.3 | 0.0 | 94.9 | true |
| 1 | 32 | 2 | direct | 258072 | 0 | 258072 | 0 | 12902.1 | 4.607 | 100.0 | 95.0 | 95.0 | true |
| 1 | 32 | 2 | weir | 173928 | 0 | 173928 | 0 | 8695.3 | 7.167 | 82.5 | 0.0 | 94.7 | true |
| 1 | 32 | 3 | direct | 265005 | 0 | 265005 | 0 | 13249.2 | 4.607 | 100.1 | 94.8 | 94.8 | true |
| 1 | 32 | 3 | weir | 177336 | 0 | 177336 | 0 | 8865.5 | 7.167 | 83.0 | 0.0 | 99.9 | true |
| 1 | 128 | 1 | direct | 286977 | 0 | 286977 | 0 | 14344.7 | 45.055 | 100.2 | 95.2 | 95.2 | true |
| 1 | 128 | 1 | weir | 292030 | 0 | 292030 | 0 | 14595.8 | 18.431 | 65.3 | 0.0 | 100.0 | true |
| 1 | 128 | 2 | direct | 291443 | 0 | 291443 | 0 | 14541.4 | 45.055 | 100.1 | 96.0 | 96.0 | true |
| 1 | 128 | 2 | weir | 296055 | 0 | 296055 | 0 | 14800.1 | 18.431 | 65.3 | 0.0 | 99.8 | true |
| 1 | 128 | 3 | direct | 297816 | 0 | 297816 | 0 | 14888.2 | 45.055 | 100.1 | 95.7 | 95.7 | true |
| 1 | 128 | 3 | weir | 296734 | 0 | 296734 | 0 | 14832.3 | 18.431 | 65.9 | 0.0 | 100.0 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 97708 | 1.124 | 109808/0 | 0.017 | 0.632 | 0 | 32.000 |
| 1 | 8 | 2 | 97473 | 1.128 | 109982/0 | 0.018 | 0.634 | 0 | 32.000 |
| 1 | 8 | 3 | 98779 | 1.128 | 111394/0 | 0.017 | 0.626 | 0 | 32.000 |
| 1 | 32 | 1 | 100017 | 1.718 | 171867/0 | 0.094 | 1.218 | 0 | 32.000 |
| 1 | 32 | 2 | 99857 | 1.742 | 173928/0 | 0.094 | 1.198 | 0 | 32.000 |
| 1 | 32 | 3 | 101887 | 1.741 | 177336/0 | 0.094 | 1.171 | 0 | 32.000 |
| 1 | 128 | 1 | 74932 | 3.897 | 292030/0 | 0.361 | 2.050 | 0 | 32.000 |
| 1 | 128 | 2 | 74203 | 3.990 | 296055/0 | 0.357 | 1.986 | 0 | 32.000 |
| 1 | 128 | 3 | 74294 | 3.994 | 296734/0 | 0.364 | 1.999 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 14591.4 (128 workers), Weir 14742.7 (128 workers), observed change +1.04%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 10343.3 | 5519.4 | -46.64% | 0.479 / 1.023 / 2.303 | 1.407 / 2.815 / 3.583 |
| 32 | 13124.8 | 8717.8 | -33.58% | 1.279 / 4.607 / 40.959 | 3.583 / 7.167 / 9.215 |
| 128 | 14591.4 | 14742.7 | +1.04% | 5.119 / 45.055 / 53.247 | 8.191 / 18.431 / 22.527 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
