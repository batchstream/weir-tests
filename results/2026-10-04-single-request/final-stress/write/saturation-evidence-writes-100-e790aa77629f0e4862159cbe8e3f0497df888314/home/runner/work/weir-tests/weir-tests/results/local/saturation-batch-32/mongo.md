# Database saturation comparison

Backend **mongo**. Started 2026-10-04T00:11:55.493513816Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 51992 | 51992 | 0 | 2591.1 | 3.583 | 68.3 | 15.7 | 99.4 | true |
| 1 | 8 | 1 | weir | 0 | 42659 | 42659 | 0 | 2132.8 | 5.119 | 61.8 | 0.0 | 99.6 | true |
| 1 | 8 | 2 | direct | 0 | 65194 | 65194 | 0 | 3257.3 | 3.071 | 88.6 | 73.7 | 99.8 | true |
| 1 | 8 | 2 | weir | 0 | 48150 | 48150 | 0 | 2407.2 | 4.607 | 72.5 | 16.0 | 95.5 | true |
| 1 | 8 | 3 | direct | 0 | 56800 | 56800 | 0 | 2829.6 | 3.327 | 77.3 | 41.9 | 98.8 | true |
| 1 | 8 | 3 | weir | 0 | 45568 | 45568 | 0 | 2277.2 | 5.119 | 71.2 | 10.6 | 99.8 | true |
| 1 | 32 | 1 | direct | 0 | 43980 | 43980 | 0 | 2196.5 | 73.727 | 62.2 | 10.5 | 99.3 | true |
| 1 | 32 | 1 | weir | 0 | 60957 | 60957 | 0 | 3041.4 | 28.671 | 83.0 | 48.3 | 95.9 | true |
| 1 | 32 | 2 | direct | 0 | 63616 | 63616 | 0 | 3172.7 | 57.343 | 88.8 | 47.5 | 95.1 | true |
| 1 | 32 | 2 | weir | 0 | 51498 | 51498 | 0 | 2570.5 | 49.151 | 74.2 | 32.1 | 95.2 | true |
| 1 | 32 | 3 | direct | 0 | 52839 | 52839 | 0 | 2641.6 | 61.439 | 79.4 | 58.8 | 95.4 | true |
| 1 | 32 | 3 | weir | 0 | 39836 | 39836 | 0 | 1974.0 | 36.863 | 57.4 | 15.7 | 99.1 | true |
| 1 | 128 | 1 | direct | 0 | 57865 | 57865 | 0 | 2889.8 | 147.455 | 92.6 | 66.0 | 98.4 | true |
| 1 | 128 | 1 | weir | 0 | 61724 | 61724 | 0 | 3084.8 | 180.223 | 64.1 | 21.5 | 95.2 | true |
| 1 | 128 | 2 | direct | 0 | 59033 | 59033 | 0 | 2949.1 | 147.455 | 91.4 | 54.3 | 97.9 | true |
| 1 | 128 | 2 | weir | 0 | 92046 | 92046 | 0 | 4596.5 | 81.919 | 90.8 | 64.9 | 96.7 | true |
| 1 | 128 | 3 | direct | 0 | 57049 | 57049 | 0 | 2851.0 | 163.839 | 91.4 | 59.5 | 96.9 | true |
| 1 | 128 | 3 | weir | 0 | 77935 | 77935 | 0 | 3895.0 | 106.495 | 82.7 | 48.7 | 96.3 | true |
| 1 | 512 | 1 | direct | 0 | 59888 | 59888 | 0 | 2954.0 | 360.447 | 98.7 | 89.5 | 95.0 | true |
| 1 | 512 | 1 | weir | 0 | 132380 | 132380 | 0 | 6589.3 | 196.607 | 93.4 | 70.1 | 96.7 | true |
| 1 | 512 | 2 | direct | 0 | 57891 | 57891 | 0 | 2860.8 | 393.215 | 99.0 | 91.9 | 97.3 | true |
| 1 | 512 | 2 | weir | 0 | 123109 | 123109 | 0 | 6139.4 | 262.143 | 89.3 | 59.5 | 96.2 | true |
| 1 | 512 | 3 | direct | 0 | 58380 | 58380 | 0 | 2903.7 | 393.215 | 98.7 | 92.9 | 98.5 | true |
| 1 | 512 | 3 | weir | 0 | 141716 | 141716 | 0 | 7010.8 | 180.223 | 98.4 | 90.9 | 96.3 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 39423 | 1.082 | 0/42659 | 0.012 | 3.006 | 0 | 32.000 |
| 1 | 8 | 2 | 44678 | 1.078 | 0/48150 | 0.012 | 2.545 | 0 | 32.000 |
| 1 | 8 | 3 | 42142 | 1.081 | 0/45568 | 0.012 | 2.785 | 0 | 32.000 |
| 1 | 32 | 1 | 44760 | 1.362 | 0/60957 | 0.067 | 7.981 | 0 | 32.000 |
| 1 | 32 | 2 | 37447 | 1.375 | 0/51498 | 0.069 | 9.130 | 0 | 32.000 |
| 1 | 32 | 3 | 28800 | 1.383 | 0/39836 | 0.067 | 13.222 | 0 | 32.000 |
| 1 | 128 | 1 | 20841 | 2.962 | 0/61724 | 5.242 | 24.478 | 0 | 32.000 |
| 1 | 128 | 2 | 30840 | 2.985 | 0/92046 | 2.131 | 14.958 | 0 | 32.000 |
| 1 | 128 | 3 | 26594 | 2.931 | 0/77935 | 3.160 | 18.507 | 0 | 32.000 |
| 1 | 512 | 1 | 13600 | 9.734 | 0/132380 | 10.746 | 39.196 | 0 | 32.000 |
| 1 | 512 | 2 | 13098 | 9.399 | 0/123109 | 11.502 | 41.109 | 0 | 32.000 |
| 1 | 512 | 3 | 14925 | 9.495 | 0/141716 | 10.374 | 35.979 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 2906.2 (512 workers), Weir 6581.0 (512 workers), observed change +126.45%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 2892.4 | 2272.4 | -21.43% | 1.279 / 3.327 / 45.055 | 2.303 / 5.119 / 45.055 |
| 32 | 2670.5 | 2527.3 | -5.36% | 3.839 / 61.439 / 147.455 | 6.655 / 36.863 / 131.071 |
| 128 | 2896.6 | 3858.9 | +33.22% | 20.479 / 147.455 / 327.679 | 18.431 / 114.687 / 294.911 |
| 512 | 2906.2 | 6581.0 | +126.45% | 180.223 / 393.215 / 589.823 | 61.439 / 212.991 / 491.519 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
