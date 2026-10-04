# Database saturation comparison

Backend **search**. Started 2026-10-04T00:40:11.75401268Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 13400 | 13400 | 0 | 669.8 | 57.343 | 100.1 | 98.4 | 98.4 | true |
| 1 | 8 | 1 | weir | 0 | 19317 | 19317 | 0 | 965.6 | 45.055 | 95.7 | 75.8 | 97.6 | true |
| 1 | 8 | 2 | direct | 0 | 30518 | 30518 | 0 | 1525.5 | 6.655 | 75.4 | 5.5 | 96.0 | true |
| 1 | 8 | 2 | weir | 0 | 28609 | 28609 | 0 | 1430.1 | 7.167 | 73.1 | 5.4 | 96.2 | true |
| 1 | 8 | 3 | direct | 0 | 32461 | 32461 | 0 | 1622.7 | 6.143 | 71.6 | 5.2 | 95.3 | true |
| 1 | 8 | 3 | weir | 0 | 30145 | 30145 | 0 | 1506.9 | 6.655 | 70.3 | 0.0 | 96.2 | true |
| 1 | 32 | 1 | direct | 0 | 34389 | 34389 | 0 | 1718.1 | 24.575 | 67.9 | 0.0 | 95.1 | true |
| 1 | 32 | 1 | weir | 0 | 30220 | 30220 | 0 | 1509.7 | 28.671 | 68.6 | 0.0 | 96.2 | true |
| 1 | 32 | 2 | direct | 0 | 35135 | 35135 | 0 | 1755.0 | 22.527 | 67.5 | 0.0 | 95.0 | true |
| 1 | 32 | 2 | weir | 0 | 30297 | 30297 | 0 | 1513.4 | 26.623 | 67.1 | 0.0 | 96.1 | true |
| 1 | 32 | 3 | direct | 0 | 35916 | 35916 | 0 | 1794.4 | 22.527 | 66.9 | 0.0 | 95.0 | true |
| 1 | 32 | 3 | weir | 0 | 30888 | 30888 | 0 | 1543.0 | 24.575 | 65.4 | 0.0 | 96.1 | true |
| 1 | 128 | 1 | direct | 0 | 38687 | 38687 | 0 | 1927.9 | 81.919 | 68.1 | 0.0 | 94.9 | true |
| 1 | 128 | 1 | weir | 0 | 95759 | 95759 | 0 | 4783.5 | 36.863 | 69.2 | 5.4 | 98.8 | true |
| 1 | 128 | 2 | direct | 0 | 38547 | 38547 | 0 | 1921.7 | 81.919 | 68.3 | 0.0 | 94.9 | true |
| 1 | 128 | 2 | weir | 0 | 97377 | 97377 | 0 | 4863.4 | 36.863 | 72.2 | 5.3 | 98.3 | true |
| 1 | 128 | 3 | direct | 0 | 39009 | 39009 | 0 | 1944.7 | 81.919 | 68.0 | 0.0 | 94.9 | true |
| 1 | 128 | 3 | weir | 0 | 96695 | 96695 | 0 | 4829.9 | 36.863 | 69.5 | 0.0 | 98.9 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 19212 | 1.005 | 0/19317 | 0.011 | 7.783 | 0 | 32.000 |
| 1 | 8 | 2 | 28516 | 1.003 | 0/28609 | 0.010 | 5.158 | 0 | 32.000 |
| 1 | 8 | 3 | 30085 | 1.002 | 0/30145 | 0.009 | 4.892 | 0 | 32.000 |
| 1 | 32 | 1 | 30098 | 1.004 | 0/30220 | 0.012 | 20.768 | 0 | 32.000 |
| 1 | 32 | 2 | 30162 | 1.004 | 0/30297 | 0.012 | 20.725 | 0 | 32.000 |
| 1 | 32 | 3 | 30818 | 1.002 | 0/30888 | 0.012 | 20.305 | 0 | 32.000 |
| 1 | 128 | 1 | 26341 | 3.635 | 0/95759 | 0.511 | 23.994 | 0 | 32.000 |
| 1 | 128 | 2 | 26842 | 3.628 | 0/97377 | 0.517 | 23.505 | 0 | 32.000 |
| 1 | 128 | 3 | 26477 | 3.652 | 0/96695 | 0.506 | 23.866 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 1931.4 (128 workers), Weir 4825.6 (128 workers), observed change +149.85%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 1272.7 | 1300.9 | +2.21% | 4.607 / 8.191 / 57.343 | 5.119 / 8.191 / 49.151 |
| 32 | 1755.8 | 1522.0 | -13.32% | 18.431 / 22.527 / 32.767 | 20.479 / 26.623 / 36.863 |
| 128 | 1931.4 | 4825.6 | +149.85% | 65.535 / 81.919 / 98.303 | 26.623 / 36.863 / 65.535 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
