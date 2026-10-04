# Database saturation comparison

Backend **search**. Started 2026-10-03T22:36:08.634459666Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 34708 | 3812 | 38520 | 0 | 1925.9 | 6.143 | 100.2 | 97.2 | 97.2 | true |
| 1 | 8 | 1 | weir | 52600 | 5880 | 58480 | 0 | 2923.8 | 4.607 | 97.7 | 94.8 | 94.8 | true |
| 1 | 8 | 2 | direct | 107373 | 12039 | 119412 | 0 | 5970.1 | 2.559 | 99.9 | 99.5 | 99.5 | true |
| 1 | 8 | 2 | weir | 71081 | 7969 | 79050 | 0 | 3952.2 | 3.583 | 91.1 | 68.4 | 100.0 | true |
| 1 | 8 | 3 | direct | 105929 | 11844 | 117773 | 0 | 5888.2 | 2.559 | 99.8 | 99.2 | 99.2 | true |
| 1 | 8 | 3 | weir | 76461 | 8552 | 85013 | 0 | 4250.3 | 3.327 | 90.7 | 73.6 | 100.0 | true |
| 1 | 32 | 1 | direct | 135800 | 15227 | 151027 | 0 | 7548.3 | 18.431 | 99.6 | 99.9 | 99.9 | true |
| 1 | 32 | 1 | weir | 119321 | 13343 | 132664 | 0 | 6632.4 | 10.239 | 96.7 | 95.0 | 95.0 | true |
| 1 | 32 | 2 | direct | 132226 | 14824 | 147050 | 0 | 7350.8 | 20.479 | 99.7 | 100.0 | 100.0 | true |
| 1 | 32 | 2 | weir | 119499 | 13349 | 132848 | 0 | 6641.7 | 9.215 | 96.4 | 95.1 | 95.1 | true |
| 1 | 32 | 3 | direct | 138339 | 15497 | 153836 | 0 | 7688.6 | 18.431 | 99.8 | 99.4 | 99.4 | true |
| 1 | 32 | 3 | weir | 122783 | 13708 | 136491 | 0 | 6822.8 | 9.215 | 96.8 | 95.1 | 95.1 | true |
| 1 | 128 | 1 | direct | 138763 | 15467 | 154230 | 0 | 7685.2 | 131.071 | 99.7 | 99.9 | 99.9 | true |
| 1 | 128 | 1 | weir | 181158 | 20094 | 201252 | 0 | 10058.9 | 24.575 | 88.6 | 10.7 | 95.8 | true |
| 1 | 128 | 2 | direct | 134380 | 14960 | 149340 | 0 | 7448.1 | 147.455 | 99.7 | 94.8 | 94.8 | true |
| 1 | 128 | 2 | weir | 176045 | 19539 | 195584 | 0 | 9776.4 | 24.575 | 89.8 | 15.8 | 95.5 | true |
| 1 | 128 | 3 | direct | 138713 | 15429 | 154142 | 0 | 7684.8 | 131.071 | 0.0 | 0.0 | 0.0 | true |
| 1 | 128 | 3 | weir | 182655 | 20240 | 202895 | 0 | 10139.9 | 24.575 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 1 | direct | 132859 | 14707 | 147566 | 0 | 7288.6 | 655.359 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 1 | weir | 272828 | 30418 | 303246 | 0 | 15149.1 | 61.439 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 2 | direct | 140285 | 15520 | 155805 | 0 | 7695.7 | 655.359 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 2 | weir | 260532 | 29093 | 289625 | 0 | 14468.5 | 65.535 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 3 | direct | 144984 | 15979 | 160963 | 0 | 7947.6 | 655.359 | 0.0 | 0.0 | 0.0 | true |
| 1 | 512 | 3 | weir | 263682 | 29479 | 293161 | 0 | 14641.2 | 65.535 | 0.0 | 0.0 | 0.0 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 54185 | 1.079 | 52600/5880 | 0.014 | 1.908 | 0 | 32.000 |
| 1 | 8 | 2 | 72723 | 1.087 | 71081/7969 | 0.014 | 1.228 | 0 | 32.000 |
| 1 | 8 | 3 | 77939 | 1.091 | 76461/8552 | 0.014 | 1.123 | 0 | 32.000 |
| 1 | 32 | 1 | 91619 | 1.448 | 119321/13343 | 0.070 | 2.338 | 0 | 32.000 |
| 1 | 32 | 2 | 90735 | 1.464 | 119499/13349 | 0.073 | 2.312 | 0 | 32.000 |
| 1 | 32 | 3 | 93214 | 1.464 | 122783/13708 | 0.073 | 2.220 | 0 | 32.000 |
| 1 | 128 | 1 | 67724 | 2.972 | 181158/20094 | 0.402 | 4.200 | 0 | 32.000 |
| 1 | 128 | 2 | 65836 | 2.971 | 176045/19539 | 0.465 | 4.379 | 0 | 32.000 |
| 1 | 128 | 3 | 69192 | 2.932 | 182655/20240 | 0.411 | 4.226 | 0 | 32.000 |
| 1 | 512 | 1 | 35610 | 8.516 | 272828/30418 | 1.648 | 8.970 | 0 | 32.000 |
| 1 | 512 | 2 | 35223 | 8.223 | 260532/29093 | 1.764 | 9.445 | 0 | 32.000 |
| 1 | 512 | 3 | 33906 | 8.646 | 263682/29479 | 1.812 | 9.316 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 7644.0 (512 workers), Weir 14752.9 (512 workers), observed change +93.00%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 4594.8 | 3708.7 | -19.28% | 0.767 / 3.071 / 36.863 | 1.663 / 3.839 / 10.239 |
| 32 | 7529.2 | 6699.0 | -11.03% | 1.535 / 20.479 / 53.247 | 4.607 / 9.215 / 15.359 |
| 128 | 7606.1 | 9991.7 | +31.37% | 1.919 / 131.071 / 180.223 | 12.287 / 24.575 / 36.863 |
| 512 | 7644.0 | 14752.9 | +93.00% | 2.559 / 655.359 / 720.895 | 36.863 / 65.535 / 81.919 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
