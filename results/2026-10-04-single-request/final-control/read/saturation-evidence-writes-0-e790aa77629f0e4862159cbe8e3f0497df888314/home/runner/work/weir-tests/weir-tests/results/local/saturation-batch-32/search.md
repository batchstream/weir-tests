# Database saturation comparison

Backend **search**. Started 2026-10-04T00:39:54.059611758Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 102172 | 0 | 102172 | 0 | 5108.4 | 1.919 | 99.9 | 94.9 | 94.9 | true |
| 1 | 8 | 1 | weir | 105670 | 0 | 105670 | 0 | 5283.1 | 2.815 | 78.8 | 0.0 | 94.7 | true |
| 1 | 8 | 2 | direct | 185745 | 0 | 185745 | 0 | 9286.9 | 1.151 | 99.8 | 99.6 | 99.6 | true |
| 1 | 8 | 2 | weir | 105021 | 0 | 105021 | 0 | 5250.7 | 2.815 | 78.1 | 0.0 | 99.7 | true |
| 1 | 8 | 3 | direct | 187639 | 0 | 187639 | 0 | 9381.6 | 1.151 | 99.8 | 99.7 | 99.7 | true |
| 1 | 8 | 3 | weir | 104958 | 0 | 104958 | 0 | 5247.6 | 2.815 | 72.4 | 0.0 | 99.6 | true |
| 1 | 32 | 1 | direct | 239769 | 0 | 239769 | 0 | 11987.6 | 4.095 | 99.9 | 99.6 | 99.6 | true |
| 1 | 32 | 1 | weir | 162470 | 0 | 162470 | 0 | 8122.8 | 7.679 | 75.0 | 0.0 | 94.9 | true |
| 1 | 32 | 2 | direct | 238928 | 0 | 238928 | 0 | 11945.1 | 4.095 | 99.6 | 99.6 | 99.6 | true |
| 1 | 32 | 2 | weir | 167295 | 0 | 167295 | 0 | 8364.0 | 7.167 | 77.4 | 0.0 | 99.7 | true |
| 1 | 32 | 3 | direct | 239516 | 0 | 239516 | 0 | 11974.8 | 4.095 | 99.8 | 99.6 | 99.6 | true |
| 1 | 32 | 3 | weir | 164280 | 0 | 164280 | 0 | 8213.0 | 7.167 | 76.7 | 0.0 | 95.0 | true |
| 1 | 128 | 1 | direct | 264005 | 0 | 264005 | 0 | 13198.3 | 40.959 | 100.0 | 96.3 | 96.3 | true |
| 1 | 128 | 1 | weir | 255929 | 0 | 255929 | 0 | 12794.0 | 18.431 | 65.8 | 0.0 | 94.9 | true |
| 1 | 128 | 2 | direct | 255552 | 0 | 255552 | 0 | 12775.0 | 45.055 | 100.0 | 95.1 | 95.1 | true |
| 1 | 128 | 2 | weir | 255668 | 0 | 255668 | 0 | 12779.2 | 18.431 | 66.0 | 0.0 | 94.9 | true |
| 1 | 128 | 3 | direct | 260256 | 0 | 260256 | 0 | 13009.4 | 40.959 | 100.1 | 95.2 | 95.2 | true |
| 1 | 128 | 3 | weir | 250586 | 0 | 250586 | 0 | 12524.6 | 20.479 | 64.5 | 0.0 | 99.9 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 95362 | 1.108 | 105670/0 | 0.014 | 0.784 | 0 | 32.000 |
| 1 | 8 | 2 | 94936 | 1.106 | 105021/0 | 0.014 | 0.786 | 0 | 32.000 |
| 1 | 8 | 3 | 94767 | 1.108 | 104958/0 | 0.014 | 0.751 | 0 | 32.000 |
| 1 | 32 | 1 | 103846 | 1.565 | 162470/0 | 0.078 | 1.630 | 0 | 32.000 |
| 1 | 32 | 2 | 106720 | 1.568 | 167295/0 | 0.076 | 1.616 | 0 | 32.000 |
| 1 | 32 | 3 | 104986 | 1.565 | 164280/0 | 0.076 | 1.639 | 0 | 32.000 |
| 1 | 128 | 1 | 76523 | 3.344 | 255929/0 | 0.365 | 3.100 | 0 | 32.000 |
| 1 | 128 | 2 | 75921 | 3.368 | 255668/0 | 0.378 | 3.171 | 0 | 32.000 |
| 1 | 128 | 3 | 77777 | 3.222 | 250586/0 | 0.359 | 3.184 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 12994.2 (128 workers), Weir 12699.2 (128 workers), observed change -2.27%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 7925.6 | 5260.5 | -33.63% | 0.639 / 1.279 / 20.479 | 1.407 / 2.815 / 3.839 |
| 32 | 11969.2 | 8233.2 | -31.21% | 1.791 / 4.095 / 36.863 | 3.583 / 7.167 / 10.239 |
| 128 | 12994.2 | 12699.2 | -2.27% | 6.655 / 40.959 / 49.151 | 10.239 / 18.431 / 24.575 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
