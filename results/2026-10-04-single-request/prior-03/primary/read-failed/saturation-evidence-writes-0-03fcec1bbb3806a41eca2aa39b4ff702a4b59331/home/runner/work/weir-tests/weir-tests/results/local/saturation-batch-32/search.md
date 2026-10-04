# Database saturation comparison

Backend **search**. Started 2026-10-03T23:11:53.556511491Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 43333 | 0 | 43333 | 0 | 2166.5 | 3.839 | 100.0 | 98.9 | 98.9 | true |
| 1 | 8 | 1 | weir | 74249 | 0 | 74249 | 0 | 3712.0 | 3.839 | 78.5 | 0.0 | 99.6 | true |
| 1 | 8 | 2 | direct | 116269 | 0 | 116269 | 0 | 5813.1 | 1.791 | 99.6 | 96.8 | 96.8 | true |
| 1 | 8 | 2 | weir | 76266 | 0 | 76266 | 0 | 3813.0 | 3.583 | 76.4 | 0.0 | 99.7 | true |
| 1 | 8 | 3 | direct | 117618 | 0 | 117618 | 0 | 5880.5 | 1.791 | 99.6 | 96.9 | 96.9 | true |
| 1 | 8 | 3 | weir | 77825 | 0 | 77825 | 0 | 3890.8 | 3.583 | 76.6 | 0.0 | 98.4 | true |
| 1 | 32 | 1 | direct | 141866 | 0 | 141866 | 0 | 7092.4 | 10.239 | 100.0 | 98.9 | 98.9 | true |
| 1 | 32 | 1 | weir | 115755 | 0 | 115755 | 0 | 5786.3 | 10.239 | 77.3 | 0.0 | 99.6 | true |
| 1 | 32 | 2 | direct | 141841 | 0 | 141841 | 0 | 7091.1 | 10.239 | 99.7 | 98.0 | 98.0 | true |
| 1 | 32 | 2 | weir | 115181 | 0 | 115181 | 0 | 5757.0 | 10.239 | 76.9 | 0.0 | 94.9 | true |
| 1 | 32 | 3 | direct | 137931 | 0 | 137931 | 0 | 6895.6 | 9.215 | 99.8 | 97.6 | 97.6 | true |
| 1 | 32 | 3 | weir | 114228 | 0 | 114228 | 0 | 5710.7 | 10.239 | 76.0 | 0.0 | 99.8 | true |
| 1 | 128 | 1 | direct | 147691 | 0 | 147691 | 0 | 7382.1 | 45.055 | 100.0 | 99.0 | 99.0 | true |
| 1 | 128 | 1 | weir | 170891 | 0 | 170891 | 0 | 8541.8 | 28.671 | 65.1 | 0.0 | 99.4 | true |
| 1 | 128 | 2 | direct | 147386 | 0 | 147386 | 0 | 7365.8 | 45.055 | 100.0 | 98.8 | 98.8 | true |
| 1 | 128 | 2 | weir | 175173 | 0 | 175173 | 0 | 8754.6 | 28.671 | 63.8 | 0.0 | 99.9 | true |
| 1 | 128 | 3 | direct | 152891 | 0 | 152891 | 0 | 7642.2 | 45.055 | 100.1 | 99.0 | 99.0 | true |
| 1 | 128 | 3 | weir | 179530 | 0 | 179530 | 0 | 8970.3 | 26.623 | 63.7 | 0.0 | 100.0 | true |
| 1 | 512 | 1 | direct | 0 | 0 | 0 | 0 | 0.0 | 0.000 | 0.0 | 0.0 | 0.0 | false |
| 1 | 512 | 1 | weir | 253639 | 0 | 253639 | 0 | 12667.3 | 81.919 | 47.8 | 0.0 | 99.6 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 67657 | 1.097 | 74249/0 | 0.022 | 1.091 | 0 | 32.000 |
| 1 | 8 | 2 | 69419 | 1.099 | 76266/0 | 0.022 | 1.048 | 0 | 32.000 |
| 1 | 8 | 3 | 71244 | 1.092 | 77825/0 | 0.021 | 1.030 | 0 | 32.000 |
| 1 | 32 | 1 | 74162 | 1.561 | 115755/0 | 0.115 | 2.284 | 0 | 32.000 |
| 1 | 32 | 2 | 73122 | 1.575 | 115181/0 | 0.115 | 2.253 | 0 | 32.000 |
| 1 | 32 | 3 | 72294 | 1.580 | 114228/0 | 0.119 | 2.251 | 0 | 32.000 |
| 1 | 128 | 1 | 50650 | 3.374 | 170891/0 | 0.560 | 4.486 | 0 | 32.000 |
| 1 | 128 | 2 | 52330 | 3.347 | 175173/0 | 0.536 | 4.259 | 0 | 32.000 |
| 1 | 128 | 3 | 54128 | 3.317 | 179530/0 | 0.519 | 4.252 | 0 | 32.000 |
| 1 | 512 | 1 | 28272 | 8.971 | 253639/0 | 2.174 | 8.988 | 0 | 32.000 |

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.

Incomplete: direct single-request stage failed: independent client 14582 warmup failed: [Post "http://127.0.0.1:32769/weirtest_4729a20f2e12f629061d617a/_mget?realtime=true": context deadline exceeded Post "http://127.0.0.1:32769/weirtest_4729a20f2e12f629061d617a/_mget?realtime=true": context deadline exceeded] exit status 1 exit status 1 exit status 1 exit status 1 
