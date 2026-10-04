# Database saturation comparison

Backend **mongo**. Started 2026-10-04T03:16:23.234034764Z.

independent OS client processes send one record per native call or unary SDK RPC; native single-record snapshot transaction FindOne/compute/ReplaceOne/commit or real-time single-ID mget/compute/version-conditional PUT; SDK Lua AtomicTransform computes the same revision toggle from the current source; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 24684 | 24684 | 0 | 1231.5 | 45.055 | 98.8 | 96.5 | 96.5 | true |
| 1 | 8 | 1 | weir | 0 | 22757 | 22757 | 0 | 1137.7 | 22.527 | 99.7 | 98.2 | 98.2 | true |
| 1 | 8 | 2 | direct | 0 | 23971 | 23971 | 0 | 1198.4 | 49.151 | 99.0 | 96.8 | 96.8 | true |
| 1 | 8 | 2 | weir | 0 | 22310 | 22310 | 0 | 1115.4 | 22.527 | 99.6 | 98.6 | 98.6 | true |
| 1 | 8 | 3 | direct | 0 | 23742 | 23742 | 0 | 1187.0 | 45.055 | 99.2 | 98.1 | 98.1 | true |
| 1 | 8 | 3 | weir | 0 | 21595 | 21595 | 0 | 1079.5 | 24.575 | 99.6 | 98.6 | 98.6 | true |
| 1 | 32 | 1 | direct | 0 | 24568 | 24568 | 0 | 1227.9 | 73.727 | 99.7 | 98.8 | 98.8 | true |
| 1 | 32 | 1 | weir | 0 | 25605 | 25605 | 0 | 1279.6 | 61.439 | 99.8 | 98.9 | 98.9 | true |
| 1 | 32 | 2 | direct | 0 | 24085 | 24085 | 0 | 1204.0 | 73.727 | 99.7 | 98.4 | 98.4 | true |
| 1 | 32 | 2 | weir | 0 | 25322 | 25322 | 0 | 1265.6 | 61.439 | 99.9 | 98.9 | 98.9 | true |
| 1 | 32 | 3 | direct | 0 | 24049 | 24049 | 0 | 1201.6 | 73.727 | 99.7 | 98.9 | 98.9 | true |
| 1 | 32 | 3 | weir | 0 | 24865 | 24865 | 0 | 1240.0 | 57.343 | 99.8 | 98.5 | 98.5 | true |
| 1 | 128 | 1 | direct | 0 | 24773 | 24773 | 0 | 1232.5 | 196.607 | 100.1 | 98.3 | 98.3 | true |
| 1 | 128 | 1 | weir | 0 | 45387 | 45387 | 0 | 2266.4 | 114.687 | 100.0 | 98.9 | 98.9 | true |
| 1 | 128 | 2 | direct | 0 | 24979 | 24979 | 0 | 1244.3 | 196.607 | 99.6 | 98.9 | 98.9 | true |
| 1 | 128 | 2 | weir | 0 | 43072 | 43072 | 0 | 2151.0 | 122.879 | 100.0 | 98.9 | 98.9 | true |
| 1 | 128 | 3 | direct | 0 | 24998 | 24998 | 0 | 1231.9 | 196.607 | 100.2 | 97.8 | 97.8 | true |
| 1 | 128 | 3 | weir | 0 | 43851 | 43851 | 0 | 2189.8 | 114.687 | 100.0 | 98.7 | 98.7 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 21763 | 1.046 | 0/22757 | 0.023 | 5.722 | 0 | 32.000 |
| 1 | 8 | 2 | 21349 | 1.045 | 0/22310 | 0.023 | 5.869 | 0 | 32.000 |
| 1 | 8 | 3 | 20719 | 1.042 | 0/21595 | 0.023 | 6.139 | 0 | 32.000 |
| 1 | 32 | 1 | 20860 | 1.227 | 0/25605 | 0.103 | 20.168 | 0 | 32.000 |
| 1 | 32 | 2 | 20628 | 1.228 | 0/25322 | 0.104 | 20.357 | 0 | 32.000 |
| 1 | 32 | 3 | 20192 | 1.231 | 0/24865 | 0.114 | 20.729 | 0 | 32.000 |
| 1 | 128 | 1 | 17957 | 2.528 | 0/45387 | 3.108 | 29.220 | 0 | 32.000 |
| 1 | 128 | 2 | 16922 | 2.545 | 0/43072 | 3.641 | 31.547 | 0 | 32.000 |
| 1 | 128 | 3 | 17489 | 2.507 | 0/43851 | 3.232 | 30.373 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 1236.2 (128 workers), Weir 2202.4 (128 workers), observed change +78.16%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 1205.7 | 1110.9 | -7.86% | 3.839 / 49.151 / 53.247 | 5.631 / 22.527 / 32.767 |
| 32 | 1211.2 | 1261.7 | +4.17% | 14.335 / 73.727 / 98.303 | 20.479 / 61.439 / 90.111 |
| 128 | 1236.2 | 2202.4 | +78.16% | 98.303 / 196.607 / 425.983 | 53.247 / 114.687 / 212.991 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors/commitTransaction/abortTransaction deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
