# Database saturation comparison

Backend **mongo**. Started 2026-10-03T22:58:18.963946274Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 71194 | 71194 | 0 | 3559.5 | 2.559 | 91.3 | 78.7 | 99.6 | true |
| 1 | 8 | 1 | weir | 0 | 50365 | 50365 | 0 | 2517.8 | 5.119 | 74.1 | 26.8 | 96.0 | true |
| 1 | 8 | 2 | direct | 0 | 66711 | 66711 | 0 | 3335.4 | 3.327 | 90.0 | 63.0 | 99.6 | true |
| 1 | 8 | 2 | weir | 0 | 42222 | 42222 | 0 | 2110.8 | 5.631 | 61.2 | 10.7 | 94.9 | true |
| 1 | 8 | 3 | direct | 0 | 48444 | 48444 | 0 | 2421.2 | 7.167 | 67.4 | 10.4 | 99.1 | true |
| 1 | 8 | 3 | weir | 0 | 43152 | 43152 | 0 | 2151.2 | 6.143 | 69.4 | 21.4 | 95.6 | true |
| 1 | 32 | 1 | direct | 0 | 67927 | 67927 | 0 | 3389.2 | 57.343 | 91.2 | 58.5 | 95.3 | true |
| 1 | 32 | 1 | weir | 0 | 66796 | 66796 | 0 | 3338.8 | 30.719 | 90.9 | 69.3 | 95.8 | true |
| 1 | 32 | 2 | direct | 0 | 70490 | 70490 | 0 | 3517.0 | 57.343 | 97.5 | 85.9 | 96.3 | true |
| 1 | 32 | 2 | weir | 0 | 59828 | 59828 | 0 | 2990.9 | 36.863 | 80.7 | 31.9 | 95.6 | true |
| 1 | 32 | 3 | direct | 0 | 72372 | 72372 | 0 | 3618.0 | 57.343 | 98.1 | 96.3 | 96.3 | true |
| 1 | 32 | 3 | weir | 0 | 66960 | 66960 | 0 | 3347.0 | 28.671 | 96.8 | 91.2 | 96.4 | true |
| 1 | 128 | 1 | direct | 0 | 66960 | 66960 | 0 | 3346.6 | 131.071 | 91.1 | 59.1 | 96.9 | true |
| 1 | 128 | 1 | weir | 0 | 98096 | 98096 | 0 | 4901.8 | 81.919 | 95.3 | 75.0 | 96.4 | true |
| 1 | 128 | 2 | direct | 0 | 61925 | 61925 | 0 | 3093.4 | 106.495 | 92.5 | 70.9 | 97.1 | true |
| 1 | 128 | 2 | weir | 0 | 79975 | 79975 | 0 | 3993.4 | 122.879 | 77.4 | 32.2 | 95.5 | true |
| 1 | 128 | 3 | direct | 0 | 57383 | 57383 | 0 | 2867.1 | 106.495 | 97.4 | 87.2 | 97.8 | true |
| 1 | 128 | 3 | weir | 0 | 92040 | 92040 | 0 | 4599.4 | 90.111 | 92.4 | 64.5 | 96.9 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 46656 | 1.079 | 0/50365 | 0.013 | 2.412 | 0 | 32.000 |
| 1 | 8 | 2 | 38962 | 1.084 | 0/42222 | 0.012 | 3.078 | 0 | 32.000 |
| 1 | 8 | 3 | 39902 | 1.081 | 0/43152 | 0.012 | 3.019 | 0 | 32.000 |
| 1 | 32 | 1 | 49197 | 1.358 | 0/66796 | 0.065 | 6.945 | 0 | 32.000 |
| 1 | 32 | 2 | 43595 | 1.372 | 0/59828 | 0.066 | 7.930 | 0 | 32.000 |
| 1 | 32 | 3 | 49366 | 1.356 | 0/66960 | 0.066 | 7.023 | 0 | 32.000 |
| 1 | 128 | 1 | 34881 | 2.812 | 0/98096 | 2.337 | 13.693 | 0 | 32.000 |
| 1 | 128 | 2 | 27708 | 2.886 | 0/79975 | 3.168 | 18.371 | 0 | 32.000 |
| 1 | 128 | 3 | 32361 | 2.844 | 0/92040 | 2.057 | 14.255 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 3508.0 (32 workers), Weir 4498.1 (128 workers), observed change +28.22%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 3105.3 | 2259.9 | -27.23% | 1.279 / 3.583 / 45.055 | 2.303 / 5.631 / 57.343 |
| 32 | 3508.0 | 3225.6 | -8.05% | 3.583 / 57.343 / 81.919 | 6.143 / 30.719 / 90.111 |
| 128 | 3102.3 | 4498.1 | +44.99% | 18.431 / 114.687 / 229.375 | 18.431 / 90.111 / 212.991 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
