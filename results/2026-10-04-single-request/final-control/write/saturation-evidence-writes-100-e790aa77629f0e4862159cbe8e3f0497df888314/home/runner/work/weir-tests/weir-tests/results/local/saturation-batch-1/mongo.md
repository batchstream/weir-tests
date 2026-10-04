# Database saturation comparison

Backend **mongo**. Started 2026-10-04T00:11:48.015953064Z.

independent OS client processes send one record per native call or unary SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain.

Warmup 10s and measurement 20s per path/stage, 3 paired rounds, requested writes 100%.

Complete business-request budget for both paths: 10s.

Configured Weir Store backend budget: 10s; the active caller deadline also applies.

4 independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.

| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |
| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 8 | 1 | direct | 0 | 54402 | 54402 | 0 | 2719.9 | 3.071 | 99.2 | 97.2 | 97.2 | true |
| 1 | 8 | 1 | weir | 0 | 46806 | 46806 | 0 | 2340.0 | 6.143 | 98.2 | 94.6 | 94.6 | true |
| 1 | 8 | 2 | direct | 0 | 52377 | 52377 | 0 | 2618.7 | 3.071 | 99.3 | 96.8 | 96.8 | true |
| 1 | 8 | 2 | weir | 0 | 45475 | 45475 | 0 | 2271.6 | 6.143 | 99.6 | 98.5 | 98.5 | true |
| 1 | 8 | 3 | direct | 0 | 48594 | 48594 | 0 | 2429.5 | 3.583 | 99.3 | 96.9 | 96.9 | true |
| 1 | 8 | 3 | weir | 0 | 44232 | 44232 | 0 | 2210.9 | 6.655 | 99.6 | 98.5 | 98.5 | true |
| 1 | 32 | 1 | direct | 0 | 54702 | 54702 | 0 | 2734.5 | 61.439 | 99.3 | 97.9 | 97.9 | true |
| 1 | 32 | 1 | weir | 0 | 47115 | 47115 | 0 | 2355.2 | 40.959 | 99.8 | 99.4 | 99.4 | true |
| 1 | 32 | 2 | direct | 0 | 53442 | 53442 | 0 | 2671.7 | 61.439 | 99.1 | 97.9 | 97.9 | true |
| 1 | 32 | 2 | weir | 0 | 45663 | 45663 | 0 | 2282.5 | 40.959 | 99.7 | 99.0 | 99.0 | true |
| 1 | 32 | 3 | direct | 0 | 51826 | 51826 | 0 | 2582.9 | 61.439 | 99.5 | 97.1 | 97.1 | true |
| 1 | 32 | 3 | weir | 0 | 44844 | 44844 | 0 | 2238.9 | 45.055 | 99.7 | 98.0 | 98.0 | true |
| 1 | 128 | 1 | direct | 0 | 49188 | 49188 | 0 | 2450.0 | 106.495 | 99.7 | 98.1 | 98.1 | true |
| 1 | 128 | 1 | weir | 0 | 45144 | 45144 | 0 | 2254.6 | 106.495 | 99.8 | 98.9 | 98.9 | true |
| 1 | 128 | 2 | direct | 0 | 55662 | 55662 | 0 | 2780.9 | 98.303 | 99.6 | 97.9 | 97.9 | true |
| 1 | 128 | 2 | weir | 0 | 45336 | 45336 | 0 | 2258.3 | 106.495 | 99.5 | 97.4 | 97.4 | true |
| 1 | 128 | 3 | direct | 0 | 50944 | 50944 | 0 | 2545.5 | 106.495 | 99.9 | 98.8 | 98.8 | true |
| 1 | 128 | 3 | weir | 0 | 48441 | 48441 | 0 | 2415.0 | 98.303 | 99.4 | 98.0 | 98.0 | true |

Measured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):

| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |
| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| 1 | 8 | 1 | 46806 | 1.000 | 0/46806 | 0.026 | 2.129 | 0 | 32.000 |
| 1 | 8 | 2 | 45475 | 1.000 | 0/45475 | 0.022 | 2.460 | 0 | 32.000 |
| 1 | 8 | 3 | 44232 | 1.000 | 0/44232 | 0.022 | 2.531 | 0 | 32.000 |
| 1 | 32 | 1 | 47115 | 1.000 | 0/47115 | 0.198 | 9.391 | 0 | 32.000 |
| 1 | 32 | 2 | 45663 | 1.000 | 0/45663 | 0.191 | 9.874 | 0 | 32.000 |
| 1 | 32 | 3 | 44844 | 1.000 | 0/44844 | 0.185 | 10.118 | 0 | 32.000 |
| 1 | 128 | 1 | 45144 | 1.000 | 0/45144 | 37.436 | 12.358 | 0 | 32.000 |
| 1 | 128 | 2 | 45336 | 1.000 | 0/45336 | 37.361 | 12.270 | 0 | 32.000 |
| 1 | 128 | 3 | 48441 | 1.000 | 0/48441 | 34.823 | 11.406 | 0 | 32.000 |

**Batch 1 at demonstrated database CPU saturation:** direct 2663.0 logical ops/s (32 workers), Weir 2292.2 logical ops/s (32 workers), Weir/direct 0.8608, change -13.92%.

Best verified business QPS within this ladder: direct 2663.0 (32 workers), Weir 2309.3 (128 workers), observed change -13.28%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.

Observed single-request business performance at matched client concurrency (independent of the database-capacity qualification):

| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |
| ---: | ---: | ---: | ---: | --- | --- |
| 8 | 2589.4 | 2274.2 | -12.17% | 1.663 / 3.327 / 49.151 | 2.815 / 6.143 / 18.431 |
| 32 | 2663.0 | 2292.2 | -13.92% | 5.119 / 61.439 / 73.727 | 9.215 / 40.959 / 73.727 |
| 128 | 2592.0 | 2309.3 | -10.91% | 26.623 / 98.303 / 229.375 | 53.247 / 106.495 / 196.607 |

These QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.

Aggregation evidence: the JSON records one business request per record, Read/Mutate RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.

The JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.
