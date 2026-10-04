# Database saturation comparison

Backend **search**. Started 2026-10-04T00:24:54.474274652Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 22791 | 2481 | 25272 | 0 | 1263.5 | 61.439 | 99.7 | 98.9 | 98.9 | true |
| 1 | 8 | 1 | weir | 28340 | 3121 | 31461 | 0 | 1572.9 | 12.287 | 99.9 | 98.9 | 98.9 | true |
| 1 | 8 | 2 | direct | 72432 | 8127 | 80559 | 0 | 4027.7 | 2.815 | 99.7 | 97.0 | 97.0 | true |
| 1 | 8 | 2 | weir | 43581 | 4891 | 48472 | 0 | 2423.4 | 5.631 | 95.7 | 99.5 | 99.5 | true |
| 1 | 8 | 3 | direct | 72292 | 8067 | 80359 | 0 | 4017.7 | 2.815 | 99.7 | 97.0 | 97.0 | true |
| 1 | 8 | 3 | weir | 50290 | 5643 | 55933 | 0 | 2796.4 | 4.607 | 89.7 | 43.8 | 98.8 | true |
| 1 | 32 | 1 | direct | 92614 | 10374 | 102988 | 0 | 5148.6 | 36.863 | 99.9 | 98.8 | 98.8 | true |
| 1 | 32 | 1 | weir | 82914 | 9292 | 92206 | 0 | 4609.4 | 13.311 | 93.0 | 94.9 | 94.9 | true |
| 1 | 32 | 2 | direct | 95397 | 10708 | 106105 | 0 | 5304.3 | 36.863 | 99.7 | 98.5 | 98.5 | true |
| 1 | 32 | 2 | weir | 83195 | 9296 | 92491 | 0 | 4623.9 | 13.311 | 92.9 | 93.7 | 99.0 | true |
| 1 | 32 | 3 | direct | 97327 | 10900 | 108227 | 0 | 5402.4 | 36.863 | 99.9 | 98.5 | 98.5 | true |
| 1 | 32 | 3 | weir | 84652 | 9502 | 94154 | 0 | 4707.0 | 13.311 | 90.7 | 50.1 | 95.0 | true |
| 1 | 128 | 1 | direct | 101099 | 11435 | 112534 | 0 | 5624.3 | 61.439 | 100.0 | 99.0 | 99.0 | true |
| 1 | 128 | 1 | weir | 128895 | 14368 | 143263 | 0 | 7159.0 | 36.863 | 83.1 | 5.7 | 95.1 | true |
| 1 | 128 | 2 | direct | 103172 | 11645 | 114817 | 0 | 5739.2 | 61.439 | 100.1 | 99.0 | 99.0 | true |
| 1 | 128 | 2 | weir | 132624 | 14794 | 147418 | 0 | 7367.8 | 32.767 | 81.7 | 0.0 | 94.6 | true |
| 1 | 128 | 3 | direct | 101657 | 11504 | 113161 | 0 | 5655.2 | 61.439 | 99.9 | 98.9 | 98.9 | true |
| 1 | 128 | 3 | weir | 126657 | 14119 | 140776 | 0 | 7036.0 | 36.863 | 82.8 | 11.2 | 95.3 | true |
| 1 | 512 | 1 | direct | 0 | 0 | 0 | 0 | 0.0 | 0.000 | 0.0 | 0.0 | 0.0 | false |
| 1 | 512 | 1 | weir | 188679 | 21165 | 209844 | 0 | 10480.9 | 90.111 | 69.4 | 0.0 | 95.1 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 29411 | 1.070 | 28340/3121 | 0.021 | 3.838 | 0 | 32.000 |
| 1 | 8 | 2 | 44899 | 1.080 | 43581/4891 | 0.022 | 2.168 | 0 | 32.000 |
| 1 | 8 | 3 | 51580 | 1.084 | 50290/5643 | 0.022 | 1.748 | 0 | 32.000 |
| 1 | 32 | 1 | 61958 | 1.488 | 82914/9292 | 0.117 | 3.231 | 0 | 32.000 |
| 1 | 32 | 2 | 62708 | 1.475 | 83195/9296 | 0.112 | 3.232 | 0 | 32.000 |
| 1 | 32 | 3 | 62767 | 1.500 | 84652/9502 | 0.119 | 3.105 | 0 | 32.000 |
| 1 | 128 | 1 | 48240 | 2.970 | 128895/14368 | 0.599 | 5.623 | 0 | 32.000 |
| 1 | 128 | 2 | 50378 | 2.926 | 132624/14794 | 0.596 | 5.248 | 0 | 32.000 |
| 1 | 128 | 3 | 47306 | 2.976 | 126657/14119 | 0.626 | 5.651 | 0 | 32.000 |
| 1 | 512 | 1 | 25457 | 8.243 | 188679/21165 | 2.373 | 11.747 | 0 | 32.000 |

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.

Incomplete: direct single-request stage failed: independent client 14467 warmup failed: [Post "http://127.0.0.1:32769/weirtest_bd90bd2994abd7f6e35d116e/_mget?realtime=true": context deadline exceeded Post "http://127.0.0.1:32769/weirtest_bd90bd2994abd7f6e35d116e/_mget?realtime=true": context deadline exceeded] exit status 1 exit status 1 exit status 1 exit status 1 
