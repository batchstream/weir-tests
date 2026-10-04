# Database saturation comparison

Backend **search**. Started 2026-10-04T00:10:05.219377619Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 12723 | 12723 | 0 | 636.0 | 57.343 | 100.1 | 98.3 | 98.3 | true |
| 1 | 8 | 1 | weir | 0 | 17995 | 17995 | 0 | 899.5 | 45.055 | 96.4 | 76.9 | 98.9 | true |
| 1 | 8 | 2 | direct | 0 | 33029 | 33029 | 0 | 1648.7 | 6.143 | 75.6 | 5.3 | 95.1 | true |
| 1 | 8 | 2 | weir | 0 | 28357 | 28357 | 0 | 1417.6 | 7.679 | 77.6 | 10.7 | 96.8 | true |
| 1 | 8 | 3 | direct | 0 | 33524 | 33524 | 0 | 1675.9 | 6.655 | 73.3 | 5.3 | 95.5 | true |
| 1 | 8 | 3 | weir | 0 | 32050 | 32050 | 0 | 1602.2 | 6.655 | 71.6 | 0.0 | 96.8 | true |
| 1 | 32 | 1 | direct | 0 | 37098 | 37098 | 0 | 1853.4 | 20.479 | 73.1 | 0.0 | 95.3 | true |
| 1 | 32 | 1 | weir | 0 | 32019 | 32019 | 0 | 1599.6 | 24.575 | 72.5 | 0.0 | 96.4 | true |
| 1 | 32 | 2 | direct | 0 | 38061 | 38061 | 0 | 1901.3 | 20.479 | 72.4 | 0.0 | 95.2 | true |
| 1 | 32 | 2 | weir | 0 | 32129 | 32129 | 0 | 1605.0 | 24.575 | 71.7 | 0.0 | 96.3 | true |
| 1 | 32 | 3 | direct | 0 | 36991 | 36991 | 0 | 1848.1 | 20.479 | 70.4 | 0.0 | 95.1 | true |
| 1 | 32 | 3 | weir | 0 | 31909 | 31909 | 0 | 1594.1 | 24.575 | 69.4 | 0.0 | 96.5 | true |
| 1 | 128 | 1 | direct | 0 | 40045 | 40045 | 0 | 1996.2 | 73.727 | 72.5 | 0.0 | 94.9 | true |
| 1 | 128 | 1 | weir | 0 | 100567 | 100567 | 0 | 5022.5 | 32.767 | 75.0 | 5.4 | 98.6 | true |
| 1 | 128 | 2 | direct | 0 | 40439 | 40439 | 0 | 2016.2 | 73.727 | 72.0 | 0.0 | 95.1 | true |
| 1 | 128 | 2 | weir | 0 | 101305 | 101305 | 0 | 5060.2 | 32.767 | 75.7 | 11.1 | 98.8 | true |
| 1 | 128 | 3 | direct | 0 | 41056 | 41056 | 0 | 2046.6 | 73.727 | 73.2 | 0.0 | 94.9 | true |
| 1 | 128 | 3 | weir | 0 | 101345 | 101345 | 0 | 5061.7 | 30.719 | 73.0 | 0.0 | 98.6 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 17886 | 1.006 | 0/17995 | 0.012 | 8.364 | 0 | 32.000 |
| 1 | 8 | 2 | 28235 | 1.004 | 0/28357 | 0.010 | 5.180 | 0 | 32.000 |
| 1 | 8 | 3 | 31918 | 1.004 | 0/32050 | 0.010 | 4.548 | 0 | 32.000 |
| 1 | 32 | 1 | 31880 | 1.004 | 0/32019 | 0.012 | 19.529 | 0 | 32.000 |
| 1 | 32 | 2 | 31989 | 1.004 | 0/32129 | 0.012 | 19.467 | 0 | 32.000 |
| 1 | 32 | 3 | 31774 | 1.004 | 0/31909 | 0.012 | 19.620 | 0 | 32.000 |
| 1 | 128 | 1 | 27623 | 3.641 | 0/100567 | 0.505 | 22.878 | 0 | 32.000 |
| 1 | 128 | 2 | 28111 | 3.604 | 0/101305 | 0.565 | 22.412 | 0 | 32.000 |
| 1 | 128 | 3 | 27983 | 3.622 | 0/101345 | 0.476 | 22.566 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 2019.7 (128 workers), Weir 5048.1 (128 workers), observed change +149.95%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 1320.3 | 1306.4 | -1.05% | 4.607 / 8.191 / 57.343 | 5.119 / 8.191 / 49.151 |
| 32 | 1867.6 | 1599.6 | -14.35% | 18.431 / 20.479 / 26.623 | 20.479 / 24.575 / 30.719 |
| 128 | 2019.7 | 5048.1 | +149.95% | 65.535 / 73.727 / 114.687 | 24.575 / 32.767 / 65.535 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
