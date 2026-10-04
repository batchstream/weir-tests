# Database saturation comparison

Backend **mongo**. Started 2026-10-04T00:30:51.238594672Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 91838 | 10314 | 102152 | 0 | 5107.3 | 2.303 | 99.5 | 98.5 | 98.5 | true |
| 1 | 8 | 1 | weir | 68583 | 7684 | 76267 | 0 | 3812.7 | 3.839 | 77.4 | 0.0 | 99.7 | true |
| 1 | 8 | 2 | direct | 90966 | 10216 | 101182 | 0 | 5058.8 | 2.303 | 99.5 | 98.5 | 98.5 | true |
| 1 | 8 | 2 | weir | 68354 | 7651 | 76005 | 0 | 3799.8 | 3.839 | 77.8 | 0.0 | 99.7 | true |
| 1 | 8 | 3 | direct | 91536 | 10289 | 101825 | 0 | 5090.9 | 2.303 | 99.7 | 98.2 | 98.2 | true |
| 1 | 8 | 3 | weir | 67966 | 7610 | 75576 | 0 | 3778.5 | 4.095 | 77.7 | 0.0 | 94.6 | true |
| 1 | 32 | 1 | direct | 99366 | 11129 | 110495 | 0 | 5523.9 | 40.959 | 100.0 | 98.0 | 98.0 | true |
| 1 | 32 | 1 | weir | 98179 | 11009 | 109188 | 0 | 5458.5 | 11.263 | 85.2 | 5.6 | 94.7 | true |
| 1 | 32 | 2 | direct | 96948 | 10868 | 107816 | 0 | 5379.0 | 40.959 | 100.1 | 98.5 | 98.5 | true |
| 1 | 32 | 2 | weir | 95289 | 10663 | 105952 | 0 | 5296.8 | 12.287 | 84.7 | 0.0 | 94.9 | true |
| 1 | 32 | 3 | direct | 95431 | 10648 | 106079 | 0 | 5303.0 | 40.959 | 100.0 | 98.0 | 98.0 | true |
| 1 | 32 | 3 | weir | 94557 | 10583 | 105140 | 0 | 5256.1 | 12.287 | 85.9 | 5.7 | 95.4 | true |
| 1 | 128 | 1 | direct | 90851 | 10320 | 101171 | 0 | 5055.1 | 73.727 | 100.1 | 98.9 | 98.9 | true |
| 1 | 128 | 1 | weir | 142652 | 15848 | 158500 | 0 | 7922.2 | 32.767 | 78.3 | 0.0 | 95.0 | true |
| 1 | 128 | 2 | direct | 92203 | 10486 | 102689 | 0 | 5121.6 | 73.727 | 100.2 | 99.1 | 99.1 | true |
| 1 | 128 | 2 | weir | 143690 | 15935 | 159625 | 0 | 7978.6 | 32.767 | 78.7 | 0.0 | 94.9 | true |
| 1 | 128 | 3 | direct | 94200 | 10672 | 104872 | 0 | 5240.7 | 65.535 | 99.9 | 99.2 | 99.2 | true |
| 1 | 128 | 3 | weir | 142354 | 15857 | 158211 | 0 | 7907.7 | 32.767 | 77.6 | 0.0 | 95.6 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 68823 | 1.108 | 68583/7684 | 0.025 | 0.949 | 0 | 32.000 |
| 1 | 8 | 2 | 68611 | 1.108 | 68354/7651 | 0.025 | 0.956 | 0 | 32.000 |
| 1 | 8 | 3 | 68241 | 1.107 | 67966/7610 | 0.025 | 0.952 | 0 | 32.000 |
| 1 | 32 | 1 | 67188 | 1.625 | 98179/11009 | 0.133 | 1.927 | 0 | 32.000 |
| 1 | 32 | 2 | 64189 | 1.651 | 95289/10663 | 0.140 | 1.925 | 0 | 32.000 |
| 1 | 32 | 3 | 64661 | 1.626 | 94557/10583 | 0.136 | 1.983 | 0 | 32.000 |
| 1 | 128 | 1 | 46229 | 3.429 | 142652/15848 | 0.592 | 3.741 | 0 | 32.000 |
| 1 | 128 | 2 | 46697 | 3.418 | 143690/15935 | 0.579 | 3.731 | 0 | 32.000 |
| 1 | 128 | 3 | 46562 | 3.398 | 142354/15857 | 0.581 | 3.721 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 5402.0 (32 workers), Weir 7936.2 (128 workers), observed change +46.91%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 5085.7 | 3797.0 | -25.34% | 0.895 / 2.303 / 40.959 | 1.919 / 4.095 / 5.631 |
| 32 | 5402.0 | 5337.1 | -1.20% | 3.327 / 40.959 / 49.151 | 5.631 / 12.287 / 15.359 |
| 128 | 5139.1 | 7936.2 | +54.43% | 15.359 / 73.727 / 90.111 | 15.359 / 32.767 / 45.055 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
