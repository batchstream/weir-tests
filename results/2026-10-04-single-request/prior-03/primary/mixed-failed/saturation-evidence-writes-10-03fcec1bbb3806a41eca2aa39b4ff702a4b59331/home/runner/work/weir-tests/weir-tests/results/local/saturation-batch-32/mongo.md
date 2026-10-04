# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:58:35.19077007Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 92503 | 10374 | 102877 | 0 | 5143.5 | 2.303 | 99.5 | 98.4 | 98.4 | true |
| 1 | 8 | 1 | weir | 61973 | 6946 | 68919 | 0 | 3445.5 | 4.607 | 70.4 | 0.0 | 94.5 | true |
| 1 | 8 | 2 | direct | 91490 | 10274 | 101764 | 0 | 5088.0 | 2.303 | 99.6 | 98.9 | 98.9 | true |
| 1 | 8 | 2 | weir | 67976 | 7619 | 75595 | 0 | 3779.4 | 4.095 | 77.7 | 0.0 | 99.8 | true |
| 1 | 8 | 3 | direct | 91042 | 10230 | 101272 | 0 | 5063.3 | 2.303 | 99.6 | 98.1 | 98.1 | true |
| 1 | 8 | 3 | weir | 68525 | 7671 | 76196 | 0 | 3809.5 | 3.839 | 77.6 | 0.0 | 99.4 | true |
| 1 | 32 | 1 | direct | 96461 | 10800 | 107261 | 0 | 5361.9 | 40.959 | 100.0 | 99.0 | 99.0 | true |
| 1 | 32 | 1 | weir | 96009 | 10754 | 106763 | 0 | 5337.3 | 12.287 | 84.9 | 0.0 | 94.6 | true |
| 1 | 32 | 2 | direct | 94152 | 10524 | 104676 | 0 | 5233.2 | 40.959 | 100.0 | 97.9 | 97.9 | true |
| 1 | 32 | 2 | weir | 96061 | 10728 | 106789 | 0 | 5338.7 | 12.287 | 85.3 | 0.0 | 94.9 | true |
| 1 | 32 | 3 | direct | 92783 | 10354 | 103137 | 0 | 5156.0 | 40.959 | 100.0 | 99.0 | 99.0 | true |
| 1 | 32 | 3 | weir | 91557 | 10236 | 101793 | 0 | 5088.0 | 12.287 | 85.9 | 5.6 | 95.2 | true |
| 1 | 128 | 1 | direct | 89094 | 10123 | 99217 | 0 | 4956.4 | 73.727 | 100.1 | 99.4 | 99.4 | true |
| 1 | 128 | 1 | weir | 138264 | 15402 | 153666 | 0 | 7681.2 | 32.767 | 78.0 | 0.0 | 95.3 | true |
| 1 | 128 | 2 | direct | 86900 | 9870 | 96770 | 0 | 4834.6 | 73.727 | 100.1 | 99.3 | 99.3 | true |
| 1 | 128 | 2 | weir | 138971 | 15478 | 154449 | 0 | 7719.0 | 32.767 | 79.1 | 0.0 | 95.3 | true |
| 1 | 128 | 3 | direct | 88234 | 9976 | 98210 | 0 | 4896.5 | 73.727 | 100.3 | 99.0 | 99.0 | true |
| 1 | 128 | 3 | weir | 144521 | 16027 | 160548 | 0 | 8011.1 | 32.767 | 78.3 | 0.0 | 94.5 | true |
| 1 | 512 | 1 | direct | 89991 | 9805 | 99796 | 0 | 4964.0 | 212.991 | 100.0 | 95.8 | 95.8 | true |
| 1 | 512 | 1 | weir | 215888 | 24131 | 240019 | 0 | 11989.4 | 81.919 | 68.5 | 0.0 | 95.3 | true |
| 1 | 512 | 2 | direct | 90017 | 9748 | 99765 | 0 | 4960.8 | 212.991 | 100.3 | 95.0 | 95.0 | true |
| 1 | 512 | 2 | weir | 211335 | 23673 | 235008 | 0 | 11738.9 | 90.111 | 69.3 | 0.0 | 94.9 | true |
| 1 | 512 | 3 | direct | 94169 | 10276 | 104445 | 0 | 5197.5 | 196.607 | 100.1 | 97.1 | 97.1 | true |
| 1 | 512 | 3 | weir | 213425 | 23887 | 237312 | 0 | 11855.5 | 81.919 | 69.9 | 0.0 | 95.1 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 62510 | 1.103 | 61973/6946 | 0.027 | 1.016 | 0 | 32.000 |
| 1 | 8 | 2 | 68112 | 1.110 | 67976/7619 | 0.026 | 0.955 | 0 | 32.000 |
| 1 | 8 | 3 | 68871 | 1.106 | 68525/7671 | 0.025 | 0.945 | 0 | 32.000 |
| 1 | 32 | 1 | 65760 | 1.624 | 96009/10754 | 0.135 | 1.957 | 0 | 32.000 |
| 1 | 32 | 2 | 65229 | 1.637 | 96061/10728 | 0.140 | 1.927 | 0 | 32.000 |
| 1 | 32 | 3 | 62509 | 1.628 | 91557/10236 | 0.137 | 2.076 | 0 | 32.000 |
| 1 | 128 | 1 | 44233 | 3.474 | 138264/15402 | 0.614 | 3.818 | 0 | 32.000 |
| 1 | 128 | 2 | 43963 | 3.513 | 138971/15478 | 0.613 | 3.959 | 0 | 32.000 |
| 1 | 128 | 3 | 47961 | 3.347 | 144521/16027 | 0.576 | 3.753 | 0 | 32.000 |
| 1 | 512 | 1 | 24735 | 9.704 | 215888/24131 | 2.486 | 8.104 | 0 | 32.000 |
| 1 | 512 | 2 | 23581 | 9.966 | 211335/23673 | 2.517 | 8.819 | 0 | 32.000 |
| 1 | 512 | 3 | 24986 | 9.498 | 213425/23887 | 2.478 | 8.177 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 5250.4 (32 workers), Weir 11861.3 (512 workers), observed change +125.91%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 5098.3 | 3678.1 | -27.86% | 0.895 / 2.303 / 40.959 | 2.047 / 4.095 / 5.631 |
| 32 | 5250.4 | 5254.7 | +0.08% | 3.327 / 40.959 / 53.247 | 5.631 / 12.287 / 16.383 |
| 128 | 4895.9 | 7803.9 | +59.40% | 16.383 / 73.727 / 90.111 | 15.359 / 32.767 / 45.055 |
| 512 | 5040.8 | 11861.3 | +135.31% | 98.303 / 196.607 / 327.679 | 40.959 / 81.919 / 114.687 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
