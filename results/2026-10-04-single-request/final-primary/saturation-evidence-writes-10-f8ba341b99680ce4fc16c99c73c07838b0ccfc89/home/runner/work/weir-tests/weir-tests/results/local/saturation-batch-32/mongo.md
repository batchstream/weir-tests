# Database saturation comparison

Backend **mongo**. Started 2026-10-04T00:00:56.86430191Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 91151 | 10229 | 101380 | 0 | 5068.7 | 2.303 | 99.6 | 99.4 | 99.4 | true |
| 1 | 8 | 1 | weir | 59504 | 6684 | 66188 | 0 | 3309.1 | 5.119 | 67.0 | 0.0 | 94.9 | true |
| 1 | 8 | 2 | direct | 92276 | 10347 | 102623 | 0 | 5130.9 | 2.303 | 99.8 | 97.8 | 97.8 | true |
| 1 | 8 | 2 | weir | 67852 | 7586 | 75438 | 0 | 3771.6 | 4.095 | 77.8 | 0.0 | 100.0 | true |
| 1 | 8 | 3 | direct | 91612 | 10295 | 101907 | 0 | 5095.0 | 2.303 | 99.5 | 98.5 | 98.5 | true |
| 1 | 8 | 3 | weir | 68496 | 7667 | 76163 | 0 | 3807.7 | 4.095 | 77.7 | 0.0 | 99.7 | true |
| 1 | 32 | 1 | direct | 98288 | 11015 | 109303 | 0 | 5464.5 | 40.959 | 100.0 | 98.5 | 98.5 | true |
| 1 | 32 | 1 | weir | 97372 | 10917 | 108289 | 0 | 5413.8 | 12.287 | 84.7 | 0.0 | 95.1 | true |
| 1 | 32 | 2 | direct | 96932 | 10883 | 107815 | 0 | 5378.9 | 40.959 | 100.0 | 98.5 | 98.5 | true |
| 1 | 32 | 2 | weir | 96904 | 10847 | 107751 | 0 | 5386.8 | 12.287 | 84.5 | 0.0 | 95.0 | true |
| 1 | 32 | 3 | direct | 98585 | 11026 | 109611 | 0 | 5479.4 | 40.959 | 100.0 | 98.5 | 98.5 | true |
| 1 | 32 | 3 | weir | 95747 | 10722 | 106469 | 0 | 5322.2 | 12.287 | 85.0 | 0.0 | 95.1 | true |
| 1 | 128 | 1 | direct | 94225 | 10702 | 104927 | 0 | 5243.0 | 65.535 | 100.0 | 98.7 | 98.7 | true |
| 1 | 128 | 1 | weir | 145934 | 16199 | 162133 | 0 | 8101.2 | 32.767 | 78.0 | 0.0 | 94.9 | true |
| 1 | 128 | 2 | direct | 96412 | 10946 | 107358 | 0 | 5352.8 | 65.535 | 100.1 | 98.6 | 98.6 | true |
| 1 | 128 | 2 | weir | 147573 | 16442 | 164015 | 0 | 8195.5 | 30.719 | 78.6 | 0.0 | 94.5 | true |
| 1 | 128 | 3 | direct | 93185 | 10590 | 103775 | 0 | 5185.3 | 65.535 | 100.1 | 99.4 | 99.4 | true |
| 1 | 128 | 3 | weir | 142571 | 15836 | 158407 | 0 | 7917.1 | 32.767 | 77.7 | 0.0 | 94.8 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 59917 | 1.105 | 59504/6684 | 0.028 | 1.047 | 0 | 32.000 |
| 1 | 8 | 2 | 68225 | 1.106 | 67852/7586 | 0.025 | 0.955 | 0 | 32.000 |
| 1 | 8 | 3 | 68891 | 1.106 | 68496/7667 | 0.025 | 0.947 | 0 | 32.000 |
| 1 | 32 | 1 | 66431 | 1.630 | 97372/10917 | 0.133 | 1.911 | 0 | 32.000 |
| 1 | 32 | 2 | 65128 | 1.654 | 96904/10847 | 0.139 | 1.874 | 0 | 32.000 |
| 1 | 32 | 3 | 65050 | 1.637 | 95747/10722 | 0.140 | 1.961 | 0 | 32.000 |
| 1 | 128 | 1 | 48670 | 3.331 | 145934/16199 | 0.545 | 3.675 | 0 | 32.000 |
| 1 | 128 | 2 | 47302 | 3.467 | 147573/16442 | 0.546 | 3.706 | 0 | 32.000 |
| 1 | 128 | 3 | 46412 | 3.413 | 142571/15836 | 0.557 | 3.746 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 5440.9 (32 workers), Weir 8071.3 (128 workers), observed change +48.35%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 5098.2 | 3629.4 | -28.81% | 0.895 / 2.303 / 40.959 | 2.047 / 4.607 / 6.143 |
| 32 | 5440.9 | 5374.3 | -1.22% | 3.327 / 40.959 / 53.247 | 5.631 / 12.287 / 15.359 |
| 128 | 5260.4 | 8071.3 | +53.43% | 15.359 / 65.535 / 81.919 | 15.359 / 32.767 / 45.055 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
