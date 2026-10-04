# Database saturation comparison

Backend **search**. Started 2026-10-04T00:10:09.620632022Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 23163 | 2505 | 25668 | 0 | 1283.3 | 61.439 | 99.9 | 99.0 | 99.0 | true |
| 1 | 8 | 1 | weir | 29882 | 3306 | 33188 | 0 | 1659.1 | 10.239 | 100.0 | 98.9 | 98.9 | true |
| 1 | 8 | 2 | direct | 65660 | 7374 | 73034 | 0 | 3651.5 | 3.071 | 99.8 | 98.2 | 98.2 | true |
| 1 | 8 | 2 | weir | 45568 | 5106 | 50674 | 0 | 2533.5 | 5.631 | 94.9 | 93.0 | 98.4 | true |
| 1 | 8 | 3 | direct | 78910 | 8843 | 87753 | 0 | 4387.3 | 2.559 | 99.6 | 96.7 | 96.7 | true |
| 1 | 8 | 3 | weir | 50648 | 5708 | 56356 | 0 | 2817.6 | 5.119 | 92.3 | 60.0 | 98.4 | true |
| 1 | 32 | 1 | direct | 91626 | 10257 | 101883 | 0 | 5093.3 | 36.863 | 99.9 | 98.0 | 98.0 | true |
| 1 | 32 | 1 | weir | 82544 | 9221 | 91765 | 0 | 4587.5 | 13.311 | 92.5 | 77.7 | 94.5 | true |
| 1 | 32 | 2 | direct | 96070 | 10737 | 106807 | 0 | 5339.6 | 36.863 | 99.9 | 98.8 | 98.8 | true |
| 1 | 32 | 2 | weir | 84432 | 9445 | 93877 | 0 | 4692.4 | 13.311 | 91.5 | 61.3 | 94.6 | true |
| 1 | 32 | 3 | direct | 98444 | 11016 | 109460 | 0 | 5472.2 | 32.767 | 99.8 | 98.5 | 98.5 | true |
| 1 | 32 | 3 | weir | 86176 | 9677 | 95853 | 0 | 4791.3 | 13.311 | 89.7 | 33.4 | 94.7 | true |
| 1 | 128 | 1 | direct | 104238 | 11774 | 116012 | 0 | 5787.6 | 57.343 | 100.0 | 98.5 | 98.5 | true |
| 1 | 128 | 1 | weir | 129646 | 14468 | 144114 | 0 | 7201.6 | 36.863 | 82.4 | 0.0 | 94.5 | true |
| 1 | 128 | 2 | direct | 101204 | 11482 | 112686 | 0 | 5632.3 | 61.439 | 99.9 | 99.0 | 99.0 | true |
| 1 | 128 | 2 | weir | 130876 | 14609 | 145485 | 0 | 7270.5 | 32.767 | 81.5 | 0.0 | 95.0 | true |
| 1 | 128 | 3 | direct | 98924 | 11219 | 110143 | 0 | 5504.6 | 61.439 | 100.0 | 99.0 | 99.0 | true |
| 1 | 128 | 3 | weir | 126792 | 14154 | 140946 | 0 | 7043.7 | 36.863 | 83.3 | 11.0 | 94.6 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 31128 | 1.066 | 29882/3306 | 0.020 | 3.630 | 0 | 32.000 |
| 1 | 8 | 2 | 46795 | 1.083 | 45568/5106 | 0.022 | 2.022 | 0 | 32.000 |
| 1 | 8 | 3 | 52082 | 1.082 | 50648/5708 | 0.021 | 1.734 | 0 | 32.000 |
| 1 | 32 | 1 | 62558 | 1.467 | 82544/9221 | 0.110 | 3.321 | 0 | 32.000 |
| 1 | 32 | 2 | 63431 | 1.480 | 84432/9445 | 0.116 | 3.112 | 0 | 32.000 |
| 1 | 32 | 3 | 64499 | 1.486 | 86176/9677 | 0.113 | 2.967 | 0 | 32.000 |
| 1 | 128 | 1 | 48407 | 2.977 | 129646/14468 | 0.601 | 5.466 | 0 | 32.000 |
| 1 | 128 | 2 | 49218 | 2.956 | 130876/14609 | 0.558 | 5.366 | 0 | 32.000 |
| 1 | 128 | 3 | 48338 | 2.916 | 126792/14154 | 0.599 | 5.737 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 5641.6 (128 workers), Weir 7171.9 (128 workers), observed change +27.13%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 3107.3 | 2336.7 | -24.80% | 1.279 / 3.583 / 57.343 | 2.559 / 5.631 / 45.055 |
| 32 | 5301.7 | 4690.4 | -11.53% | 3.839 / 36.863 / 45.055 | 6.143 / 13.311 / 20.479 |
| 128 | 5641.6 | 7171.9 | +27.13% | 15.359 / 61.439 / 73.727 | 18.431 / 36.863 / 49.151 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
