# Database saturation comparison

Backend **search**. Started 2026-10-03T23:10:53.028092741Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 24551 | 2673 | 27224 | 0 | 1361.0 | 57.343 | 99.8 | 97.9 | 97.9 | true |
| 1 | 8 | 1 | weir | 32179 | 3555 | 35734 | 0 | 1786.5 | 9.215 | 99.6 | 97.8 | 97.8 | true |
| 1 | 8 | 2 | direct | 74622 | 8348 | 82970 | 0 | 4148.0 | 2.815 | 99.7 | 97.0 | 97.0 | true |
| 1 | 8 | 2 | weir | 47265 | 5297 | 52562 | 0 | 2627.9 | 5.119 | 95.5 | 98.9 | 98.9 | true |
| 1 | 8 | 3 | direct | 76474 | 8537 | 85011 | 0 | 4250.2 | 2.559 | 99.5 | 97.0 | 97.0 | true |
| 1 | 8 | 3 | weir | 59236 | 6657 | 65893 | 0 | 3294.3 | 4.607 | 88.6 | 16.3 | 99.2 | true |
| 1 | 32 | 1 | direct | 95889 | 10743 | 106632 | 0 | 5330.0 | 36.863 | 99.9 | 98.9 | 98.9 | true |
| 1 | 32 | 1 | weir | 78669 | 8834 | 87503 | 0 | 4374.0 | 14.335 | 93.8 | 88.5 | 99.8 | true |
| 1 | 32 | 2 | direct | 97224 | 10895 | 108119 | 0 | 5403.1 | 36.863 | 99.8 | 98.4 | 98.4 | true |
| 1 | 32 | 2 | weir | 87111 | 9778 | 96889 | 0 | 4843.6 | 13.311 | 91.6 | 77.8 | 94.7 | true |
| 1 | 32 | 3 | direct | 90423 | 10129 | 100552 | 0 | 5026.7 | 36.863 | 99.8 | 97.8 | 97.8 | true |
| 1 | 32 | 3 | weir | 86800 | 9722 | 96522 | 0 | 4824.0 | 13.311 | 91.5 | 82.6 | 99.5 | true |
| 1 | 128 | 1 | direct | 101621 | 11508 | 113129 | 0 | 5641.9 | 61.439 | 100.1 | 98.6 | 98.6 | true |
| 1 | 128 | 1 | weir | 129616 | 14475 | 144091 | 0 | 7201.2 | 36.863 | 83.7 | 16.8 | 94.4 | true |
| 1 | 128 | 2 | direct | 96812 | 10981 | 107793 | 0 | 5386.7 | 65.535 | 99.9 | 99.0 | 99.0 | true |
| 1 | 128 | 2 | weir | 132444 | 14779 | 147223 | 0 | 7357.8 | 32.767 | 83.2 | 0.0 | 95.3 | true |
| 1 | 128 | 3 | direct | 101036 | 11440 | 112476 | 0 | 5619.4 | 61.439 | 100.0 | 98.8 | 98.8 | true |
| 1 | 128 | 3 | weir | 131355 | 14661 | 146016 | 0 | 7295.3 | 36.863 | 82.6 | 5.5 | 94.6 | true |
| 1 | 512 | 1 | direct | 0 | 0 | 0 | 0 | 0.0 | 0.000 | 0.0 | 0.0 | 0.0 | false |
| 1 | 512 | 1 | weir | 184552 | 20631 | 205183 | 0 | 10247.2 | 98.303 | 70.1 | 0.0 | 94.9 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 33516 | 1.066 | 32179/3555 | 0.020 | 3.315 | 0 | 32.000 |
| 1 | 8 | 2 | 48563 | 1.082 | 47265/5297 | 0.022 | 1.914 | 0 | 32.000 |
| 1 | 8 | 3 | 60680 | 1.086 | 59236/6657 | 0.021 | 1.375 | 0 | 32.000 |
| 1 | 32 | 1 | 59588 | 1.468 | 78669/8834 | 0.110 | 3.724 | 0 | 32.000 |
| 1 | 32 | 2 | 65007 | 1.490 | 87111/9778 | 0.115 | 2.940 | 0 | 32.000 |
| 1 | 32 | 3 | 64920 | 1.487 | 86800/9722 | 0.112 | 3.018 | 0 | 32.000 |
| 1 | 128 | 1 | 48108 | 2.995 | 129616/14475 | 0.615 | 5.563 | 0 | 32.000 |
| 1 | 128 | 2 | 50066 | 2.941 | 132444/14779 | 0.564 | 5.197 | 0 | 32.000 |
| 1 | 128 | 3 | 48589 | 3.005 | 131355/14661 | 0.607 | 5.293 | 0 | 32.000 |
| 1 | 512 | 1 | 23898 | 8.586 | 184552/20631 | 2.474 | 12.294 | 0 | 32.000 |

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.

Incomplete: direct single-request stage failed: independent client 14546 warmup failed: [Post "http://127.0.0.1:32769/weirtest_a6067408c9c65d9a55b1026d/_mget?realtime=true": context deadline exceeded Post "http://127.0.0.1:32769/weirtest_a6067408c9c65d9a55b1026d/_mget?realtime=true": context deadline exceeded Post "http://127.0.0.1:32769/weirtest_a6067408c9c65d9a55b1026d/_mget?realtime=true": net/http: timeout awaiting response headers Post "http://127.0.0.1:32769/weirtest_a6067408c9c65d9a55b1026d/_mget?realtime=true": context deadline exceeded Post "http://127.0.0.1:32769/weirtest_a6067408c9c65d9a55b1026d/_mget?realtime=true": context deadline exceeded] exit status 1 exit status 1 exit status 1 exit status 1 
