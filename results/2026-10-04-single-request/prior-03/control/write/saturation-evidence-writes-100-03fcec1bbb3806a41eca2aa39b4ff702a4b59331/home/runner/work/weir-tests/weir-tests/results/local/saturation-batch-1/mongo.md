# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:59:01.898708434Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 58939 | 58939 | 0 | 2941.2 | 2.303 | 76.6 | 36.8 | 99.5 | true |
| 1 | 8 | 1 | weir | 0 | 54877 | 54877 | 0 | 2742.9 | 4.607 | 81.8 | 26.8 | 95.8 | true |
| 1 | 8 | 2 | direct | 0 | 54298 | 54298 | 0 | 2714.7 | 3.839 | 73.7 | 26.5 | 99.8 | true |
| 1 | 8 | 2 | weir | 0 | 35958 | 35958 | 0 | 1797.8 | 5.631 | 58.3 | 5.4 | 95.6 | true |
| 1 | 8 | 3 | direct | 0 | 48170 | 48170 | 0 | 2408.4 | 4.607 | 66.6 | 5.3 | 99.2 | true |
| 1 | 8 | 3 | weir | 0 | 43346 | 43346 | 0 | 2167.1 | 5.119 | 69.0 | 0.0 | 96.2 | true |
| 1 | 32 | 1 | direct | 0 | 65021 | 65021 | 0 | 3250.7 | 57.343 | 90.7 | 53.9 | 96.2 | true |
| 1 | 32 | 1 | weir | 0 | 53326 | 53326 | 0 | 2657.4 | 40.959 | 83.4 | 32.1 | 95.9 | true |
| 1 | 32 | 2 | direct | 0 | 63137 | 63137 | 0 | 3156.5 | 61.439 | 89.2 | 70.1 | 96.4 | true |
| 1 | 32 | 2 | weir | 0 | 46505 | 46505 | 0 | 2319.1 | 57.343 | 73.9 | 26.9 | 95.5 | true |
| 1 | 32 | 3 | direct | 0 | 67449 | 67449 | 0 | 3369.8 | 57.343 | 93.9 | 74.5 | 96.0 | true |
| 1 | 32 | 3 | weir | 0 | 51821 | 51821 | 0 | 2590.5 | 57.343 | 84.8 | 43.4 | 96.6 | true |
| 1 | 128 | 1 | direct | 0 | 57447 | 57447 | 0 | 2870.0 | 180.223 | 82.3 | 37.0 | 95.4 | true |
| 1 | 128 | 1 | weir | 0 | 52307 | 52307 | 0 | 2602.6 | 122.879 | 86.2 | 48.4 | 96.3 | true |
| 1 | 128 | 2 | direct | 0 | 65707 | 65707 | 0 | 3283.8 | 106.495 | 95.0 | 76.1 | 97.4 | true |
| 1 | 128 | 2 | weir | 0 | 57902 | 57902 | 0 | 2887.1 | 98.303 | 93.4 | 85.6 | 95.9 | true |
| 1 | 128 | 3 | direct | 0 | 62904 | 62904 | 0 | 3142.9 | 106.495 | 95.3 | 82.2 | 97.9 | true |
| 1 | 128 | 3 | weir | 0 | 52691 | 52691 | 0 | 2631.4 | 131.071 | 85.8 | 48.4 | 96.3 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 54877 | 1.000 | 0/54877 | 0.013 | 2.131 | 0 | 32.000 |
| 1 | 8 | 2 | 35958 | 1.000 | 0/35958 | 0.013 | 3.661 | 0 | 32.000 |
| 1 | 8 | 3 | 43346 | 1.000 | 0/43346 | 0.013 | 2.925 | 0 | 32.000 |
| 1 | 32 | 1 | 53326 | 1.000 | 0/53326 | 0.126 | 8.908 | 0 | 32.000 |
| 1 | 32 | 2 | 46505 | 1.000 | 0/46505 | 0.116 | 10.807 | 0 | 32.000 |
| 1 | 32 | 3 | 51821 | 1.000 | 0/51821 | 0.119 | 9.278 | 0 | 32.000 |
| 1 | 128 | 1 | 52307 | 1.000 | 0/52307 | 32.943 | 10.884 | 0 | 32.000 |
| 1 | 128 | 2 | 57902 | 1.000 | 0/57902 | 29.558 | 9.754 | 0 | 32.000 |
| 1 | 128 | 3 | 52691 | 1.000 | 0/52691 | 32.465 | 10.789 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 3259.0 (32 workers), Weir 2707.0 (128 workers), observed change -16.94%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 2688.3 | 2236.0 | -16.83% | 1.279 / 3.071 / 45.055 | 2.303 / 5.119 / 40.959 |
| 32 | 3259.0 | 2522.3 | -22.60% | 3.839 / 57.343 / 90.111 | 7.167 / 53.247 / 106.495 |
| 128 | 3098.9 | 2707.0 | -12.64% | 18.431 / 122.879 / 262.143 | 32.767 / 114.687 / 294.911 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
