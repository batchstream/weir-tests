# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:23:53.475905932Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 133525 | 14856 | 148381 | 0 | 7418.8 | 1.663 | 99.4 | 99.4 | 99.4 | true |
| 1 | 8 | 1 | weir | 86791 | 9680 | 96471 | 0 | 4822.9 | 3.327 | 86.1 | 37.1 | 95.1 | true |
| 1 | 8 | 2 | direct | 133723 | 14884 | 148607 | 0 | 7430.1 | 1.791 | 99.7 | 99.6 | 99.6 | true |
| 1 | 8 | 2 | weir | 89072 | 10018 | 99090 | 0 | 4954.1 | 3.327 | 88.0 | 0.0 | 94.9 | true |
| 1 | 8 | 3 | direct | 132420 | 14755 | 147175 | 0 | 7358.3 | 1.663 | 99.6 | 98.9 | 98.9 | true |
| 1 | 8 | 3 | weir | 88727 | 9974 | 98701 | 0 | 4934.7 | 3.327 | 88.8 | 26.1 | 99.6 | true |
| 1 | 32 | 1 | direct | 156043 | 17495 | 173538 | 0 | 8676.1 | 7.167 | 99.9 | 94.9 | 94.9 | true |
| 1 | 32 | 1 | weir | 130706 | 14656 | 145362 | 0 | 7266.8 | 9.215 | 97.6 | 94.9 | 94.9 | true |
| 1 | 32 | 2 | direct | 157435 | 17634 | 175069 | 0 | 8752.4 | 6.655 | 100.1 | 95.2 | 95.2 | true |
| 1 | 32 | 2 | weir | 129958 | 14582 | 144540 | 0 | 7225.3 | 9.215 | 97.5 | 95.0 | 95.0 | true |
| 1 | 32 | 3 | direct | 159596 | 17883 | 177479 | 0 | 8873.1 | 6.655 | 99.9 | 99.7 | 99.7 | true |
| 1 | 32 | 3 | weir | 130192 | 14630 | 144822 | 0 | 7240.0 | 9.215 | 97.7 | 95.1 | 95.1 | true |
| 1 | 128 | 1 | direct | 167108 | 18613 | 185721 | 0 | 9282.4 | 61.439 | 100.3 | 96.0 | 96.0 | true |
| 1 | 128 | 1 | weir | 201414 | 22285 | 223699 | 0 | 11181.3 | 22.527 | 91.2 | 68.8 | 95.1 | true |
| 1 | 128 | 2 | direct | 170451 | 18975 | 189426 | 0 | 9468.1 | 61.439 | 100.2 | 97.3 | 97.3 | true |
| 1 | 128 | 2 | weir | 201324 | 22233 | 223557 | 0 | 11172.0 | 22.527 | 90.7 | 47.6 | 95.1 | true |
| 1 | 128 | 3 | direct | 164577 | 18357 | 182934 | 0 | 9143.6 | 61.439 | 99.9 | 95.9 | 95.9 | true |
| 1 | 128 | 3 | weir | 199555 | 22090 | 221645 | 0 | 11079.7 | 22.527 | 89.6 | 31.7 | 95.0 | true |
| 1 | 512 | 1 | direct | 153805 | 17018 | 170823 | 0 | 8505.0 | 163.839 | 100.3 | 98.6 | 98.6 | true |
| 1 | 512 | 1 | weir | 296233 | 32954 | 329187 | 0 | 16446.1 | 61.439 | 76.4 | 0.0 | 95.1 | true |
| 1 | 512 | 2 | direct | 154255 | 17025 | 171280 | 0 | 8532.7 | 163.839 | 100.0 | 97.5 | 97.5 | true |
| 1 | 512 | 2 | weir | 299560 | 33392 | 332952 | 0 | 16636.3 | 57.343 | 76.9 | 0.0 | 95.2 | true |
| 1 | 512 | 3 | direct | 167515 | 18662 | 186177 | 0 | 9295.6 | 114.687 | 100.1 | 98.4 | 98.4 | true |
| 1 | 512 | 3 | weir | 288880 | 32174 | 321054 | 0 | 16042.0 | 61.439 | 78.3 | 0.0 | 94.9 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 86836 | 1.111 | 86791/9680 | 0.018 | 0.801 | 0 | 32.000 |
| 1 | 8 | 2 | 89515 | 1.107 | 89072/10018 | 0.017 | 0.781 | 0 | 32.000 |
| 1 | 8 | 3 | 89090 | 1.108 | 88727/9974 | 0.017 | 0.798 | 0 | 32.000 |
| 1 | 32 | 1 | 91249 | 1.593 | 130706/14656 | 0.087 | 1.581 | 0 | 32.000 |
| 1 | 32 | 2 | 91880 | 1.573 | 129958/14582 | 0.087 | 1.619 | 0 | 32.000 |
| 1 | 32 | 3 | 91663 | 1.580 | 130192/14630 | 0.086 | 1.624 | 0 | 32.000 |
| 1 | 128 | 1 | 68092 | 3.285 | 201414/22285 | 0.395 | 2.962 | 0 | 32.000 |
| 1 | 128 | 2 | 66578 | 3.358 | 201324/22233 | 0.399 | 3.057 | 0 | 32.000 |
| 1 | 128 | 3 | 67725 | 3.273 | 199555/22090 | 0.409 | 3.039 | 0 | 32.000 |
| 1 | 512 | 1 | 35477 | 9.279 | 296233/32954 | 1.754 | 6.415 | 0 | 32.000 |
| 1 | 512 | 2 | 34338 | 9.696 | 299560/33392 | 1.889 | 6.497 | 0 | 32.000 |
| 1 | 512 | 3 | 34108 | 9.413 | 288880/32174 | 1.990 | 7.047 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 9298.0 (128 workers), Weir 16374.8 (512 workers), observed change +76.11%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 7402.4 | 4903.9 | -33.75% | 0.511 / 1.663 / 36.863 | 1.407 / 3.327 / 4.607 |
| 32 | 8767.2 | 7244.0 | -17.37% | 1.791 / 6.655 / 53.247 | 4.095 / 9.215 / 13.311 |
| 128 | 9298.0 | 11144.4 | +19.86% | 6.655 / 61.439 / 73.727 | 11.263 / 22.527 / 32.767 |
| 512 | 8777.3 | 16374.8 | +86.56% | 61.439 / 131.071 / 229.375 | 28.671 / 61.439 / 90.111 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
