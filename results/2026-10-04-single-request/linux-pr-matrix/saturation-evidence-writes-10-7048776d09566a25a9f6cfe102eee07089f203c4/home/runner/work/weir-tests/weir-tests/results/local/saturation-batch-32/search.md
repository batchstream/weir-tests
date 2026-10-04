# Database saturation comparison

Backend **search**. Started 2026-10-03T22:31:12.308020257Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 10%.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 70055 | 7842 | 77897 | 0 | 3894.7 | 3.071 | 100.1 | 97.2 | 97.2 | true |
| 1 | 8 | 1 | weir | 83213 | 9304 | 92517 | 0 | 4625.7 | 3.327 | 89.6 | 42.5 | 96.0 | true |
| 1 | 8 | 2 | direct | 179290 | 19905 | 199195 | 0 | 9959.5 | 1.791 | 99.8 | 99.8 | 99.8 | true |
| 1 | 8 | 2 | weir | 103228 | 11591 | 114819 | 0 | 5734.7 | 2.815 | 77.7 | 0.0 | 95.3 | true |
| 1 | 8 | 3 | direct | 183971 | 20342 | 204313 | 0 | 10215.3 | 1.791 | 99.9 | 95.3 | 95.3 | true |
| 1 | 8 | 3 | weir | 112137 | 12554 | 124691 | 0 | 6234.1 | 2.815 | 76.8 | 0.0 | 96.0 | true |
| 1 | 32 | 1 | direct | 208716 | 23289 | 232005 | 0 | 11598.8 | 12.287 | 99.8 | 95.3 | 95.3 | true |
| 1 | 32 | 1 | weir | 156778 | 17561 | 174339 | 0 | 8716.1 | 7.679 | 82.0 | 0.0 | 96.2 | true |
| 1 | 32 | 2 | direct | 214786 | 23970 | 238756 | 0 | 11934.2 | 10.239 | 100.0 | 95.6 | 95.6 | true |
| 1 | 32 | 2 | weir | 154516 | 17335 | 171851 | 0 | 8590.9 | 7.679 | 83.6 | 16.0 | 96.9 | true |
| 1 | 32 | 3 | direct | 200041 | 22345 | 222386 | 0 | 11113.9 | 11.263 | 93.5 | 69.3 | 95.5 | true |
| 1 | 32 | 3 | weir | 161794 | 18128 | 179922 | 0 | 8995.1 | 7.167 | 82.3 | 0.0 | 96.2 | true |
| 1 | 128 | 1 | direct | 218894 | 24146 | 243040 | 0 | 12123.6 | 90.111 | 100.0 | 96.6 | 96.6 | true |
| 1 | 128 | 1 | weir | 204853 | 22619 | 227472 | 0 | 11370.1 | 22.527 | 76.7 | 0.0 | 97.2 | true |
| 1 | 128 | 2 | direct | 210971 | 23259 | 234230 | 0 | 11689.8 | 90.111 | 99.1 | 91.1 | 96.4 | true |
| 1 | 128 | 2 | weir | 220077 | 24344 | 244421 | 0 | 12215.6 | 20.479 | 77.7 | 0.0 | 96.8 | true |
| 1 | 128 | 3 | direct | 219963 | 24259 | 244222 | 0 | 12185.9 | 90.111 | 0.0 | 0.0 | 0.0 | true |
| 1 | 128 | 3 | weir | 228077 | 25077 | 253154 | 0 | 12655.1 | 20.479 | 0.0 | 0.0 | 0.0 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 85945 | 1.076 | 83213/9304 | 0.010 | 1.076 | 0 | 32.000 |
| 1 | 8 | 2 | 106683 | 1.076 | 103228/11591 | 0.008 | 0.790 | 0 | 32.000 |
| 1 | 8 | 3 | 115992 | 1.075 | 112137/12554 | 0.008 | 0.682 | 0 | 32.000 |
| 1 | 32 | 1 | 127483 | 1.368 | 156778/17561 | 0.048 | 1.554 | 0 | 32.000 |
| 1 | 32 | 2 | 125254 | 1.372 | 154516/17335 | 0.047 | 1.668 | 0 | 32.000 |
| 1 | 32 | 3 | 131093 | 1.372 | 161794/18128 | 0.046 | 1.497 | 0 | 32.000 |
| 1 | 128 | 1 | 82961 | 2.742 | 204853/22619 | 0.366 | 3.444 | 0 | 32.000 |
| 1 | 128 | 2 | 89478 | 2.732 | 220077/24344 | 0.318 | 3.167 | 0 | 32.000 |
| 1 | 128 | 3 | 92493 | 2.737 | 228077/25077 | 0.332 | 3.117 | 0 | 32.000 |

**Batch 1: database-full-load throughput comparison unavailable.** both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence.

Best verified business QPS within this ladder: direct 11999.8 (128 workers), Weir 12080.3 (128 workers), observed change +0.67%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 8023.1 | 5531.6 | -31.05% | 0.479 / 1.919 / 13.311 | 1.151 / 2.815 / 5.631 |
| 32 | 11548.9 | 8767.4 | -24.08% | 1.151 / 11.263 / 36.863 | 3.327 / 7.679 / 11.263 |
| 128 | 11999.8 | 12080.3 | +0.67% | 1.663 / 90.111 / 106.495 | 10.239 / 20.479 / 28.671 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
