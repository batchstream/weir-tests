# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:59:37.797452752Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 125488 | 0 | 125488 | 0 | 6274.1 | 1.791 | 99.8 | 98.5 | 98.5 | true |
| 1 | 8 | 1 | weir | 74490 | 0 | 74490 | 0 | 3723.8 | 4.095 | 60.9 | 0.0 | 99.8 | true |
| 1 | 8 | 2 | direct | 124681 | 0 | 124681 | 0 | 6233.7 | 1.791 | 99.9 | 99.0 | 99.0 | true |
| 1 | 8 | 2 | weir | 82211 | 0 | 82211 | 0 | 4110.2 | 3.583 | 68.3 | 0.0 | 99.3 | true |
| 1 | 8 | 3 | direct | 124014 | 0 | 124014 | 0 | 6200.5 | 1.791 | 99.9 | 98.4 | 98.4 | true |
| 1 | 8 | 3 | weir | 82332 | 0 | 82332 | 0 | 4116.0 | 3.583 | 68.6 | 0.0 | 99.8 | true |
| 1 | 32 | 1 | direct | 131661 | 0 | 131661 | 0 | 6582.1 | 16.383 | 100.0 | 98.0 | 98.0 | true |
| 1 | 32 | 1 | weir | 125923 | 0 | 125923 | 0 | 6295.2 | 10.239 | 69.7 | 0.0 | 99.5 | true |
| 1 | 32 | 2 | direct | 130714 | 0 | 130714 | 0 | 6534.6 | 18.431 | 100.1 | 98.5 | 98.5 | true |
| 1 | 32 | 2 | weir | 126705 | 0 | 126705 | 0 | 6334.2 | 10.239 | 69.5 | 0.0 | 99.2 | true |
| 1 | 32 | 3 | direct | 132651 | 0 | 132651 | 0 | 6631.7 | 16.383 | 100.2 | 98.3 | 98.3 | true |
| 1 | 32 | 3 | weir | 126311 | 0 | 126311 | 0 | 6314.6 | 10.239 | 69.7 | 0.0 | 94.6 | true |
| 1 | 128 | 1 | direct | 129356 | 0 | 129356 | 0 | 6450.0 | 57.343 | 100.1 | 99.0 | 99.0 | true |
| 1 | 128 | 1 | weir | 199980 | 0 | 199980 | 0 | 9994.8 | 26.623 | 55.7 | 0.0 | 99.4 | true |
| 1 | 128 | 2 | direct | 129397 | 0 | 129397 | 0 | 6466.0 | 57.343 | 100.1 | 99.4 | 99.4 | true |
| 1 | 128 | 2 | weir | 203216 | 0 | 203216 | 0 | 10157.1 | 26.623 | 55.7 | 0.0 | 99.6 | true |
| 1 | 128 | 3 | direct | 128115 | 0 | 128115 | 0 | 6401.9 | 57.343 | 100.1 | 99.3 | 99.3 | true |
| 1 | 128 | 3 | weir | 200942 | 0 | 200942 | 0 | 10044.3 | 26.623 | 55.3 | 0.0 | 99.2 | true |
| 1 | 512 | 1 | direct | 123671 | 0 | 123671 | 0 | 6153.7 | 147.455 | 100.3 | 99.7 | 99.7 | true |
| 1 | 512 | 1 | weir | 290909 | 0 | 290909 | 0 | 14528.1 | 73.727 | 38.8 | 0.0 | 99.4 | true |
| 1 | 512 | 2 | direct | 126573 | 0 | 126573 | 0 | 6300.3 | 131.071 | 100.2 | 99.1 | 99.1 | true |
| 1 | 512 | 2 | weir | 290581 | 0 | 290581 | 0 | 14513.9 | 73.727 | 38.2 | 0.0 | 99.3 | true |
| 1 | 512 | 3 | direct | 120923 | 0 | 120923 | 0 | 6017.1 | 163.839 | 100.3 | 94.8 | 94.8 | true |
| 1 | 512 | 3 | weir | 292417 | 0 | 292417 | 0 | 14611.1 | 73.727 | 37.6 | 0.0 | 99.2 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 66950 | 1.113 | 74490/0 | 0.027 | 0.885 | 0 | 32.000 |
| 1 | 8 | 2 | 73482 | 1.119 | 82211/0 | 0.025 | 0.821 | 0 | 32.000 |
| 1 | 8 | 3 | 73444 | 1.121 | 82332/0 | 0.026 | 0.819 | 0 | 32.000 |
| 1 | 32 | 1 | 71985 | 1.749 | 125923/0 | 0.139 | 1.570 | 0 | 32.000 |
| 1 | 32 | 2 | 71679 | 1.768 | 126705/0 | 0.140 | 1.556 | 0 | 32.000 |
| 1 | 32 | 3 | 72043 | 1.753 | 126311/0 | 0.138 | 1.555 | 0 | 32.000 |
| 1 | 128 | 1 | 52056 | 3.842 | 199980/0 | 0.519 | 2.751 | 0 | 32.000 |
| 1 | 128 | 2 | 51884 | 3.917 | 203216/0 | 0.517 | 2.740 | 0 | 32.000 |
| 1 | 128 | 3 | 51666 | 3.889 | 200942/0 | 0.517 | 2.778 | 0 | 32.000 |
| 1 | 512 | 1 | 27974 | 10.399 | 290909/0 | 2.121 | 5.260 | 0 | 32.000 |
| 1 | 512 | 2 | 26873 | 10.813 | 290581/0 | 2.252 | 5.472 | 0 | 32.000 |
| 1 | 512 | 3 | 25470 | 11.481 | 292417/0 | 2.286 | 5.682 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 6582.8 (32 workers), Weir 14551.0 (512 workers), observed change +121.05%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 6236.1 | 3983.3 | -36.13% | 0.831 / 1.791 / 30.719 | 1.919 / 3.839 / 5.119 |
| 32 | 6582.8 | 6314.7 | -4.07% | 3.071 / 16.383 / 45.055 | 5.119 / 10.239 / 13.311 |
| 128 | 6439.3 | 10065.4 | +56.31% | 13.311 / 57.343 / 65.535 | 12.287 / 26.623 / 36.863 |
| 512 | 6157.0 | 14551.0 | +136.33% | 90.111 / 147.455 / 180.223 | 32.767 / 73.727 / 90.111 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
