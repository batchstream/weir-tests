# Database saturation comparison

Backend **search**. Started 2026-10-03T22:35:40.91067679Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 221075 | 0 | 221075 | 0 | 11053.5 | 1.023 | 99.7 | 96.9 | 96.9 | true |
| 1 | 8 | 1 | weir | 133943 | 0 | 133943 | 0 | 6696.5 | 2.559 | 66.5 | 0.0 | 96.1 | true |
| 1 | 8 | 2 | direct | 284287 | 0 | 284287 | 0 | 14214.0 | 0.831 | 99.6 | 95.5 | 95.5 | true |
| 1 | 8 | 2 | weir | 134231 | 0 | 134231 | 0 | 6711.3 | 2.559 | 66.1 | 0.0 | 96.7 | true |
| 1 | 8 | 3 | direct | 280972 | 0 | 280972 | 0 | 14048.2 | 0.831 | 99.8 | 95.9 | 95.9 | true |
| 1 | 8 | 3 | weir | 146485 | 0 | 146485 | 0 | 7324.0 | 2.047 | 69.0 | 0.0 | 95.7 | true |
| 1 | 32 | 1 | direct | 353621 | 0 | 353621 | 0 | 17680.1 | 3.583 | 99.7 | 96.9 | 96.9 | true |
| 1 | 32 | 1 | weir | 207007 | 0 | 207007 | 0 | 10349.4 | 6.655 | 67.4 | 0.0 | 96.4 | true |
| 1 | 32 | 2 | direct | 366794 | 0 | 366794 | 0 | 18339.0 | 3.327 | 99.5 | 96.6 | 96.6 | true |
| 1 | 32 | 2 | weir | 211105 | 0 | 211105 | 0 | 10553.9 | 6.655 | 68.7 | 0.0 | 96.6 | true |
| 1 | 32 | 3 | direct | 365860 | 0 | 365860 | 0 | 18292.2 | 3.071 | 99.4 | 96.5 | 96.5 | true |
| 1 | 32 | 3 | weir | 206655 | 0 | 206655 | 0 | 10332.0 | 6.655 | 69.0 | 0.0 | 96.8 | true |
| 1 | 128 | 1 | direct | 370060 | 0 | 370060 | 0 | 18499.2 | 28.671 | 99.9 | 96.5 | 96.5 | true |
| 1 | 128 | 1 | weir | 328577 | 0 | 328577 | 0 | 16424.0 | 15.359 | 61.3 | 0.0 | 96.2 | true |
| 1 | 128 | 2 | direct | 370107 | 0 | 370107 | 0 | 18499.3 | 28.671 | 99.9 | 97.2 | 97.2 | true |
| 1 | 128 | 2 | weir | 340980 | 0 | 340980 | 0 | 17044.5 | 14.335 | 62.6 | 0.0 | 96.0 | true |
| 1 | 128 | 3 | direct | 359101 | 0 | 359101 | 0 | 17951.4 | 28.671 | 0.0 | 0.0 | 0.0 | true |
| 1 | 128 | 3 | weir | 323707 | 0 | 323707 | 0 | 16181.2 | 15.359 | 60.4 | 0.0 | 26.8 | true |
| 1 | 512 | 1 | direct | 295506 | 0 | 295506 | 0 | 14765.3 | 65.535 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 1 | weir | 428868 | 0 | 428868 | 0 | 21430.2 | 45.055 | 46.3 | 0.0 | 96.7 | true |
| 1 | 512 | 2 | direct | 290114 | 0 | 290114 | 0 | 14497.2 | 73.727 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 2 | weir | 451863 | 0 | 451863 | 0 | 22575.7 | 45.055 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 3 | direct | 295901 | 0 | 295901 | 0 | 14763.4 | 65.535 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 3 | weir | 433643 | 0 | 433643 | 0 | 21662.0 | 45.055 | 0.0 | 0.0 | 0.0 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 122267 | 1.095 | 133943/0 | 0.010 | 0.570 | 0 | 32.000 |
| 1 | 8 | 2 | 122704 | 1.094 | 134231/0 | 0.010 | 0.574 | 0 | 32.000 |
| 1 | 8 | 3 | 133244 | 1.099 | 146485/0 | 0.010 | 0.535 | 0 | 32.000 |
| 1 | 32 | 1 | 141322 | 1.465 | 207007/0 | 0.052 | 1.208 | 0 | 32.000 |
| 1 | 32 | 2 | 143856 | 1.467 | 211105/0 | 0.051 | 1.199 | 0 | 32.000 |
| 1 | 32 | 3 | 140139 | 1.475 | 206655/0 | 0.054 | 1.217 | 0 | 32.000 |
| 1 | 128 | 1 | 104761 | 3.136 | 328577/0 | 0.297 | 2.336 | 0 | 32.000 |
| 1 | 128 | 2 | 108659 | 3.138 | 340980/0 | 0.274 | 2.250 | 0 | 32.000 |
| 1 | 128 | 3 | 102770 | 3.150 | 323707/0 | 0.293 | 2.376 | 0 | 32.000 |
| 1 | 512 | 1 | 45259 | 9.476 | 428868/0 | 1.421 | 5.353 | 0 | 32.000 |
| 1 | 512 | 2 | 49489 | 9.131 | 451863/0 | 1.290 | 5.219 | 0 | 32.000 |
| 1 | 512 | 3 | 46944 | 9.237 | 433643/0 | 1.404 | 5.350 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 18316.6 (128 workers), Weir 21889.3 (512 workers), observed change +19.51%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 13105.2 | 6910.6 | -47.27% | 0.447 / 0.895 / 3.071 | 1.023 / 2.303 / 3.583 |
| 32 | 18103.8 | 10411.7 | -42.49% | 1.279 / 3.327 / 22.527 | 2.815 / 6.655 / 9.215 |
| 128 | 18316.6 | 16549.9 | -9.65% | 5.119 / 28.671 / 36.863 | 7.679 / 15.359 / 20.479 |
| 512 | 14675.3 | 21889.3 | +49.16% | 30.719 / 73.727 / 81.919 | 22.527 / 45.055 / 57.343 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
