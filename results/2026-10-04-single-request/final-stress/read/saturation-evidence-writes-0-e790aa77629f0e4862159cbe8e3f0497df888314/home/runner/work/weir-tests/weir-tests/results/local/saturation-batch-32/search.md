# Database saturation comparison

Backend **search**. Started 2026-10-04T00:24:12.956347172Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 75325 | 0 | 75325 | 0 | 3766.1 | 2.559 | 100.0 | 96.7 | 96.7 | true |
| 1 | 8 | 1 | weir | 92952 | 0 | 92952 | 0 | 4647.3 | 3.071 | 77.9 | 0.0 | 95.1 | true |
| 1 | 8 | 2 | direct | 165998 | 0 | 165998 | 0 | 8299.6 | 1.279 | 99.8 | 99.8 | 99.8 | true |
| 1 | 8 | 2 | weir | 93131 | 0 | 93131 | 0 | 4656.3 | 3.071 | 76.5 | 0.0 | 95.5 | true |
| 1 | 8 | 3 | direct | 169552 | 0 | 169552 | 0 | 8467.3 | 1.279 | 99.6 | 99.5 | 99.5 | true |
| 1 | 8 | 3 | weir | 92477 | 0 | 92477 | 0 | 4623.6 | 3.071 | 75.6 | 0.0 | 95.6 | true |
| 1 | 32 | 1 | direct | 206410 | 0 | 206410 | 0 | 10320.0 | 5.119 | 99.8 | 95.2 | 95.2 | true |
| 1 | 32 | 1 | weir | 144631 | 0 | 144631 | 0 | 7230.5 | 9.215 | 76.5 | 0.0 | 96.0 | true |
| 1 | 32 | 2 | direct | 206834 | 0 | 206834 | 0 | 10340.5 | 5.119 | 99.6 | 100.0 | 100.0 | true |
| 1 | 32 | 2 | weir | 147935 | 0 | 147935 | 0 | 7395.8 | 8.191 | 78.9 | 0.0 | 95.3 | true |
| 1 | 32 | 3 | direct | 208824 | 0 | 208824 | 0 | 10440.3 | 5.119 | 99.7 | 95.0 | 95.0 | true |
| 1 | 32 | 3 | weir | 149442 | 0 | 149442 | 0 | 7470.5 | 8.191 | 77.8 | 0.0 | 95.8 | true |
| 1 | 128 | 1 | direct | 221665 | 0 | 221665 | 0 | 11076.9 | 45.055 | 99.9 | 97.4 | 97.4 | true |
| 1 | 128 | 1 | weir | 223941 | 0 | 223941 | 0 | 11193.7 | 22.527 | 66.7 | 0.0 | 96.1 | true |
| 1 | 128 | 2 | direct | 221262 | 0 | 221262 | 0 | 11059.9 | 45.055 | 99.9 | 97.0 | 97.0 | true |
| 1 | 128 | 2 | weir | 226307 | 0 | 226307 | 0 | 11312.0 | 20.479 | 65.9 | 0.0 | 96.0 | true |
| 1 | 128 | 3 | direct | 222915 | 0 | 222915 | 0 | 11143.0 | 45.055 | 99.9 | 98.3 | 98.3 | true |
| 1 | 128 | 3 | weir | 225265 | 0 | 225265 | 0 | 11259.1 | 22.527 | 65.3 | 0.0 | 95.5 | true |
| 1 | 512 | 1 | direct | 202522 | 0 | 202522 | 0 | 10096.9 | 90.111 | 99.9 | 98.5 | 98.5 | true |
| 1 | 512 | 1 | weir | 308937 | 0 | 308937 | 0 | 15433.4 | 61.439 | 50.3 | 0.0 | 95.6 | true |
| 1 | 512 | 2 | direct | 213195 | 0 | 213195 | 0 | 10647.6 | 90.111 | 99.9 | 98.9 | 98.9 | true |
| 1 | 512 | 2 | weir | 308825 | 0 | 308825 | 0 | 15426.8 | 61.439 | 50.8 | 0.0 | 96.0 | true |
| 1 | 512 | 3 | direct | 207640 | 0 | 207640 | 0 | 10352.2 | 90.111 | 99.8 | 98.5 | 98.5 | true |
| 1 | 512 | 3 | weir | 308662 | 0 | 308662 | 0 | 15426.9 | 61.439 | 50.2 | 0.0 | 96.0 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 84011 | 1.106 | 92952/0 | 0.017 | 0.871 | 0 | 32.000 |
| 1 | 8 | 2 | 83999 | 1.109 | 93131/0 | 0.017 | 0.876 | 0 | 32.000 |
| 1 | 8 | 3 | 83493 | 1.108 | 92477/0 | 0.017 | 0.868 | 0 | 32.000 |
| 1 | 32 | 1 | 90855 | 1.592 | 144631/0 | 0.093 | 1.838 | 0 | 32.000 |
| 1 | 32 | 2 | 92664 | 1.596 | 147935/0 | 0.091 | 1.856 | 0 | 32.000 |
| 1 | 32 | 3 | 94293 | 1.585 | 149442/0 | 0.091 | 1.795 | 0 | 32.000 |
| 1 | 128 | 1 | 64060 | 3.496 | 223941/0 | 0.462 | 3.592 | 0 | 32.000 |
| 1 | 128 | 2 | 65713 | 3.444 | 226307/0 | 0.440 | 3.532 | 0 | 32.000 |
| 1 | 128 | 3 | 64796 | 3.477 | 225265/0 | 0.430 | 3.530 | 0 | 32.000 |
| 1 | 512 | 1 | 31825 | 9.707 | 308937/0 | 1.865 | 7.707 | 0 | 32.000 |
| 1 | 512 | 2 | 31038 | 9.950 | 308825/0 | 1.959 | 7.721 | 0 | 32.000 |
| 1 | 512 | 3 | 30905 | 9.987 | 308662/0 | 2.007 | 7.567 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 11093.3 (128 workers), Weir 15429.1 (512 workers), observed change +39.08%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 6845.0 | 4642.4 | -32.18% | 0.703 / 1.663 / 24.575 | 1.663 / 3.071 / 4.607 |
| 32 | 10366.9 | 7365.6 | -28.95% | 2.047 / 5.119 / 36.863 | 4.095 / 8.191 / 11.263 |
| 128 | 11093.3 | 11254.9 | +1.46% | 7.679 / 45.055 / 49.151 | 11.263 / 22.527 / 26.623 |
| 512 | 10365.4 | 15429.1 | +48.85% | 49.151 / 90.111 / 106.495 | 32.767 / 61.439 / 81.919 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
