# Database saturation comparison

Backend **search**. Started 2026-10-03T23:07:59.966610313Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 43872 | 0 | 43872 | 0 | 2188.8 | 3.839 | 100.1 | 98.5 | 98.5 | true |
| 1 | 8 | 1 | weir | 71401 | 0 | 71401 | 0 | 3569.7 | 4.095 | 82.0 | 0.0 | 99.2 | true |
| 1 | 8 | 2 | direct | 113115 | 0 | 113115 | 0 | 5655.5 | 1.791 | 99.6 | 97.7 | 97.7 | true |
| 1 | 8 | 2 | weir | 73581 | 0 | 73581 | 0 | 3678.8 | 3.839 | 79.7 | 0.0 | 99.2 | true |
| 1 | 8 | 3 | direct | 112913 | 0 | 112913 | 0 | 5645.4 | 1.791 | 99.7 | 97.2 | 97.2 | true |
| 1 | 8 | 3 | weir | 73431 | 0 | 73431 | 0 | 3671.0 | 3.839 | 80.0 | 0.0 | 99.3 | true |
| 1 | 32 | 1 | direct | 138719 | 0 | 138719 | 0 | 6935.2 | 10.239 | 99.8 | 98.9 | 98.9 | true |
| 1 | 32 | 1 | weir | 93285 | 0 | 93285 | 0 | 4663.3 | 13.311 | 87.6 | 5.6 | 95.3 | true |
| 1 | 32 | 2 | direct | 139283 | 0 | 139283 | 0 | 6963.2 | 10.239 | 99.7 | 98.5 | 98.5 | true |
| 1 | 32 | 2 | weir | 93752 | 0 | 93752 | 0 | 4687.0 | 12.287 | 88.0 | 0.0 | 94.7 | true |
| 1 | 32 | 3 | direct | 138640 | 0 | 138640 | 0 | 6931.2 | 9.215 | 99.9 | 98.6 | 98.6 | true |
| 1 | 32 | 3 | weir | 95816 | 0 | 95816 | 0 | 4789.3 | 12.287 | 87.8 | 0.0 | 94.6 | true |
| 1 | 128 | 1 | direct | 149056 | 0 | 149056 | 0 | 7449.0 | 45.055 | 100.0 | 98.8 | 98.8 | true |
| 1 | 128 | 1 | weir | 100959 | 0 | 100959 | 0 | 5043.6 | 36.863 | 91.6 | 83.9 | 94.7 | true |
| 1 | 128 | 2 | direct | 147595 | 0 | 147595 | 0 | 7376.6 | 49.151 | 100.0 | 98.9 | 98.9 | true |
| 1 | 128 | 2 | weir | 100594 | 0 | 100594 | 0 | 5025.3 | 36.863 | 90.2 | 55.5 | 94.8 | true |
| 1 | 128 | 3 | direct | 151379 | 0 | 151379 | 0 | 7565.3 | 45.055 | 100.0 | 98.9 | 98.9 | true |
| 1 | 128 | 3 | weir | 99542 | 0 | 99542 | 0 | 4972.6 | 36.863 | 91.1 | 73.0 | 95.6 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 71401 | 1.000 | 71401/0 | 0.024 | 1.140 | 0 | 32.000 |
| 1 | 8 | 2 | 73581 | 1.000 | 73581/0 | 0.022 | 1.100 | 0 | 32.000 |
| 1 | 8 | 3 | 73431 | 1.000 | 73431/0 | 0.023 | 1.098 | 0 | 32.000 |
| 1 | 32 | 1 | 93285 | 1.000 | 93285/0 | 0.200 | 3.084 | 0 | 32.000 |
| 1 | 32 | 2 | 93752 | 1.000 | 93752/0 | 0.200 | 3.098 | 0 | 32.000 |
| 1 | 32 | 3 | 95816 | 1.000 | 95816/0 | 0.190 | 3.020 | 0 | 32.000 |
| 1 | 128 | 1 | 100959 | 1.000 | 100959/0 | 13.855 | 4.896 | 0 | 32.000 |
| 1 | 128 | 2 | 100594 | 1.000 | 100594/0 | 13.914 | 4.903 | 0 | 32.000 |
| 1 | 128 | 3 | 99542 | 1.000 | 99542/0 | 14.093 | 4.948 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 7463.6 (128 workers), Weir 5013.8 (128 workers), observed change -32.82%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 4494.9 | 3639.8 | -19.02% | 1.151 / 2.303 / 26.623 | 2.047 / 3.839 / 5.631 |
| 32 | 6943.2 | 4713.2 | -32.12% | 3.327 / 10.239 / 36.863 | 6.655 / 12.287 / 15.359 |
| 128 | 7463.6 | 5013.8 | -32.82% | 13.311 / 45.055 / 53.247 | 26.623 / 36.863 / 45.055 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
