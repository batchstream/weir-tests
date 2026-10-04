# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:58:20.908894149Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 100074 | 100074 | 0 | 5003.6 | 1.791 | 98.1 | 94.3 | 99.5 | true |
| 1 | 8 | 1 | weir | 0 | 69662 | 69662 | 0 | 3482.8 | 3.839 | 83.3 | 36.9 | 94.8 | true |
| 1 | 8 | 2 | direct | 0 | 79963 | 79963 | 0 | 3997.9 | 3.583 | 84.0 | 31.3 | 99.5 | true |
| 1 | 8 | 2 | weir | 0 | 67927 | 67927 | 0 | 3395.9 | 4.095 | 81.0 | 37.1 | 94.8 | true |
| 1 | 8 | 3 | direct | 0 | 83631 | 83631 | 0 | 4181.4 | 2.303 | 94.2 | 84.0 | 99.7 | true |
| 1 | 8 | 3 | weir | 0 | 70729 | 70729 | 0 | 3536.0 | 3.327 | 91.6 | 78.9 | 94.8 | true |
| 1 | 32 | 1 | direct | 0 | 105058 | 105058 | 0 | 5252.5 | 53.247 | 99.0 | 95.1 | 95.1 | true |
| 1 | 32 | 1 | weir | 0 | 98323 | 98323 | 0 | 4915.4 | 20.479 | 99.0 | 90.7 | 96.2 | true |
| 1 | 32 | 2 | direct | 0 | 106151 | 106151 | 0 | 5307.0 | 53.247 | 98.4 | 96.2 | 96.2 | true |
| 1 | 32 | 2 | weir | 0 | 97169 | 97169 | 0 | 4858.0 | 20.479 | 99.7 | 96.1 | 96.1 | true |
| 1 | 32 | 3 | direct | 0 | 103354 | 103354 | 0 | 5167.3 | 53.247 | 98.2 | 97.0 | 97.0 | true |
| 1 | 32 | 3 | weir | 0 | 91586 | 91586 | 0 | 4578.6 | 20.479 | 98.9 | 96.3 | 96.3 | true |
| 1 | 128 | 1 | direct | 0 | 103601 | 103601 | 0 | 5178.5 | 81.919 | 99.5 | 97.4 | 97.4 | true |
| 1 | 128 | 1 | weir | 0 | 147183 | 147183 | 0 | 7351.4 | 57.343 | 98.2 | 97.0 | 97.0 | true |
| 1 | 128 | 2 | direct | 0 | 105069 | 105069 | 0 | 5251.2 | 81.919 | 99.5 | 97.1 | 97.1 | true |
| 1 | 128 | 2 | weir | 0 | 139934 | 139934 | 0 | 6994.3 | 73.727 | 97.5 | 91.2 | 96.4 | true |
| 1 | 128 | 3 | direct | 0 | 96428 | 96428 | 0 | 4820.1 | 81.919 | 99.7 | 97.2 | 97.2 | true |
| 1 | 128 | 3 | weir | 0 | 137058 | 137058 | 0 | 6848.7 | 61.439 | 99.1 | 96.9 | 96.9 | true |
| 1 | 512 | 1 | direct | 0 | 83522 | 83522 | 0 | 4155.0 | 262.143 | 100.0 | 99.1 | 99.1 | true |
| 1 | 512 | 1 | weir | 0 | 192647 | 192647 | 0 | 9626.9 | 122.879 | 98.4 | 97.0 | 97.0 | true |
| 1 | 512 | 2 | direct | 0 | 77061 | 77061 | 0 | 3832.2 | 327.679 | 100.0 | 99.6 | 99.6 | true |
| 1 | 512 | 2 | weir | 0 | 179467 | 179467 | 0 | 8944.4 | 147.455 | 95.9 | 91.2 | 96.5 | true |
| 1 | 512 | 3 | direct | 0 | 81598 | 81598 | 0 | 4058.3 | 294.911 | 99.6 | 99.7 | 99.7 | true |
| 1 | 512 | 3 | weir | 0 | 186258 | 186258 | 0 | 9303.8 | 122.879 | 99.0 | 97.2 | 97.2 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 63897 | 1.090 | 0/69662 | 0.008 | 1.741 | 0 | 32.000 |
| 1 | 8 | 2 | 61864 | 1.098 | 0/67927 | 0.009 | 1.798 | 0 | 32.000 |
| 1 | 8 | 3 | 64708 | 1.093 | 0/70729 | 0.009 | 1.714 | 0 | 32.000 |
| 1 | 32 | 1 | 74110 | 1.327 | 0/98323 | 0.044 | 4.558 | 0 | 32.000 |
| 1 | 32 | 2 | 72987 | 1.331 | 0/97169 | 0.045 | 4.513 | 0 | 32.000 |
| 1 | 32 | 3 | 68257 | 1.342 | 0/91586 | 0.047 | 5.067 | 0 | 32.000 |
| 1 | 128 | 1 | 50784 | 2.898 | 0/147183 | 1.397 | 8.511 | 0 | 32.000 |
| 1 | 128 | 2 | 47343 | 2.956 | 0/139934 | 1.576 | 9.322 | 0 | 32.000 |
| 1 | 128 | 3 | 47103 | 2.910 | 0/137058 | 1.346 | 9.415 | 0 | 32.000 |
| 1 | 512 | 1 | 18711 | 10.296 | 0/192647 | 6.580 | 24.293 | 0 | 32.000 |
| 1 | 512 | 2 | 18601 | 9.648 | 0/179467 | 7.868 | 24.190 | 0 | 32.000 |
| 1 | 512 | 3 | 18974 | 9.816 | 0/186258 | 6.877 | 23.938 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 5242.3 (32 workers), Weir 9291.4 (512 workers), observed change +77.24%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 4394.3 | 3471.6 | -21.00% | 1.023 / 2.047 / 32.767 | 1.791 / 3.583 / 15.359 |
| 32 | 5242.3 | 4784.0 | -8.74% | 2.559 / 53.247 / 73.727 | 4.607 / 20.479 / 57.343 |
| 128 | 5083.3 | 7064.9 | +38.98% | 10.239 / 81.919 / 122.879 | 13.311 / 61.439 / 90.111 |
| 512 | 4015.1 | 9291.4 | +131.41% | 106.495 / 294.911 / 491.519 | 40.959 / 122.879 / 393.215 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
