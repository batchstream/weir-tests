# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:58:48.484880993Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 124268 | 0 | 124268 | 0 | 6213.1 | 1.791 | 99.9 | 98.5 | 98.5 | true |
| 1 | 8 | 1 | weir | 72991 | 0 | 72991 | 0 | 3649.1 | 4.607 | 65.1 | 0.0 | 94.5 | true |
| 1 | 8 | 2 | direct | 124381 | 0 | 124381 | 0 | 6218.7 | 1.919 | 99.9 | 99.0 | 99.0 | true |
| 1 | 8 | 2 | weir | 79741 | 0 | 79741 | 0 | 3986.6 | 3.583 | 71.4 | 0.0 | 99.9 | true |
| 1 | 8 | 3 | direct | 123484 | 0 | 123484 | 0 | 6173.9 | 1.791 | 99.8 | 98.6 | 98.6 | true |
| 1 | 8 | 3 | weir | 79758 | 0 | 79758 | 0 | 3987.5 | 3.583 | 71.7 | 0.0 | 99.7 | true |
| 1 | 32 | 1 | direct | 130197 | 0 | 130197 | 0 | 6496.9 | 18.431 | 100.1 | 98.0 | 98.0 | true |
| 1 | 32 | 1 | weir | 98669 | 0 | 98669 | 0 | 4932.4 | 12.287 | 85.6 | 0.0 | 94.9 | true |
| 1 | 32 | 2 | direct | 130569 | 0 | 130569 | 0 | 6527.0 | 18.431 | 100.1 | 98.5 | 98.5 | true |
| 1 | 32 | 2 | weir | 98528 | 0 | 98528 | 0 | 4925.0 | 12.287 | 85.6 | 0.0 | 99.7 | true |
| 1 | 32 | 3 | direct | 131763 | 0 | 131763 | 0 | 6587.1 | 18.431 | 100.1 | 98.3 | 98.3 | true |
| 1 | 32 | 3 | weir | 97128 | 0 | 97128 | 0 | 4855.3 | 12.287 | 85.2 | 0.0 | 95.3 | true |
| 1 | 128 | 1 | direct | 128407 | 0 | 128407 | 0 | 6417.2 | 57.343 | 100.1 | 98.9 | 98.9 | true |
| 1 | 128 | 1 | weir | 102847 | 0 | 102847 | 0 | 5138.2 | 36.863 | 89.3 | 16.9 | 95.7 | true |
| 1 | 128 | 2 | direct | 129095 | 0 | 129095 | 0 | 6450.9 | 57.343 | 100.3 | 99.3 | 99.3 | true |
| 1 | 128 | 2 | weir | 106401 | 0 | 106401 | 0 | 5315.4 | 36.863 | 91.5 | 89.8 | 95.4 | true |
| 1 | 128 | 3 | direct | 124511 | 0 | 124511 | 0 | 6221.7 | 57.343 | 100.1 | 99.5 | 99.5 | true |
| 1 | 128 | 3 | weir | 105215 | 0 | 105215 | 0 | 5255.9 | 36.863 | 90.5 | 66.8 | 94.9 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 72991 | 1.000 | 72991/0 | 0.030 | 0.894 | 0 | 32.000 |
| 1 | 8 | 2 | 79741 | 1.000 | 79741/0 | 0.028 | 0.838 | 0 | 32.000 |
| 1 | 8 | 3 | 79758 | 1.000 | 79758/0 | 0.028 | 0.839 | 0 | 32.000 |
| 1 | 32 | 1 | 98669 | 1.000 | 98669/0 | 0.289 | 2.123 | 0 | 32.000 |
| 1 | 32 | 2 | 98528 | 1.000 | 98528/0 | 0.286 | 2.137 | 0 | 32.000 |
| 1 | 32 | 3 | 97128 | 1.000 | 97128/0 | 0.295 | 2.142 | 0 | 32.000 |
| 1 | 128 | 1 | 102847 | 1.000 | 102847/0 | 12.547 | 4.022 | 0 | 32.000 |
| 1 | 128 | 2 | 106401 | 1.000 | 106401/0 | 12.311 | 3.829 | 0 | 32.000 |
| 1 | 128 | 3 | 105215 | 1.000 | 105215/0 | 12.160 | 3.957 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 6537.0 (32 workers), Weir 5236.5 (128 workers), observed change -19.89%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 6201.9 | 3874.4 | -37.53% | 0.831 / 1.791 / 30.719 | 1.919 / 3.839 / 5.631 |
| 32 | 6537.0 | 4904.2 | -24.98% | 3.071 / 18.431 / 45.055 | 6.143 / 12.287 / 16.383 |
| 128 | 6363.2 | 5236.5 | -17.71% | 14.335 / 57.343 / 65.535 | 24.575 / 36.863 / 45.055 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
