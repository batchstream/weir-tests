# Database saturation comparison

Backend **mongo**. Started 2026-10-03T23:17:58.292585151Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 125836 | 0 | 125836 | 0 | 6291.5 | 1.791 | 99.9 | 98.3 | 98.3 | true |
| 1 | 8 | 1 | weir | 81717 | 0 | 81717 | 0 | 4085.5 | 3.583 | 68.1 | 0.0 | 100.0 | true |
| 1 | 8 | 2 | direct | 125206 | 0 | 125206 | 0 | 6259.9 | 1.791 | 100.0 | 97.9 | 97.9 | true |
| 1 | 8 | 2 | weir | 83027 | 0 | 83027 | 0 | 4151.0 | 3.583 | 68.5 | 0.0 | 99.5 | true |
| 1 | 8 | 3 | direct | 124485 | 0 | 124485 | 0 | 6223.9 | 1.791 | 99.9 | 99.0 | 99.0 | true |
| 1 | 8 | 3 | weir | 82814 | 0 | 82814 | 0 | 4140.4 | 3.583 | 68.5 | 0.0 | 99.4 | true |
| 1 | 32 | 1 | direct | 132840 | 0 | 132840 | 0 | 6640.9 | 16.383 | 100.2 | 98.3 | 98.3 | true |
| 1 | 32 | 1 | weir | 125916 | 0 | 125916 | 0 | 6294.7 | 10.239 | 69.7 | 0.0 | 94.4 | true |
| 1 | 32 | 2 | direct | 132746 | 0 | 132746 | 0 | 6636.3 | 16.383 | 100.2 | 98.6 | 98.6 | true |
| 1 | 32 | 2 | weir | 126114 | 0 | 126114 | 0 | 6304.0 | 10.239 | 69.9 | 0.0 | 99.6 | true |
| 1 | 32 | 3 | direct | 133160 | 0 | 133160 | 0 | 6656.7 | 14.335 | 100.1 | 98.5 | 98.5 | true |
| 1 | 32 | 3 | weir | 127304 | 0 | 127304 | 0 | 6363.9 | 10.239 | 70.2 | 0.0 | 99.5 | true |
| 1 | 128 | 1 | direct | 125125 | 0 | 125125 | 0 | 6251.9 | 57.343 | 100.1 | 98.9 | 98.9 | true |
| 1 | 128 | 1 | weir | 203013 | 0 | 203013 | 0 | 10144.2 | 26.623 | 55.5 | 0.0 | 99.3 | true |
| 1 | 128 | 2 | direct | 127279 | 0 | 127279 | 0 | 6359.7 | 57.343 | 100.2 | 99.5 | 99.5 | true |
| 1 | 128 | 2 | weir | 204802 | 0 | 204802 | 0 | 10235.4 | 26.623 | 55.8 | 0.0 | 99.2 | true |
| 1 | 128 | 3 | direct | 131427 | 0 | 131427 | 0 | 6567.6 | 57.343 | 100.2 | 98.9 | 98.9 | true |
| 1 | 128 | 3 | weir | 201946 | 0 | 201946 | 0 | 10092.0 | 26.623 | 55.6 | 0.0 | 99.6 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 72973 | 1.120 | 81717/0 | 0.026 | 0.823 | 0 | 32.000 |
| 1 | 8 | 2 | 74176 | 1.119 | 83027/0 | 0.025 | 0.814 | 0 | 32.000 |
| 1 | 8 | 3 | 73951 | 1.120 | 82814/0 | 0.025 | 0.816 | 0 | 32.000 |
| 1 | 32 | 1 | 71779 | 1.754 | 125916/0 | 0.141 | 1.562 | 0 | 32.000 |
| 1 | 32 | 2 | 71824 | 1.756 | 126114/0 | 0.138 | 1.563 | 0 | 32.000 |
| 1 | 32 | 3 | 72764 | 1.750 | 127304/0 | 0.136 | 1.550 | 0 | 32.000 |
| 1 | 128 | 1 | 52105 | 3.896 | 203013/0 | 0.516 | 2.739 | 0 | 32.000 |
| 1 | 128 | 2 | 52131 | 3.929 | 204802/0 | 0.516 | 2.685 | 0 | 32.000 |
| 1 | 128 | 3 | 52201 | 3.869 | 201946/0 | 0.502 | 2.762 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 6644.6 (32 workers), Weir 10157.2 (128 workers), observed change +52.86%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 6258.4 | 4125.6 | -34.08% | 0.831 / 1.791 / 30.719 | 1.791 / 3.583 / 4.607 |
| 32 | 6644.6 | 6320.8 | -4.87% | 3.071 / 15.359 / 45.055 | 5.119 / 10.239 / 12.287 |
| 128 | 6393.1 | 10157.2 | +58.88% | 14.335 / 57.343 / 65.535 | 12.287 / 26.623 / 36.863 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
