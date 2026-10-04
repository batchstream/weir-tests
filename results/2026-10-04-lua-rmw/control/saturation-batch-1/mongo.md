# Database saturation comparison

Backend **mongo**. Started 2026-10-04T02:57:13.344578508Z.

independent OS client processes send one record per native call or unary SDK RPC; native single-record snapshot transaction FindOne/compute/ReplaceOne/commit or real-time single-ID mget/compute/version-conditional PUT; SDK Lua AtomicTransform computes the same revision toggle from the current source; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 24311 | 24311 | 0 | 1215.4 | 49.151 | 98.8 | 98.5 | 98.5 | true |
| 1 | 8 | 1 | weir | 0 | 21850 | 21850 | 0 | 1092.3 | 13.311 | 99.4 | 100.0 | 100.0 | true |
| 1 | 8 | 2 | direct | 0 | 24487 | 24487 | 0 | 1224.2 | 45.055 | 99.0 | 98.4 | 98.4 | true |
| 1 | 8 | 2 | weir | 0 | 21386 | 21386 | 0 | 1069.1 | 24.575 | 99.5 | 98.2 | 98.2 | true |
| 1 | 8 | 3 | direct | 0 | 23716 | 23716 | 0 | 1185.6 | 49.151 | 99.0 | 97.0 | 97.0 | true |
| 1 | 8 | 3 | weir | 0 | 20719 | 20719 | 0 | 1035.7 | 26.623 | 99.7 | 98.3 | 98.3 | true |
| 1 | 32 | 1 | direct | 0 | 24709 | 24709 | 0 | 1235.0 | 73.727 | 99.7 | 98.4 | 98.4 | true |
| 1 | 32 | 1 | weir | 0 | 21718 | 21718 | 0 | 1085.3 | 65.535 | 100.0 | 98.8 | 98.8 | true |
| 1 | 32 | 2 | direct | 0 | 23624 | 23624 | 0 | 1169.0 | 73.727 | 99.9 | 97.4 | 97.4 | true |
| 1 | 32 | 2 | weir | 0 | 21546 | 21546 | 0 | 1074.1 | 65.535 | 100.0 | 98.5 | 98.5 | true |
| 1 | 32 | 3 | direct | 0 | 24260 | 24260 | 0 | 1212.5 | 73.727 | 99.7 | 98.8 | 98.8 | true |
| 1 | 32 | 3 | weir | 0 | 20908 | 20908 | 0 | 1044.8 | 73.727 | 100.0 | 98.8 | 98.8 | true |
| 1 | 128 | 1 | direct | 0 | 25309 | 25309 | 0 | 1260.4 | 196.607 | 100.2 | 99.0 | 99.0 | true |
| 1 | 128 | 1 | weir | 0 | 21647 | 21647 | 0 | 1077.3 | 212.991 | 99.9 | 98.6 | 98.6 | true |
| 1 | 128 | 2 | direct | 0 | 24886 | 24886 | 0 | 1239.7 | 196.607 | 100.0 | 99.5 | 99.5 | true |
| 1 | 128 | 2 | weir | 0 | 21691 | 21691 | 0 | 1079.5 | 212.991 | 99.9 | 98.5 | 98.5 | true |
| 1 | 128 | 3 | direct | 0 | 25193 | 25193 | 0 | 1254.6 | 196.607 | 100.1 | 99.0 | 99.0 | true |
| 1 | 128 | 3 | weir | 0 | 22172 | 22172 | 0 | 1103.9 | 196.607 | 99.9 | 98.4 | 98.4 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 21850 | 1.000 | 0/21850 | 0.028 | 5.673 | 0 | 32.000 |
| 1 | 8 | 2 | 21386 | 1.000 | 0/21386 | 0.024 | 6.148 | 0 | 32.000 |
| 1 | 8 | 3 | 20719 | 1.000 | 0/20719 | 0.026 | 6.337 | 0 | 32.000 |
| 1 | 32 | 1 | 21718 | 1.000 | 0/21718 | 0.171 | 24.144 | 0 | 32.000 |
| 1 | 32 | 2 | 21546 | 1.000 | 0/21546 | 0.145 | 24.749 | 0 | 32.000 |
| 1 | 32 | 3 | 20908 | 1.000 | 0/20908 | 0.124 | 25.731 | 0 | 32.000 |
| 1 | 128 | 1 | 21647 | 1.000 | 0/21647 | 82.858 | 28.614 | 0 | 32.000 |
| 1 | 128 | 2 | 21691 | 1.000 | 0/21691 | 82.867 | 28.608 | 0 | 32.000 |
| 1 | 128 | 3 | 22172 | 1.000 | 0/22172 | 80.947 | 28.023 | 0 | 32.000 |

**Batch 1 at demonstrated database CPU saturation:** direct 1208.4 logical ops/s (8 workers), Weir 1068.1 logical ops/s (32 workers), Weir/direct 0.8839, change -11.61%.

Best verified business QPS within this ladder: direct 1251.6 (128 workers), Weir 1086.9 (128 workers), observed change -13.15%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 1208.4 | 1065.7 | -11.81% | 3.839 / 45.055 / 53.247 | 6.143 / 22.527 / 32.767 |
| 32 | 1205.4 | 1068.1 | -11.39% | 14.335 / 73.727 / 98.303 | 22.527 / 73.727 / 98.303 |
| 128 | 1251.6 | 1086.9 | -13.15% | 98.303 / 196.607 / 327.679 | 106.495 / 212.991 / 327.679 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors/commitTransaction/abortTransaction deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
