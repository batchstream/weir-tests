# Database saturation comparison

Backend **search**. Started 2026-10-04T00:24:15.502673188Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 18007 | 18007 | 0 | 900.2 | 36.863 | 75.7 | 31.3 | 99.1 | true |
| 1 | 8 | 1 | weir | 0 | 16498 | 16498 | 0 | 822.7 | 32.767 | 42.8 | 0.0 | 98.7 | true |
| 1 | 8 | 2 | direct | 0 | 23597 | 23597 | 0 | 1171.5 | 15.359 | 45.3 | 0.0 | 98.1 | true |
| 1 | 8 | 2 | weir | 0 | 25069 | 25069 | 0 | 1253.2 | 10.239 | 44.7 | 0.0 | 99.1 | true |
| 1 | 8 | 3 | direct | 0 | 34727 | 34727 | 0 | 1736.1 | 6.655 | 52.2 | 0.0 | 98.9 | true |
| 1 | 8 | 3 | weir | 0 | 24253 | 24253 | 0 | 1200.7 | 10.239 | 41.5 | 0.0 | 98.4 | true |
| 1 | 32 | 1 | direct | 0 | 25344 | 25344 | 0 | 1258.8 | 90.111 | 35.4 | 0.0 | 98.1 | true |
| 1 | 32 | 1 | weir | 0 | 28901 | 28901 | 0 | 1444.0 | 49.151 | 48.1 | 0.0 | 99.5 | true |
| 1 | 32 | 2 | direct | 0 | 26249 | 26249 | 0 | 1310.7 | 81.919 | 35.1 | 0.0 | 98.7 | true |
| 1 | 32 | 2 | weir | 0 | 16498 | 16498 | 0 | 822.7 | 180.223 | 26.4 | 0.0 | 98.4 | true |
| 1 | 32 | 3 | direct | 0 | 32058 | 32058 | 0 | 1595.2 | 36.863 | 41.6 | 0.0 | 98.4 | true |
| 1 | 32 | 3 | weir | 0 | 22275 | 22275 | 0 | 1112.9 | 106.495 | 34.6 | 0.0 | 99.0 | true |
| 1 | 128 | 1 | direct | 0 | 32945 | 32945 | 0 | 1611.1 | 180.223 | 41.4 | 0.0 | 96.6 | true |
| 1 | 128 | 1 | weir | 0 | 62849 | 62849 | 0 | 3123.2 | 163.839 | 34.4 | 0.0 | 98.9 | true |
| 1 | 128 | 2 | direct | 0 | 18393 | 18393 | 0 | 917.6 | 360.447 | 23.7 | 0.0 | 98.3 | true |
| 1 | 128 | 2 | weir | 0 | 79599 | 79599 | 0 | 3973.9 | 81.919 | 41.8 | 0.0 | 99.8 | true |
| 1 | 128 | 3 | direct | 0 | 30795 | 30795 | 0 | 1536.1 | 245.759 | 38.0 | 0.0 | 98.5 | true |
| 1 | 128 | 3 | weir | 0 | 100963 | 100963 | 0 | 5043.4 | 40.959 | 50.3 | 0.0 | 95.3 | true |
| 1 | 512 | 1 | direct | 0 | 30126 | 30126 | 0 | 1441.4 | 720.895 | 36.7 | 0.0 | 99.6 | true |
| 1 | 512 | 1 | weir | 0 | 195389 | 195389 | 0 | 9705.0 | 147.455 | 47.5 | 0.0 | 95.5 | true |
| 1 | 512 | 2 | direct | 0 | 31573 | 31573 | 0 | 1550.8 | 589.823 | 40.7 | 0.0 | 97.1 | true |
| 1 | 512 | 2 | weir | 0 | 173982 | 173982 | 0 | 8664.3 | 212.991 | 38.2 | 0.0 | 95.1 | true |
| 1 | 512 | 3 | direct | 0 | 33662 | 33662 | 0 | 1655.8 | 720.895 | 41.3 | 0.0 | 97.4 | true |
| 1 | 512 | 3 | weir | 0 | 216055 | 216055 | 0 | 10772.0 | 122.879 | 51.6 | 0.0 | 95.6 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 16453 | 1.003 | 0/16498 | 0.007 | 9.403 | 0 | 32.000 |
| 1 | 8 | 2 | 25013 | 1.002 | 0/25069 | 0.007 | 6.092 | 0 | 32.000 |
| 1 | 8 | 3 | 24228 | 1.001 | 0/24253 | 0.007 | 6.368 | 0 | 32.000 |
| 1 | 32 | 1 | 28779 | 1.004 | 0/28901 | 0.009 | 21.871 | 0 | 32.000 |
| 1 | 32 | 2 | 16433 | 1.004 | 0/16498 | 0.009 | 38.687 | 0 | 32.000 |
| 1 | 32 | 3 | 22212 | 1.003 | 0/22275 | 0.008 | 28.423 | 0 | 32.000 |
| 1 | 128 | 1 | 16641 | 3.777 | 0/62849 | 1.319 | 38.579 | 0 | 32.000 |
| 1 | 128 | 2 | 21070 | 3.778 | 0/79599 | 0.863 | 30.290 | 0 | 32.000 |
| 1 | 128 | 3 | 27097 | 3.726 | 0/100963 | 0.577 | 23.489 | 0 | 32.000 |
| 1 | 512 | 1 | 16037 | 12.184 | 0/195389 | 4.765 | 36.137 | 0 | 32.000 |
| 1 | 512 | 2 | 13769 | 12.636 | 0/173982 | 5.714 | 43.742 | 0 | 32.000 |
| 1 | 512 | 3 | 17660 | 12.234 | 0/216055 | 3.505 | 32.600 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 1548.3 (512 workers), Weir 9713.4 (512 workers), observed change +527.34%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 1269.0 | 1092.3 | -13.92% | 3.583 / 12.287 / 73.727 | 4.095 / 12.287 / 147.455 |
| 32 | 1388.2 | 1126.4 | -18.86% | 14.335 / 57.343 / 212.991 | 16.383 / 90.111 / 229.375 |
| 128 | 1356.6 | 4045.2 | +198.18% | 61.439 / 245.759 / 425.983 | 22.527 / 81.919 / 245.759 |
| 512 | 1548.3 | 9713.4 | +527.34% | 294.911 / 655.359 / 1048.575 | 36.863 / 180.223 / 327.679 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
