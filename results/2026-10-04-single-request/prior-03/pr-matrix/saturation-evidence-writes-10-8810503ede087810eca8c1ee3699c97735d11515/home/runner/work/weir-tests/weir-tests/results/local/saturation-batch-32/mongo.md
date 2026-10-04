# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:58:33.690912228Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 93193 | 10443 | 103636 | 0 | 5181.6 | 2.303 | 99.6 | 99.0 | 99.0 | true |
| 1 | 8 | 1 | weir | 60367 | 6771 | 67138 | 0 | 3356.7 | 4.607 | 66.1 | 0.0 | 94.8 | true |
| 1 | 8 | 2 | direct | 92953 | 10431 | 103384 | 0 | 5169.0 | 2.303 | 99.6 | 98.0 | 98.0 | true |
| 1 | 8 | 2 | weir | 68720 | 7693 | 76413 | 0 | 3820.4 | 3.839 | 77.2 | 0.0 | 99.6 | true |
| 1 | 8 | 3 | direct | 91743 | 10308 | 102051 | 0 | 5102.3 | 2.303 | 99.5 | 98.7 | 98.7 | true |
| 1 | 8 | 3 | weir | 69325 | 7761 | 77086 | 0 | 3853.9 | 3.839 | 76.9 | 0.0 | 99.3 | true |
| 1 | 32 | 1 | direct | 99890 | 11187 | 111077 | 0 | 5553.2 | 40.959 | 100.1 | 99.3 | 99.3 | true |
| 1 | 32 | 1 | weir | 98595 | 11037 | 109632 | 0 | 5481.0 | 11.263 | 84.6 | 0.0 | 94.7 | true |
| 1 | 32 | 2 | direct | 97323 | 10893 | 108216 | 0 | 5409.9 | 40.959 | 100.1 | 98.8 | 98.8 | true |
| 1 | 32 | 2 | weir | 97387 | 10921 | 108308 | 0 | 5414.2 | 12.287 | 84.6 | 0.0 | 95.3 | true |
| 1 | 32 | 3 | direct | 98140 | 10984 | 109124 | 0 | 5455.3 | 40.959 | 99.9 | 98.0 | 98.0 | true |
| 1 | 32 | 3 | weir | 97276 | 10883 | 108159 | 0 | 5406.5 | 12.287 | 85.1 | 5.5 | 94.7 | true |
| 1 | 128 | 1 | direct | 96140 | 10947 | 107087 | 0 | 5350.9 | 65.535 | 100.1 | 99.3 | 99.3 | true |
| 1 | 128 | 1 | weir | 146949 | 16320 | 163269 | 0 | 8161.1 | 30.719 | 77.5 | 0.0 | 94.6 | true |
| 1 | 128 | 2 | direct | 96540 | 10993 | 107533 | 0 | 5373.3 | 65.535 | 100.1 | 99.4 | 99.4 | true |
| 1 | 128 | 2 | weir | 146272 | 16329 | 162601 | 0 | 8126.7 | 30.719 | 79.2 | 0.0 | 100.0 | true |
| 1 | 128 | 3 | direct | 96564 | 10922 | 107486 | 0 | 5370.8 | 65.535 | 100.2 | 99.4 | 99.4 | true |
| 1 | 128 | 3 | weir | 148458 | 16544 | 165002 | 0 | 8246.6 | 30.719 | 77.9 | 0.0 | 94.8 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 60835 | 1.104 | 60367/6771 | 0.028 | 1.022 | 0 | 32.000 |
| 1 | 8 | 2 | 68828 | 1.110 | 68720/7693 | 0.025 | 0.941 | 0 | 32.000 |
| 1 | 8 | 3 | 69421 | 1.110 | 69325/7761 | 0.025 | 0.930 | 0 | 32.000 |
| 1 | 32 | 1 | 66794 | 1.641 | 98595/11037 | 0.134 | 1.907 | 0 | 32.000 |
| 1 | 32 | 2 | 66574 | 1.627 | 97387/10921 | 0.131 | 1.941 | 0 | 32.000 |
| 1 | 32 | 3 | 66440 | 1.628 | 97276/10883 | 0.135 | 1.965 | 0 | 32.000 |
| 1 | 128 | 1 | 47906 | 3.408 | 146949/16320 | 0.556 | 3.613 | 0 | 32.000 |
| 1 | 128 | 2 | 47641 | 3.413 | 146272/16329 | 0.598 | 3.778 | 0 | 32.000 |
| 1 | 128 | 3 | 47708 | 3.459 | 148458/16544 | 0.572 | 3.700 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 5472.8 (32 workers), Weir 8178.1 (128 workers), observed change +49.43%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 5150.9 | 3677.0 | -28.61% | 0.895 / 2.303 / 36.863 | 2.047 / 4.095 / 5.631 |
| 32 | 5472.8 | 5433.9 | -0.71% | 3.327 / 40.959 / 49.151 | 5.631 / 12.287 / 15.359 |
| 128 | 5365.0 | 8178.1 | +52.43% | 15.359 / 65.535 / 81.919 | 15.359 / 30.719 / 45.055 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
