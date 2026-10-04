# Database saturation comparison

Backend **search**. Started 2026-10-03T23:08:15.5211979Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 17026 | 17026 | 0 | 851.1 | 53.247 | 91.0 | 68.5 | 99.9 | true |
| 1 | 8 | 1 | weir | 0 | 16034 | 16034 | 0 | 801.5 | 49.151 | 48.3 | 0.0 | 99.4 | true |
| 1 | 8 | 2 | direct | 0 | 17150 | 17150 | 0 | 851.6 | 26.623 | 34.2 | 0.0 | 98.1 | true |
| 1 | 8 | 2 | weir | 0 | 10537 | 10537 | 0 | 526.5 | 98.303 | 23.3 | 0.0 | 98.7 | true |
| 1 | 8 | 3 | direct | 0 | 25533 | 25533 | 0 | 1276.5 | 7.679 | 38.0 | 0.0 | 98.8 | true |
| 1 | 8 | 3 | weir | 0 | 32038 | 32038 | 0 | 1601.6 | 6.143 | 52.1 | 0.0 | 99.6 | true |
| 1 | 32 | 1 | direct | 0 | 28781 | 28781 | 0 | 1437.7 | 73.727 | 39.9 | 0.0 | 98.6 | true |
| 1 | 32 | 1 | weir | 0 | 32873 | 32873 | 0 | 1642.1 | 28.671 | 53.9 | 0.0 | 99.5 | true |
| 1 | 32 | 2 | direct | 0 | 22930 | 22930 | 0 | 1145.7 | 114.687 | 30.1 | 0.0 | 98.5 | true |
| 1 | 32 | 2 | weir | 0 | 31831 | 31831 | 0 | 1589.1 | 45.055 | 49.7 | 0.0 | 99.2 | true |
| 1 | 32 | 3 | direct | 0 | 30215 | 30215 | 0 | 1502.9 | 73.727 | 39.5 | 0.0 | 98.4 | true |
| 1 | 32 | 3 | weir | 0 | 21566 | 21566 | 0 | 1076.7 | 106.495 | 35.4 | 0.0 | 99.0 | true |
| 1 | 128 | 1 | direct | 0 | 26677 | 26677 | 0 | 1321.1 | 245.759 | 35.8 | 0.0 | 97.9 | true |
| 1 | 128 | 1 | weir | 0 | 28278 | 28278 | 0 | 1409.6 | 245.759 | 46.5 | 0.0 | 99.4 | true |
| 1 | 128 | 2 | direct | 0 | 28061 | 28061 | 0 | 1398.9 | 245.759 | 36.9 | 0.0 | 98.8 | true |
| 1 | 128 | 2 | weir | 0 | 25994 | 25994 | 0 | 1295.3 | 245.759 | 43.3 | 0.0 | 99.2 | true |
| 1 | 128 | 3 | direct | 0 | 30290 | 30290 | 0 | 1510.2 | 212.991 | 38.6 | 0.0 | 98.8 | true |
| 1 | 128 | 3 | weir | 0 | 23090 | 23090 | 0 | 1130.9 | 262.143 | 35.6 | 0.0 | 97.5 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 16034 | 1.000 | 0/16034 | 0.008 | 9.623 | 0 | 32.000 |
| 1 | 8 | 2 | 10537 | 1.000 | 0/10537 | 0.007 | 14.881 | 0 | 32.000 |
| 1 | 8 | 3 | 32038 | 1.000 | 0/32038 | 0.007 | 4.696 | 0 | 32.000 |
| 1 | 32 | 1 | 32873 | 1.000 | 0/32873 | 0.010 | 19.162 | 0 | 32.000 |
| 1 | 32 | 2 | 31831 | 1.000 | 0/31831 | 0.008 | 19.826 | 0 | 32.000 |
| 1 | 32 | 3 | 21566 | 1.000 | 0/21566 | 0.009 | 29.394 | 0 | 32.000 |
| 1 | 128 | 1 | 28278 | 1.000 | 0/28278 | 67.602 | 22.666 | 0 | 32.000 |
| 1 | 128 | 2 | 25994 | 1.000 | 0/25994 | 73.567 | 24.664 | 0 | 32.000 |
| 1 | 128 | 3 | 23090 | 1.000 | 0/23090 | 83.342 | 28.184 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 1409.9 (128 workers), Weir 1435.9 (32 workers), observed change +1.85%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 992.8 | 976.4 | -1.64% | 3.583 / 26.623 / 147.455 | 4.095 / 22.527 / 122.879 |
| 32 | 1362.3 | 1435.9 | +5.40% | 13.311 / 81.919 / 212.991 | 16.383 / 49.151 / 163.839 |
| 128 | 1409.9 | 1277.8 | -9.37% | 61.439 / 245.759 / 360.447 | 73.727 / 245.759 / 393.215 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
