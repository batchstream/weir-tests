# Database saturation comparison

Backend **mongo**. Started 2026-10-04T00:11:58.479640123Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 0%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 183822 | 0 | 183822 | 0 | 9190.8 | 1.151 | 99.9 | 95.2 | 95.2 | true |
| 1 | 8 | 1 | weir | 88939 | 0 | 88939 | 0 | 4446.5 | 3.583 | 69.2 | 0.0 | 95.3 | true |
| 1 | 8 | 2 | direct | 184707 | 0 | 184707 | 0 | 9234.9 | 1.279 | 99.8 | 94.8 | 94.8 | true |
| 1 | 8 | 2 | weir | 96912 | 0 | 96912 | 0 | 4845.3 | 3.071 | 76.5 | 0.0 | 95.4 | true |
| 1 | 8 | 3 | direct | 184991 | 0 | 184991 | 0 | 9249.2 | 1.279 | 99.9 | 95.3 | 95.3 | true |
| 1 | 8 | 3 | weir | 96831 | 0 | 96831 | 0 | 4841.2 | 3.071 | 76.7 | 0.0 | 95.5 | true |
| 1 | 32 | 1 | direct | 231981 | 0 | 231981 | 0 | 11576.0 | 5.631 | 100.1 | 96.1 | 96.1 | true |
| 1 | 32 | 1 | weir | 154146 | 0 | 154146 | 0 | 7706.4 | 8.191 | 80.0 | 0.0 | 95.7 | true |
| 1 | 32 | 2 | direct | 227696 | 0 | 227696 | 0 | 11383.5 | 5.631 | 100.1 | 96.2 | 96.2 | true |
| 1 | 32 | 2 | weir | 153373 | 0 | 153373 | 0 | 7666.9 | 8.191 | 79.8 | 0.0 | 95.7 | true |
| 1 | 32 | 3 | direct | 232706 | 0 | 232706 | 0 | 11634.4 | 5.631 | 100.2 | 97.1 | 97.1 | true |
| 1 | 32 | 3 | weir | 154481 | 0 | 154481 | 0 | 7722.9 | 8.191 | 79.8 | 0.0 | 95.8 | true |
| 1 | 128 | 1 | direct | 225032 | 0 | 225032 | 0 | 11248.1 | 49.151 | 100.1 | 95.4 | 95.4 | true |
| 1 | 128 | 1 | weir | 251558 | 0 | 251558 | 0 | 12575.5 | 20.479 | 63.2 | 0.0 | 95.5 | true |
| 1 | 128 | 2 | direct | 238646 | 0 | 238646 | 0 | 11927.5 | 49.151 | 100.1 | 96.5 | 96.5 | true |
| 1 | 128 | 2 | weir | 258096 | 0 | 258096 | 0 | 12898.8 | 20.479 | 62.7 | 0.0 | 95.4 | true |
| 1 | 128 | 3 | direct | 230002 | 0 | 230002 | 0 | 11496.4 | 49.151 | 100.2 | 95.1 | 95.1 | true |
| 1 | 128 | 3 | weir | 249571 | 0 | 249571 | 0 | 12475.9 | 20.479 | 62.4 | 0.0 | 95.7 | true |
| 1 | 512 | 1 | direct | 207009 | 0 | 207009 | 0 | 10311.8 | 98.303 | 100.2 | 99.5 | 99.5 | true |
| 1 | 512 | 1 | weir | 362122 | 0 | 362122 | 0 | 18094.2 | 53.247 | 41.4 | 0.0 | 95.4 | true |
| 1 | 512 | 2 | direct | 208502 | 0 | 208502 | 0 | 10391.7 | 106.495 | 100.3 | 99.6 | 99.6 | true |
| 1 | 512 | 2 | weir | 364484 | 0 | 364484 | 0 | 18214.6 | 53.247 | 41.7 | 0.0 | 95.4 | true |
| 1 | 512 | 3 | direct | 214914 | 0 | 214914 | 0 | 10704.4 | 98.303 | 100.2 | 98.5 | 98.5 | true |
| 1 | 512 | 3 | weir | 361176 | 0 | 361176 | 0 | 18042.4 | 53.247 | 41.1 | 0.0 | 95.3 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 79461 | 1.119 | 88939/0 | 0.021 | 0.761 | 0 | 32.000 |
| 1 | 8 | 2 | 86020 | 1.127 | 96912/0 | 0.020 | 0.714 | 0 | 32.000 |
| 1 | 8 | 3 | 85996 | 1.126 | 96831/0 | 0.020 | 0.713 | 0 | 32.000 |
| 1 | 32 | 1 | 87821 | 1.755 | 154146/0 | 0.110 | 1.345 | 0 | 32.000 |
| 1 | 32 | 2 | 88087 | 1.741 | 153373/0 | 0.107 | 1.353 | 0 | 32.000 |
| 1 | 32 | 3 | 87341 | 1.769 | 154481/0 | 0.109 | 1.337 | 0 | 32.000 |
| 1 | 128 | 1 | 61732 | 4.075 | 251558/0 | 0.436 | 2.316 | 0 | 32.000 |
| 1 | 128 | 2 | 61261 | 4.213 | 258096/0 | 0.438 | 2.299 | 0 | 32.000 |
| 1 | 128 | 3 | 60768 | 4.107 | 249571/0 | 0.440 | 2.350 | 0 | 32.000 |
| 1 | 512 | 1 | 29818 | 12.144 | 362122/0 | 1.906 | 4.783 | 0 | 32.000 |
| 1 | 512 | 2 | 30543 | 11.933 | 364484/0 | 1.902 | 4.717 | 0 | 32.000 |
| 1 | 512 | 3 | 29887 | 12.085 | 361176/0 | 1.916 | 4.796 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 11557.4 (128 workers), Weir 18117.1 (512 workers), observed change +56.76%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 9225.0 | 4711.0 | -48.93% | 0.511 / 1.279 / 3.071 | 1.535 / 3.327 / 4.607 |
| 32 | 11531.3 | 7698.7 | -33.24% | 1.535 / 5.631 / 40.959 | 3.839 / 8.191 / 10.239 |
| 128 | 11557.4 | 12650.1 | +9.45% | 6.143 / 49.151 / 57.343 | 10.239 / 20.479 / 26.623 |
| 512 | 10469.3 | 18117.1 | +73.05% | 45.055 / 98.303 / 131.071 | 26.623 / 53.247 / 73.727 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
